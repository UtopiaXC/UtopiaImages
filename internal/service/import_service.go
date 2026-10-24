package service

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
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
	PreservePath   bool   `json:"preserve_path"`
	IncludeTopDir  bool   `json:"include_top_dir"`
	ConflictAction string `json:"conflict_action"` // skip / overwrite
}

// ImportZip extracts a zip archive and imports each file
func (s *ImportService) ImportZip(ctx context.Context, user *model.User, zipPath string, policyID uint, opts ImportOptions) (total, imported, skipped int, err error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	var policy model.StoragePolicy
	s.db.Where("id = ? AND user_id = ?", policyID, user.ID).First(&policy)
	if policy.ID == 0 {
		policy = model.StoragePolicy{
			UserID:     user.ID,
			BackendID:  1,
			UseRoot:    user.Role == "admin",
			PathPrefix: "upload",
			NamingRule: "{Y}/{m}/{d}/{random:16}.{ext}",
		}
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		total++
		decodedName := s.decodeZipFilename(f.Name)
		cleanedPath := pathclean.CleanPath(decodedName)
		if cleanedPath == "" || strings.HasPrefix(cleanedPath, "..") {
			skipped++
			continue
		}

		// Check conflict
		var publicPath string
		if opts.PreservePath {
			if user.Role == "admin" && policy.UseRoot {
				publicPath = cleanedPath
			} else {
				publicPath = fmt.Sprintf("%s/%s", user.Username, cleanedPath)
			}
		} else {
			ext := strings.TrimPrefix(filepath.Ext(cleanedPath), ".")
			rendered := s.upload.renderNamingRule(policy.NamingRule, filepath.Base(cleanedPath), ext, "")
			publicPath = s.upload.buildPublicPath(user, policy, rendered)
		}

		var count int64
		s.db.Model(&model.FileRecord{}).Where("public_path = ?", publicPath).Count(&count)
		if count > 0 {
			if opts.ConflictAction == "skip" {
				skipped++
				continue
			}
		}

		rc, err := f.Open()
		if err != nil {
			skipped++
			continue
		}

		// Read header for MIME
		head := make([]byte, 2048)
		hn, _ := io.ReadFull(rc, head)
		mime := mimetype.Detect(head[:hn]).String()

		// Read all
		var buf bytesWriter
		buf.Write(head[:hn])
		io.Copy(&buf, rc)
		rc.Close()

		fileData := buf.Bytes()
		fileSize := int64(len(fileData))
		isImg := strings.HasPrefix(mime, "image/")

		// Save to storage
		storageKey := publicPath
		if err := s.upload.storage.Put(ctx, storageKey, strings.NewReader(string(fileData)), fileSize); err != nil {
			skipped++
			continue
		}

		record := model.FileRecord{
			UserID:      user.ID,
			PolicyID:    policy.ID,
			BackendID:   policy.BackendID,
			PublicPath:  publicPath,
			StorageKey:  storageKey,
			DisplayName: filepath.Base(cleanedPath),
			MimeType:    mime,
			Size:        fileSize,
			IsImage:     isImg,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "public_path"}},
			DoNothing: true,
		}).Create(&record)

		imported++
	}

	return total, imported, skipped, nil
}

type bytesWriter struct {
	buf []byte
}

func (w *bytesWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *bytesWriter) Bytes() []byte {
	return w.buf
}

func (s *ImportService) decodeZipFilename(name string) string {
	// Try GBK decode for windows zip files
	reader := transform.NewReader(strings.NewReader(name), simplifiedchinese.GBK.NewDecoder())
	d, err := io.ReadAll(reader)
	if err == nil && len(d) > 0 && isLikelyValidUtf8(d) {
		return string(d)
	}
	return name
}

func isLikelyValidUtf8(b []byte) bool {
	// Quick heuristic check
	for _, c := range b {
		if c == 0xFF || c == 0xFE {
			return false
		}
	}
	return true
}

// ImportLocalDirectory imports a directory already located on the local server filesystem
func (s *ImportService) ImportLocalDirectory(ctx context.Context, user *model.User, sourceDir string, policyID uint, opts ImportOptions) (int, int, error) {
	total := 0
	imported := 0

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		total++
		_, err = s.upload.Upload(ctx, user, f, filepath.Base(path), info.Size(), UploadOptions{
			PolicyID:    policyID,
			DisplayName: rel,
		})
		if err == nil {
			imported++
		}
		return nil
	})

	return total, imported, err
}
