package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/imaging"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/gabriel-vasile/mimetype"
	"gorm.io/gorm"
)

type UploadService struct {
	db        *gorm.DB
	storage   storage.StorageDriver
	processor imaging.ImageProcessor
}

func NewUploadService(db *gorm.DB, st storage.StorageDriver, proc imaging.ImageProcessor) *UploadService {
	return &UploadService{
		db:        db,
		storage:   st,
		processor: proc,
	}
}

type UploadOptions struct {
	PolicyID            uint
	DisplayName         string
	ConvertToWebP       bool
	Quality             int
	KeepOriginal        bool
	WatermarkMode       string // inherit / on / off
	WatermarkTemplateID *uint
}

// Upload processes and stores an incoming file stream
func (s *UploadService) Upload(ctx context.Context, user *model.User, r io.Reader, originalFilename string, declaredSize int64, opts UploadOptions) (*model.FileRecord, error) {
	// 1. Read first 4KB to detect MIME type via magic number
	headerBuf := make([]byte, 4096)
	n, err := io.ReadFull(r, headerBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}
	headerBuf = headerBuf[:n]

	mime := mimetype.Detect(headerBuf)
	detectedMime := mime.String()
	detectedExt := strings.TrimPrefix(mime.Extension(), ".")
	if detectedExt == "" {
		detectedExt = strings.TrimPrefix(filepath.Ext(originalFilename), ".")
	}

	isImage := strings.HasPrefix(detectedMime, "image/") && !strings.Contains(detectedMime, "svg")

	// 2. Buffer the full stream into memory or temporary file to know exact size and sha256
	var fullBuf bytes.Buffer
	hasher := sha256.New()
	multiWriter := io.MultiWriter(&fullBuf, hasher)

	// Write the initial header bytes
	multiWriter.Write(headerBuf)

	// Stream the remaining content
	copied, err := io.Copy(multiWriter, r)
	if err != nil {
		return nil, fmt.Errorf("failed to read full payload: %w", err)
	}
	actualSize := int64(n) + copied
	fileSha256 := hex.EncodeToString(hasher.Sum(nil))

	// 3. Check storage capacity limit
	var effectiveLimit int64
	if user.StorageLimit != nil {
		effectiveLimit = *user.StorageLimit
	} else if user.Group != nil {
		effectiveLimit = user.Group.DefaultStorageLimit
	}

	if effectiveLimit > 0 && user.UsedBytes+actualSize > effectiveLimit {
		return nil, fmt.Errorf("storage capacity exceeded: current %d + upload %d > limit %d",
			user.UsedBytes, actualSize, effectiveLimit)
	}

	// 4. Resolve Storage Policy
	var policy model.StoragePolicy
	if opts.PolicyID > 0 {
		s.db.Where("id = ? AND user_id = ?", opts.PolicyID, user.ID).First(&policy)
	}
	if policy.ID == 0 {
		// Fallback to user default policy
		s.db.Where("user_id = ? AND is_default = 1", user.ID).First(&policy)
	}
	if policy.ID == 0 {
		// Or any policy of user
		s.db.Where("user_id = ?", user.ID).First(&policy)
	}
	if policy.ID == 0 {
		// Or system default
		policy = model.StoragePolicy{
			UserID:     user.ID,
			BackendID:  1,
			UseRoot:    user.Role == "admin",
			PathPrefix: "upload",
			NamingRule: "{Y}/{m}/{d}/{random:16}.{ext}",
		}
	}

	// 5. Construct public URL path
	renderedName := s.renderNamingRule(policy.NamingRule, originalFilename, detectedExt, fileSha256)
	publicPath := s.buildPublicPath(user, policy, renderedName)

	// 6. Handle Image conversion if requested
	finalBytes := fullBuf.Bytes()
	finalMime := detectedMime
	finalSize := actualSize
	sourceKey := ""
	var sourceSize int64

	var imgW, imgH int
	if isImage {
		// Read dimensions
		w, h, _ := s.processor.GetDimensions(bytes.NewReader(finalBytes))
		imgW = w
		imgH = h

		if opts.ConvertToWebP && detectedMime != "image/webp" {
			// Save unconverted original if requested
			if opts.KeepOriginal {
				sourceKey = "original/" + publicPath
				sourceSize = actualSize
				if err := s.storage.Put(ctx, sourceKey, bytes.NewReader(finalBytes), sourceSize); err != nil {
					return nil, fmt.Errorf("failed to store original image: %w", err)
				}
			}

			// Convert
			q := opts.Quality
			if q <= 0 {
				q = 85
			}
			converted, outMime, err := s.processor.Process(bytes.NewReader(finalBytes), imaging.ProcessOptions{
				Format:  "webp",
				Quality: q,
			})
			if err == nil {
				finalBytes = converted
				finalMime = outMime
				finalSize = int64(len(converted))
			}
		}
	}

	// 7. Store the primary file
	storageKey := publicPath
	if err := s.storage.Put(ctx, storageKey, bytes.NewReader(finalBytes), finalSize); err != nil {
		if sourceKey != "" {
			s.storage.Delete(ctx, sourceKey)
		}
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	displayName := opts.DisplayName
	if displayName == "" {
		displayName = originalFilename
	}

	// 8. Create Database File Record
	fileRecord := &model.FileRecord{
		UserID:              user.ID,
		PolicyID:            policy.ID,
		BackendID:           policy.BackendID,
		PublicPath:          publicPath,
		StorageKey:          storageKey,
		DisplayName:         displayName,
		MimeType:            finalMime,
		Size:                finalSize,
		Width:               imgW,
		Height:              imgH,
		Sha256:              fileSha256,
		IsImage:             isImage,
		SourceKey:           sourceKey,
		SourceSize:          sourceSize,
		WatermarkMode:       opts.WatermarkMode,
		WatermarkTemplateID: opts.WatermarkTemplateID,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(fileRecord).Error; err != nil {
			return err
		}
		// Update user quota usage
		totalAllocated := finalSize + sourceSize
		return tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"used_bytes": gorm.Expr("used_bytes + ?", totalAllocated),
			"file_count": gorm.Expr("file_count + 1"),
		}).Error
	})

	if err != nil {
		s.storage.Delete(ctx, storageKey)
		if sourceKey != "" {
			s.storage.Delete(ctx, sourceKey)
		}
		return nil, fmt.Errorf("failed to save file record: %w", err)
	}

	return fileRecord, nil
}

func (s *UploadService) buildPublicPath(user *model.User, policy model.StoragePolicy, filename string) string {
	parts := []string{}

	// Normal users must be namespaced under /{username}/
	// Only admin can configure policy with UseRoot = true
	if user.Role != "admin" || !policy.UseRoot {
		parts = append(parts, user.Username)
	}

	if policy.PathPrefix != "" {
		parts = append(parts, strings.Trim(policy.PathPrefix, "/"))
	}

	parts = append(parts, filename)
	raw := strings.Join(parts, "/")
	return pathclean.CleanPath(raw)
}

func (s *UploadService) renderNamingRule(rule, origName, ext, sha string) string {
	if rule == "" {
		rule = "{Y}/{m}/{d}/{random:16}.{ext}"
	}

	now := time.Now()
	rule = strings.ReplaceAll(rule, "{Y}", now.Format("2006"))
	rule = strings.ReplaceAll(rule, "{y}", now.Format("06"))
	rule = strings.ReplaceAll(rule, "{m}", now.Format("01"))
	rule = strings.ReplaceAll(rule, "{d}", now.Format("02"))
	rule = strings.ReplaceAll(rule, "{H}", now.Format("15"))
	rule = strings.ReplaceAll(rule, "{i}", now.Format("04"))
	rule = strings.ReplaceAll(rule, "{s}", now.Format("05"))
	rule = strings.ReplaceAll(rule, "{timestamp}", strconv.FormatInt(now.Unix(), 10))
	rule = strings.ReplaceAll(rule, "{ext}", ext)

	cleanOrig := strings.TrimSuffix(filepath.Base(origName), filepath.Ext(origName))
	rule = strings.ReplaceAll(rule, "{filename}", cleanOrig)

	if len(sha) >= 16 {
		rule = strings.ReplaceAll(rule, "{sha1}", sha[:16])
		rule = strings.ReplaceAll(rule, "{md5}", sha[:16])
	}

	// Handle {random:N}
	for {
		idx := strings.Index(rule, "{random:")
		if idx == -1 {
			break
		}
		end := strings.Index(rule[idx:], "}")
		if end == -1 {
			break
		}
		numStr := rule[idx+8 : idx+end]
		n, err := strconv.Atoi(numStr)
		if err != nil || n <= 0 || n > 64 {
			n = 16
		}
		randBytes := make([]byte, (n+1)/2)
		rand.Read(randBytes)
		randHex := hex.EncodeToString(randBytes)[:n]
		rule = rule[:idx] + randHex + rule[idx+end+1:]
	}

	return rule
}

func (s *UploadService) DeleteFile(ctx context.Context, fileID, userID uint, isAdmin bool) error {
	var file model.FileRecord
	query := s.db.Where("id = ?", fileID)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&file).Error; err != nil {
		return errors.New("file not found")
	}

	// Delete from storage
	s.storage.Delete(ctx, file.StorageKey)
	if file.SourceKey != "" {
		s.storage.Delete(ctx, file.SourceKey)
	}

	// Delete from DB & return storage capacity
	totalSize := file.Size + file.SourceSize
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&file).Error; err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", file.UserID).Updates(map[string]any{
			"used_bytes": gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", totalSize, totalSize),
			"file_count": gorm.Expr("CASE WHEN file_count >= 1 THEN file_count - 1 ELSE 0 END"),
		}).Error
	})
}
