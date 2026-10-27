package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
	"github.com/bodgit/sevenzip"
	"github.com/gabriel-vasile/mimetype"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ImportService struct {
	db     *gorm.DB
	upload *UploadService
}

func NewImportService(db *gorm.DB, up *UploadService) *ImportService {
	return &ImportService{db: db, upload: up}
}

func (s *ImportService) SetDB(db *gorm.DB) {
	s.db = db
}

type ImportOptions struct {
	PreservePath        bool   `json:"preserve_path"`
	StripTopDir         bool   `json:"strip_top_dir"`
	ConflictAction      string `json:"conflict_action"` // skip / overwrite
	WatermarkMode       string `json:"watermark_mode"`  // inherit / on / off
	WatermarkTemplateID *uint  `json:"watermark_template_id"`
	HotlinkMode         string `json:"hotlink_mode"` // inherit / on / off
}

const (
	MaxArchiveTotalBytes = 100 * 1024 * 1024 * 1024 // 100 GB max uncompressed archive size
	MaxArchiveSingleFile = 10 * 1024 * 1024 * 1024  // 10 GB max per single file
	MaxArchiveFileCount  = 500000                   // Max 500,000 files in archive
)

func (s *ImportService) resolvePolicy(user *model.User, policyID uint) (model.StoragePolicy, error) {
	var policy model.StoragePolicy
	if policyID > 0 {
		var count int64
		s.db.Table("role_policies").Where("role_id = ? AND policy_id = ?", user.RoleID, policyID).Count(&count)
		if count > 0 {
			s.db.Where("id = ?", policyID).First(&policy)
		}
	}
	if policy.ID == 0 {
		var defaultPolicyID uint
		s.db.Table("role_policies").
			Joins("JOIN storage_policies ON storage_policies.id = role_policies.policy_id").
			Where("role_policies.role_id = ? AND storage_policies.is_default = 1", user.RoleID).
			Select("storage_policies.id").Scan(&defaultPolicyID)
		if defaultPolicyID > 0 {
			s.db.Where("id = ?", defaultPolicyID).First(&policy)
		}
	}
	if policy.ID == 0 {
		var firstPolicyID uint
		s.db.Table("role_policies").Where("role_id = ?", user.RoleID).Select("policy_id").Scan(&firstPolicyID)
		if firstPolicyID > 0 {
			s.db.Where("id = ?", firstPolicyID).First(&policy)
		}
	}
	if policy.ID == 0 {
		return policy, errors.New("no permitted storage policy found for user role")
	}
	return policy, nil
}

// ImportArchive extracts a supported archive (.zip, .7z, .tar, .tar.gz, .tgz, .gz) and imports files via streaming
func (s *ImportService) ImportArchive(ctx context.Context, user *model.User, archivePath, originalFilename string, policyID uint, opts ImportOptions) (total, imported, skipped int, err error) {
	policy, err := s.resolvePolicy(user, policyID)
	if err != nil {
		return 0, 0, 0, err
	}

	watermarkMode := opts.WatermarkMode
	if watermarkMode == "" {
		watermarkMode = "inherit"
	}
	hotlinkMode := opts.HotlinkMode
	if hotlinkMode == "" {
		hotlinkMode = "inherit"
	}

	if opts.WatermarkTemplateID != nil && *opts.WatermarkTemplateID > 0 {
		var count int64
		s.db.Model(&model.WatermarkTemplate{}).
			Where("id = ? AND (owner_id IS NULL OR owner_id = ?)", *opts.WatermarkTemplateID, user.ID).
			Count(&count)
		if count == 0 {
			opts.WatermarkTemplateID = nil
		}
	}

	processItem := func(itemPath string, itemSize int64, itemReader io.Reader) error {
		total++
		cleanedPath := pathclean.CleanPath(itemPath)
		if cleanedPath == "" || strings.HasPrefix(cleanedPath, "..") {
			skipped++
			return nil
		}

		if opts.StripTopDir {
			parts := strings.Split(cleanedPath, "/")
			if len(parts) > 1 {
				cleanedPath = strings.Join(parts[1:], "/")
			}
		}

		var publicPath string
		if opts.PreservePath {
			publicPath = s.upload.buildPublicPath(user, policy, cleanedPath)
		} else {
			ext := strings.TrimPrefix(filepath.Ext(cleanedPath), ".")
			rendered := s.upload.renderNamingRule(policy.NamingRule, filepath.Base(cleanedPath), ext, "")
			publicPath = s.upload.buildPublicPath(user, policy, rendered)
		}

		var existingFile model.FileRecord
		hasExisting := false
		if err := s.db.Unscoped().Where("public_path = ?", publicPath).First(&existingFile).Error; err == nil {
			hasExisting = true
		}

		if hasExisting {
			if opts.ConflictAction == "skip" {
				skipped++
				return nil
			}

			isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
			if !isAdmin && existingFile.UserID != user.ID {
				slog.Warn("Import overwrite skipped: cross-user permission denied", "path", publicPath, "owner", existingFile.UserID, "operator", user.ID)
				skipped++
				return nil
			}
		}

		// Calculate storage size difference
		var sizeDelta int64 = itemSize
		if hasExisting && opts.ConflictAction == "overwrite" {
			sizeDelta = itemSize - existingFile.Size
		}

		targetQuotaUserID := user.ID
		if hasExisting && opts.ConflictAction == "overwrite" {
			targetQuotaUserID = existingFile.UserID
		}

		// Enforce user storage quota
		if sizeDelta > 0 {
			var quotaUser model.User
			if targetQuotaUserID == user.ID {
				quotaUser = *user
			} else {
				s.db.Preload("Role").Where("id = ?", targetQuotaUserID).First(&quotaUser)
			}
			var qRole model.Role
			if quotaUser.Role != nil {
				qRole = *quotaUser.Role
			} else {
				s.db.Where("id = ?", quotaUser.RoleID).First(&qRole)
			}
			if qRole.QuotaBytes > 0 && quotaUser.UsedBytes+sizeDelta > qRole.QuotaBytes {
				slog.Warn("Import skipped: user quota exceeded", "path", publicPath, "user", quotaUser.Username)
				skipped++
				return nil
			}
		}

		// Enforce storage policy capacity
		if sizeDelta > 0 && policy.MaxCapacityBytes > 0 && policy.UsedBytes+sizeDelta > policy.MaxCapacityBytes {
			slog.Warn("Import skipped: storage policy capacity exceeded", "path", publicPath, "policy", policy.Name)
			skipped++
			return nil
		}

		// Stream to storage while computing SHA256 (Constant O(1) memory)
		headBuf := make([]byte, 2048)
		n, _ := io.ReadFull(itemReader, headBuf)
		headBuf = headBuf[:n]
		mime := mimetype.Detect(headBuf).String()
		isImg := strings.HasPrefix(mime, "image/") && !strings.Contains(mime, "svg")

		fullStream := io.MultiReader(bytes.NewReader(headBuf), itemReader)
		hasher := sha256.New()
		teeStream := io.TeeReader(fullStream, hasher)

		storageKey := publicPath
		if err := s.upload.storage.Put(ctx, storageKey, teeStream, itemSize); err != nil {
			slog.Error("Failed to store imported archive file", "path", publicPath, "error", err)
			skipped++
			return nil
		}

		sha256Hex := hex.EncodeToString(hasher.Sum(nil))
		finalItemSize := itemSize
		if finalItemSize <= 0 {
			if st, err := s.upload.storage.Stat(ctx, storageKey); err == nil {
				finalItemSize = st.Size
			}
		}

		// Read dimensions from saved local file
		var width, height int
		if isImg && s.upload != nil && s.upload.processor != nil {
			if localPath, ok := s.upload.storage.GetLocalPath(storageKey); ok {
				if imgF, err := os.Open(localPath); err == nil {
					if w, h, err := s.upload.processor.GetDimensions(imgF); err == nil {
						width = w
						height = h
					}
					_ = imgF.Close()
				}
			} else {
				if rCloser, err := s.upload.storage.Get(ctx, storageKey); err == nil {
					if w, h, err := s.upload.processor.GetDimensions(rCloser); err == nil {
						width = w
						height = h
					}
					_ = rCloser.Close()
				}
			}
		}

		record := model.FileRecord{
			UserID:              user.ID,
			PolicyID:            policy.ID,
			PublicPath:          publicPath,
			StorageKey:          storageKey,
			DisplayName:         filepath.Base(cleanedPath),
			MimeType:            mime,
			Size:                finalItemSize,
			Sha256:              sha256Hex,
			Width:               width,
			Height:              height,
			IsImage:             isImg,
			WatermarkMode:       watermarkMode,
			WatermarkTemplateID: opts.WatermarkTemplateID,
			HotlinkMode:         hotlinkMode,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}

		if hasExisting && opts.ConflictAction == "overwrite" {
			s.db.Unscoped().Model(&model.FileRecord{}).Where("id = ?", existingFile.ID).Updates(map[string]any{
				"deleted_at":            nil,
				"sign_version":          gorm.Expr("sign_version + 1"),
				"storage_key":           storageKey,
				"mime_type":             mime,
				"size":                  finalItemSize,
				"sha256":                sha256Hex,
				"width":                 width,
				"height":                height,
				"is_image":              isImg,
				"watermark_mode":        watermarkMode,
				"watermark_template_id": opts.WatermarkTemplateID,
				"hotlink_mode":          hotlinkMode,
				"updated_at":            time.Now(),
			})
		} else {
			s.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "public_path"}},
				DoNothing: true,
			}).Create(&record)
		}

		// Track user used quota & policy used capacity
		if sizeDelta != 0 {
			userUpdates := map[string]any{
				"used_bytes": gorm.Expr("CASE WHEN used_bytes + ? >= 0 THEN used_bytes + ? ELSE 0 END", sizeDelta, sizeDelta),
			}
			if !hasExisting {
				userUpdates["file_count"] = gorm.Expr("file_count + 1")
			}
			s.db.Model(&model.User{}).Where("id = ?", targetQuotaUserID).Updates(userUpdates)
			s.db.Model(&model.StoragePolicy{}).Where("id = ?", policy.ID).Update("used_bytes", gorm.Expr("CASE WHEN used_bytes + ? >= 0 THEN used_bytes + ? ELSE 0 END", sizeDelta, sizeDelta))
			if targetQuotaUserID == user.ID {
				user.UsedBytes += sizeDelta
			}
			policy.UsedBytes += sizeDelta
		} else if !hasExisting {
			s.db.Model(&model.User{}).Where("id = ?", targetQuotaUserID).Update("file_count", gorm.Expr("file_count + 1"))
		}

		imported++
		return nil
	}

	ext := strings.ToLower(originalFilename)
	if strings.HasSuffix(ext, ".tar.gz") || strings.HasSuffix(ext, ".tgz") {
		err = s.streamTarGz(archivePath, processItem)
	} else if strings.HasSuffix(ext, ".tar") {
		err = s.streamTar(archivePath, processItem)
	} else if strings.HasSuffix(ext, ".7z") {
		err = s.stream7z(archivePath, processItem)
	} else if strings.HasSuffix(ext, ".gz") {
		err = s.streamSingleGz(archivePath, originalFilename, processItem)
	} else {
		err = s.streamZip(archivePath, processItem)
	}

	return total, imported, skipped, err
}

// ImportZip is kept for backward compatibility, routing to ImportArchive
func (s *ImportService) ImportZip(ctx context.Context, user *model.User, zipPath string, policyID uint, opts ImportOptions) (total, imported, skipped int, err error) {
	return s.ImportArchive(ctx, user, zipPath, filepath.Base(zipPath), policyID, opts)
}

func (s *ImportService) streamZip(archivePath string, fn func(path string, size int64, r io.Reader) error) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	if len(r.File) > MaxArchiveFileCount {
		return fmt.Errorf("archive contains %d files, exceeding maximum allowed of %d", len(r.File), MaxArchiveFileCount)
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > uint64(MaxArchiveSingleFile) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		decodedName := s.decodeFilename(f.Name)
		_ = fn(decodedName, int64(f.UncompressedSize64), rc)
		_ = rc.Close()
	}
	return nil
}

func (s *ImportService) stream7z(archivePath string, fn func(path string, size int64, r io.Reader) error) error {
	r, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open 7z file: %w", err)
	}
	defer r.Close()

	if len(r.File) > MaxArchiveFileCount {
		return fmt.Errorf("archive contains %d files, exceeding maximum allowed of %d", len(r.File), MaxArchiveFileCount)
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize > uint64(MaxArchiveSingleFile) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		decodedName := s.decodeFilename(f.Name)
		_ = fn(decodedName, int64(f.UncompressedSize), rc)
		_ = rc.Close()
	}
	return nil
}

func (s *ImportService) streamTar(archivePath string, fn func(path string, size int64, r io.Reader) error) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open tar file: %w", err)
	}
	defer f.Close()

	return s.streamTarReader(f, fn)
}

func (s *ImportService) streamTarGz(archivePath string, fn func(path string, size int64, r io.Reader) error) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open tar.gz file: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to initialize gzip reader: %w", err)
	}
	defer gz.Close()

	return s.streamTarReader(gz, fn)
}

func (s *ImportService) streamTarReader(r io.Reader, fn func(path string, size int64, r io.Reader) error) error {
	tr := tar.NewReader(r)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		count++
		if count > MaxArchiveFileCount {
			return fmt.Errorf("archive contains more than %d files", MaxArchiveFileCount)
		}
		if hdr.Size > MaxArchiveSingleFile {
			continue
		}
		decodedName := s.decodeFilename(hdr.Name)
		_ = fn(decodedName, hdr.Size, tr)
	}
	return nil
}

func (s *ImportService) streamSingleGz(archivePath, originalFilename string, fn func(path string, size int64, r io.Reader) error) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open gz file: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to initialize gzip reader: %w", err)
	}
	defer gz.Close()

	targetName := strings.TrimSuffix(originalFilename, ".gz")
	if gz.Name != "" {
		targetName = gz.Name
	}
	targetName = s.decodeFilename(targetName)
	return fn(targetName, -1, gz)
}

func (s *ImportService) decodeFilename(name string) string {
	// If already valid UTF-8, return directly to prevent mojibake
	if utf8.ValidString(name) {
		return name
	}

	// Try GB18030 decode for legacy Windows archive
	reader := transform.NewReader(strings.NewReader(name), simplifiedchinese.GB18030.NewDecoder())
	d, err := io.ReadAll(reader)
	if err == nil && len(d) > 0 && utf8.Valid(d) {
		return string(d)
	}

	return name
}
