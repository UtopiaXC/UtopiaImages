package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	MaxArchiveTotalBytes = 500 * 1024 * 1024 // 500 MB max uncompressed archive size
	MaxArchiveSingleFile = 50 * 1024 * 1024  // 50 MB max per single file
	MaxArchiveFileCount  = 1000              // Max 1000 files in archive
)

type extractedItem struct {
	Path string
	Data []byte
	Size int64
}

// ImportArchive extracts a supported archive (.zip, .7z, .tar, .tar.gz, .tgz, .gz) and imports files
func (s *ImportService) ImportArchive(ctx context.Context, user *model.User, archivePath, originalFilename string, policyID uint, opts ImportOptions) (total, imported, skipped int, err error) {
	ext := strings.ToLower(originalFilename)

	var items []extractedItem
	if strings.HasSuffix(ext, ".tar.gz") || strings.HasSuffix(ext, ".tgz") {
		items, err = s.extractTarGz(archivePath)
	} else if strings.HasSuffix(ext, ".tar") {
		items, err = s.extractTar(archivePath)
	} else if strings.HasSuffix(ext, ".7z") {
		items, err = s.extract7z(archivePath)
	} else if strings.HasSuffix(ext, ".gz") {
		items, err = s.extractSingleGz(archivePath, originalFilename)
	} else {
		// Default to zip
		items, err = s.extractZip(archivePath)
	}

	if err != nil {
		return 0, 0, 0, err
	}

	return s.processExtractedItems(ctx, user, items, policyID, opts)
}

// ImportZip is kept for backward compatibility, routing to ImportArchive
func (s *ImportService) ImportZip(ctx context.Context, user *model.User, zipPath string, policyID uint, opts ImportOptions) (total, imported, skipped int, err error) {
	return s.ImportArchive(ctx, user, zipPath, filepath.Base(zipPath), policyID, opts)
}

func (s *ImportService) extractZip(archivePath string) ([]extractedItem, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	if len(r.File) > MaxArchiveFileCount {
		return nil, fmt.Errorf("archive contains %d files, exceeding maximum allowed of %d", len(r.File), MaxArchiveFileCount)
	}

	var items []extractedItem
	var totalExtracted int64

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > uint64(MaxArchiveSingleFile) {
			return nil, fmt.Errorf("file '%s' exceeds single file size limit (%d MB)", f.Name, MaxArchiveSingleFile/(1024*1024))
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		limited := io.LimitReader(rc, MaxArchiveSingleFile+1)
		data, err := io.ReadAll(limited)
		rc.Close()
		if err != nil {
			continue
		}
		if int64(len(data)) > MaxArchiveSingleFile {
			return nil, fmt.Errorf("file '%s' exceeds single file size limit (%d MB)", f.Name, MaxArchiveSingleFile/(1024*1024))
		}

		totalExtracted += int64(len(data))
		if totalExtracted > MaxArchiveTotalBytes {
			return nil, fmt.Errorf("total uncompressed size exceeds maximum allowed limit (%d MB)", MaxArchiveTotalBytes/(1024*1024))
		}

		decodedName := s.decodeFilename(f.Name)
		items = append(items, extractedItem{
			Path: decodedName,
			Data: data,
			Size: int64(len(data)),
		})
	}
	return items, nil
}

func (s *ImportService) extract7z(archivePath string) ([]extractedItem, error) {
	r, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open 7z file: %w", err)
	}
	defer r.Close()

	if len(r.File) > MaxArchiveFileCount {
		return nil, fmt.Errorf("archive contains %d files, exceeding maximum allowed of %d", len(r.File), MaxArchiveFileCount)
	}

	var items []extractedItem
	var totalExtracted int64

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize > uint64(MaxArchiveSingleFile) {
			return nil, fmt.Errorf("file '%s' exceeds single file size limit (%d MB)", f.Name, MaxArchiveSingleFile/(1024*1024))
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		limited := io.LimitReader(rc, MaxArchiveSingleFile+1)
		data, err := io.ReadAll(limited)
		rc.Close()
		if err != nil {
			continue
		}
		if int64(len(data)) > MaxArchiveSingleFile {
			return nil, fmt.Errorf("file '%s' exceeds single file size limit (%d MB)", f.Name, MaxArchiveSingleFile/(1024*1024))
		}

		totalExtracted += int64(len(data))
		if totalExtracted > MaxArchiveTotalBytes {
			return nil, fmt.Errorf("total uncompressed size exceeds maximum allowed limit (%d MB)", MaxArchiveTotalBytes/(1024*1024))
		}

		decodedName := s.decodeFilename(f.Name)
		items = append(items, extractedItem{
			Path: decodedName,
			Data: data,
			Size: int64(len(data)),
		})
	}
	return items, nil
}

func (s *ImportService) extractTar(archivePath string) ([]extractedItem, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tar file: %w", err)
	}
	defer f.Close()

	return s.readTarStream(f)
}

func (s *ImportService) extractTarGz(archivePath string) ([]extractedItem, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tar.gz file: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gzip reader: %w", err)
	}
	defer gz.Close()

	return s.readTarStream(gz)
}

func (s *ImportService) readTarStream(r io.Reader) ([]extractedItem, error) {
	tr := tar.NewReader(r)
	var items []extractedItem
	var totalExtracted int64
	count := 0

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return items, err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		count++
		if count > MaxArchiveFileCount {
			return nil, fmt.Errorf("archive contains more than %d files", MaxArchiveFileCount)
		}

		limited := io.LimitReader(tr, MaxArchiveSingleFile+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			continue
		}
		if int64(len(data)) > MaxArchiveSingleFile {
			return nil, fmt.Errorf("file '%s' exceeds single file size limit (%d MB)", hdr.Name, MaxArchiveSingleFile/(1024*1024))
		}

		totalExtracted += int64(len(data))
		if totalExtracted > MaxArchiveTotalBytes {
			return nil, fmt.Errorf("total uncompressed size exceeds maximum allowed limit (%d MB)", MaxArchiveTotalBytes/(1024*1024))
		}

		decodedName := s.decodeFilename(hdr.Name)
		items = append(items, extractedItem{
			Path: decodedName,
			Data: data,
			Size: int64(len(data)),
		})
	}
	return items, nil
}

func (s *ImportService) extractSingleGz(archivePath, originalFilename string) ([]extractedItem, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open gz file: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gzip reader: %w", err)
	}
	defer gz.Close()

	limited := io.LimitReader(gz, MaxArchiveSingleFile+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read gz data: %w", err)
	}
	if int64(len(data)) > MaxArchiveSingleFile {
		return nil, fmt.Errorf("file exceeds single file size limit (%d MB)", MaxArchiveSingleFile/(1024*1024))
	}

	targetName := strings.TrimSuffix(originalFilename, ".gz")
	if gz.Name != "" {
		targetName = gz.Name
	}
	targetName = s.decodeFilename(targetName)

	return []extractedItem{
		{
			Path: targetName,
			Data: data,
			Size: int64(len(data)),
		},
	}, nil
}

func (s *ImportService) processExtractedItems(ctx context.Context, user *model.User, items []extractedItem, policyID uint, opts ImportOptions) (total, imported, skipped int, err error) {
	var policy model.StoragePolicy
	if policyID > 0 {
		s.db.Where("id = ?", policyID).First(&policy)
	}
	if policy.ID == 0 {
		s.db.Where("is_default = 1").First(&policy)
	}
	if policy.ID == 0 {
		s.db.First(&policy)
	}

	watermarkMode := opts.WatermarkMode
	if watermarkMode == "" {
		watermarkMode = "inherit"
	}
	hotlinkMode := opts.HotlinkMode
	if hotlinkMode == "" {
		hotlinkMode = "inherit"
	}

	var role model.Role
	if user.Role != nil {
		role = *user.Role
	} else {
		s.db.Where("id = ?", user.RoleID).First(&role)
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

	for _, item := range items {
		total++
		cleanedPath := pathclean.CleanPath(item.Path)
		if cleanedPath == "" || strings.HasPrefix(cleanedPath, "..") {
			skipped++
			continue
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

		var existingCount int64
		s.db.Model(&model.FileRecord{}).Where("public_path = ?", publicPath).Count(&existingCount)
		if existingCount > 0 {
			if opts.ConflictAction == "skip" {
				skipped++
				continue
			}
		}

		// Calculate storage size difference
		var sizeDelta int64 = item.Size
		if opts.ConflictAction == "overwrite" && existingCount > 0 {
			var existingFile model.FileRecord
			if err := s.db.Where("public_path = ?", publicPath).First(&existingFile).Error; err == nil {
				sizeDelta = item.Size - existingFile.Size
			}
		}

		// Enforce user storage quota
		if sizeDelta > 0 && role.QuotaBytes > 0 && user.UsedBytes+sizeDelta > role.QuotaBytes {
			slog.Warn("Import skipped: user quota exceeded", "path", publicPath, "user", user.Username)
			skipped++
			continue
		}

		// Enforce storage policy capacity
		if sizeDelta > 0 && policy.MaxCapacityBytes > 0 && policy.UsedBytes+sizeDelta > policy.MaxCapacityBytes {
			slog.Warn("Import skipped: storage policy capacity exceeded", "path", publicPath, "policy", policy.Name)
			skipped++
			continue
		}

		// Detect MIME type
		headLen := 2048
		if len(item.Data) < headLen {
			headLen = len(item.Data)
		}
		mime := mimetype.Detect(item.Data[:headLen]).String()
		isImg := strings.HasPrefix(mime, "image/")

		// Dimensions if image
		var width, height int
		if isImg && s.upload != nil && s.upload.processor != nil {
			w, h, err := s.upload.processor.GetDimensions(bytes.NewReader(item.Data))
			if err == nil {
				width = w
				height = h
			}
		}

		// Compute Sha256
		hashSum := sha256.Sum256(item.Data)
		sha256Hex := hex.EncodeToString(hashSum[:])

		storageKey := publicPath
		if err := s.upload.storage.Put(ctx, storageKey, bytes.NewReader(item.Data), item.Size); err != nil {
			slog.Error("Failed to store imported archive file", "path", publicPath, "error", err)
			skipped++
			continue
		}

		record := model.FileRecord{
			UserID:              user.ID,
			PolicyID:            policy.ID,
			PublicPath:          publicPath,
			StorageKey:          storageKey,
			DisplayName:         filepath.Base(cleanedPath),
			MimeType:            mime,
			Size:                item.Size,
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

		if opts.ConflictAction == "overwrite" && existingCount > 0 {
			s.db.Where("public_path = ?", publicPath).Updates(map[string]any{
				"storage_key":           storageKey,
				"mime_type":             mime,
				"size":                  item.Size,
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
				"used_bytes": gorm.Expr("used_bytes + ?", sizeDelta),
			}
			if existingCount == 0 {
				userUpdates["file_count"] = gorm.Expr("file_count + 1")
			}
			s.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(userUpdates)
			s.db.Model(&model.StoragePolicy{}).Where("id = ?", policy.ID).Update("used_bytes", gorm.Expr("used_bytes + ?", sizeDelta))
			user.UsedBytes += sizeDelta
			policy.UsedBytes += sizeDelta
		} else if existingCount == 0 {
			s.db.Model(&model.User{}).Where("id = ?", user.ID).Update("file_count", gorm.Expr("file_count + 1"))
		}

		imported++
	}

	return total, imported, skipped, nil
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
