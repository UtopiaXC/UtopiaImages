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

type ImportOptions struct {
	PreservePath        bool   `json:"preserve_path"`
	StripTopDir         bool   `json:"strip_top_dir"`
	ConflictAction      string `json:"conflict_action"` // skip / overwrite
	WatermarkMode       string `json:"watermark_mode"`  // inherit / on / off
	WatermarkTemplateID *uint  `json:"watermark_template_id"`
	HotlinkMode         string `json:"hotlink_mode"` // inherit / on / off
}

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

	var items []extractedItem
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
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

	var items []extractedItem
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
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

		data, err := io.ReadAll(tr)
		if err != nil {
			continue
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

	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("failed to read gz data: %w", err)
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

		// Track user used quota
		s.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"used_bytes": gorm.Expr("used_bytes + ?", item.Size),
			"file_count": gorm.Expr("file_count + 1"),
		})

		imported++
	}

	return total, imported, skipped, nil
}

func (s *ImportService) decodeFilename(name string) string {
	// If already valid UTF-8, return directly to prevent mojibake
	if utf8.ValidString(name) {
		return name
	}

	// Try GB18030 decode for legacy Windows archives
	reader := transform.NewReader(strings.NewReader(name), simplifiedchinese.GB18030.NewDecoder())
	d, err := io.ReadAll(reader)
	if err == nil && len(d) > 0 && utf8.Valid(d) {
		return string(d)
	}

	return name
}
