package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/cache"
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
	cache     *cache.DerivedCache
}

func NewUploadService(db *gorm.DB, st storage.StorageDriver, proc imaging.ImageProcessor, dc *cache.DerivedCache) *UploadService {
	return &UploadService{
		db:        db,
		storage:   st,
		processor: proc,
		cache:     dc,
	}
}

func (s *UploadService) SetDB(db *gorm.DB) {
	s.db = db
}

type UploadOptions struct {
	PolicyID            uint
	DisplayName         string
	WatermarkMode       string // inherit / on / off
	WatermarkTemplateID *uint
}

// Upload processes and stores an incoming file stream in its original form
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

	// Prioritize original file extension unconditionally
	origExt := strings.TrimPrefix(filepath.Ext(originalFilename), ".")
	finalExt := origExt
	if finalExt == "" {
		finalExt = strings.TrimPrefix(mime.Extension(), ".")
	}
	if finalExt == "" {
		finalExt = "bin"
	}

	isImage := strings.HasPrefix(detectedMime, "image/") && !strings.Contains(detectedMime, "svg")

	// 2. Buffer the full stream into memory to know exact size and sha256
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

	// 3. Check storage capacity limit against User's Role
	var role model.Role
	if user.Role != nil {
		role = *user.Role
	} else {
		s.db.Where("id = ?", user.RoleID).First(&role)
	}

	if role.QuotaBytes > 0 && user.UsedBytes+actualSize > role.QuotaBytes {
		return nil, fmt.Errorf("user storage quota exceeded: current %d + upload %d > limit %d",
			user.UsedBytes, actualSize, role.QuotaBytes)
	}

	// 4. Resolve Storage Policy permitted for user's Role
	var policy model.StoragePolicy
	if opts.PolicyID > 0 {
		var count int64
		s.db.Table("role_policies").Where("role_id = ? AND policy_id = ?", user.RoleID, opts.PolicyID).Count(&count)
		if count > 0 {
			s.db.Where("id = ?", opts.PolicyID).First(&policy)
		}
	}
	if policy.ID == 0 {
		// Find default policy assigned to this role
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
		// Fallback to any policy permitted for this role
		var firstPolicyID uint
		s.db.Table("role_policies").Where("role_id = ?", user.RoleID).Select("policy_id").Scan(&firstPolicyID)
		if firstPolicyID > 0 {
			s.db.Where("id = ?", firstPolicyID).First(&policy)
		}
	}
	if policy.ID == 0 {
		s.db.Where("is_default = 1").First(&policy)
		if policy.ID == 0 {
			s.db.First(&policy)
		}
	}

	// Check Storage Policy capacity limit
	if policy.MaxCapacityBytes > 0 && policy.UsedBytes+actualSize > policy.MaxCapacityBytes {
		return nil, fmt.Errorf("storage policy '%s' capacity exceeded", policy.Name)
	}

	// 5. Construct public URL path with preserved original extension
	renderedName := s.renderNamingRule(policy.NamingRule, originalFilename, finalExt, fileSha256)
	publicPath := s.buildPublicPath(user, policy, renderedName)

	finalBytes := fullBuf.Bytes()
	finalMime := detectedMime
	finalSize := actualSize

	var imgW, imgH int
	if isImage {
		// Read dimensions for metadata
		w, h, _ := s.processor.GetDimensions(bytes.NewReader(finalBytes))
		imgW = w
		imgH = h
	}

	// 6. Store original file strictly (never converted on upload)
	storageKey := publicPath
	if err := s.storage.Put(ctx, storageKey, bytes.NewReader(finalBytes), finalSize); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	displayName := opts.DisplayName
	if displayName == "" {
		displayName = originalFilename
	}

	// 7. Create Database File Record
	fileRecord := &model.FileRecord{
		UserID:              user.ID,
		PolicyID:            policy.ID,
		PublicPath:          publicPath,
		StorageKey:          storageKey,
		DisplayName:         displayName,
		MimeType:            finalMime,
		Size:                finalSize,
		Width:               imgW,
		Height:              imgH,
		Sha256:              fileSha256,
		IsImage:             isImage,
		SourceKey:           "",
		SourceSize:          0,
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
		if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"used_bytes": gorm.Expr("used_bytes + ?", finalSize),
			"file_count": gorm.Expr("file_count + 1"),
		}).Error; err != nil {
			return err
		}
		// Update policy used capacity
		return tx.Model(&model.StoragePolicy{}).Where("id = ?", policy.ID).Update("used_bytes", gorm.Expr("used_bytes + ?", finalSize)).Error
	})

	if err != nil {
		s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("failed to save file record: %w", err)
	}

	return fileRecord, nil
}

func (s *UploadService) buildPublicPath(user *model.User, policy model.StoragePolicy, filename string) string {
	parts := []string{}
	renderedPrefix := s.renderPathPrefix(policy.PathPrefix, user)
	if renderedPrefix != "" {
		parts = append(parts, renderedPrefix)
	}

	parts = append(parts, filename)
	raw := strings.Join(parts, "/")
	return pathclean.CleanPath(raw)
}

func (s *UploadService) renderPathPrefix(prefix string, user *model.User) string {
	if prefix == "" {
		return ""
	}
	now := time.Now()
	res := prefix
	res = strings.ReplaceAll(res, "{username}", user.Username)
	res = strings.ReplaceAll(res, "{uid}", strconv.FormatUint(uint64(user.ID), 10))
	if user.Role != nil {
		res = strings.ReplaceAll(res, "{role}", user.Role.Name)
	}
	res = strings.ReplaceAll(res, "{year}", now.Format("2006"))
	res = strings.ReplaceAll(res, "{Y}", now.Format("2006"))
	res = strings.ReplaceAll(res, "{y}", now.Format("06"))
	res = strings.ReplaceAll(res, "{month}", now.Format("01"))
	res = strings.ReplaceAll(res, "{m}", now.Format("01"))
	res = strings.ReplaceAll(res, "{day}", now.Format("02"))
	res = strings.ReplaceAll(res, "{d}", now.Format("02"))
	res = strings.ReplaceAll(res, "{hour}", now.Format("15"))
	res = strings.ReplaceAll(res, "{H}", now.Format("15"))
	return strings.Trim(res, "/")
}

func (s *UploadService) renderNamingRule(rule, origName, ext, sha string) string {
	if rule == "" {
		rule = "{Y}/{m}/{d}/{random:16}.{ext}"
	}

	now := time.Now()
	rule = strings.ReplaceAll(rule, "{year}", now.Format("2006"))
	rule = strings.ReplaceAll(rule, "{Y}", now.Format("2006"))
	rule = strings.ReplaceAll(rule, "{y}", now.Format("06"))
	rule = strings.ReplaceAll(rule, "{month}", now.Format("01"))
	rule = strings.ReplaceAll(rule, "{m}", now.Format("01"))
	rule = strings.ReplaceAll(rule, "{day}", now.Format("02"))
	rule = strings.ReplaceAll(rule, "{d}", now.Format("02"))
	rule = strings.ReplaceAll(rule, "{hour}", now.Format("15"))
	rule = strings.ReplaceAll(rule, "{H}", now.Format("15"))
	rule = strings.ReplaceAll(rule, "{minute}", now.Format("04"))
	rule = strings.ReplaceAll(rule, "{i}", now.Format("04"))
	rule = strings.ReplaceAll(rule, "{second}", now.Format("05"))
	rule = strings.ReplaceAll(rule, "{s}", now.Format("05"))
	rule = strings.ReplaceAll(rule, "{timestamp}", strconv.FormatInt(now.Unix(), 10))
	rule = strings.ReplaceAll(rule, "{time}", strconv.FormatInt(now.Unix(), 10))
	rule = strings.ReplaceAll(rule, "{ext}", ext)

	cleanOrig := strings.TrimSuffix(filepath.Base(origName), filepath.Ext(origName))
	rule = strings.ReplaceAll(rule, "{filename}", cleanOrig)

	if len(sha) >= 16 {
		rule = strings.ReplaceAll(rule, "{sha1}", sha[:16])
		rule = strings.ReplaceAll(rule, "{md5}", sha[:16])
	}
	rule = strings.ReplaceAll(rule, "{sha256}", sha)

	// Handle {random} or {random:N}
	rule = strings.ReplaceAll(rule, "{random}", "{random:16}")
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
		if err := tx.Model(&model.User{}).Where("id = ?", file.UserID).Updates(map[string]any{
			"used_bytes": gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", totalSize, totalSize),
			"file_count": gorm.Expr("CASE WHEN file_count >= 1 THEN file_count - 1 ELSE 0 END"),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.StoragePolicy{}).Where("id = ?", file.PolicyID).Update("used_bytes", gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", totalSize, totalSize)).Error
	})
}

func (s *UploadService) SaveEditedFile(ctx context.Context, user *model.User, fileID uint, imgBytes []byte, mode string, newDisplayName string) (*model.FileRecord, error) {
	var file model.FileRecord
	query := s.db.Where("id = ?", fileID)
	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if !isAdmin {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		return nil, errors.New("file not found")
	}

	// Compute dimensions and sha256
	imgCfg, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	w, h := imgCfg.Width, imgCfg.Height
	if err != nil {
		w, h = file.Width, file.Height
	}

	hash := sha256.Sum256(imgBytes)
	newSha256 := hex.EncodeToString(hash[:])
	newSize := int64(len(imgBytes))

	if mode == "copy" {
		origName := file.DisplayName
		ext := filepath.Ext(origName)
		baseName := strings.TrimSuffix(origName, ext)
		copyName := newDisplayName
		if copyName == "" {
			copyName = fmt.Sprintf("%s_edited%s", baseName, ext)
		}
		return s.Upload(ctx, user, bytes.NewReader(imgBytes), copyName, newSize, UploadOptions{
			PolicyID:            file.PolicyID,
			DisplayName:         copyName,
			WatermarkMode:       file.WatermarkMode,
			WatermarkTemplateID: file.WatermarkTemplateID,
		})
	}

	// Overwrite original
	sizeDiff := newSize - file.Size

	// Check user storage quota limit
	var role model.Role
	if user.Role != nil {
		role = *user.Role
	} else {
		s.db.Where("id = ?", user.RoleID).First(&role)
	}
	if sizeDiff > 0 && role.QuotaBytes > 0 && user.UsedBytes+sizeDiff > role.QuotaBytes {
		return nil, fmt.Errorf("user storage quota exceeded: requires %d bytes, available %d bytes",
			sizeDiff, role.QuotaBytes-user.UsedBytes)
	}

	// Check storage policy capacity limit
	var policy model.StoragePolicy
	if err := s.db.Where("id = ?", file.PolicyID).First(&policy).Error; err == nil {
		if sizeDiff > 0 && policy.MaxCapacityBytes > 0 && policy.UsedBytes+sizeDiff > policy.MaxCapacityBytes {
			return nil, fmt.Errorf("storage policy '%s' capacity exceeded", policy.Name)
		}
	}

	if err := s.storage.Put(ctx, file.StorageKey, bytes.NewReader(imgBytes), newSize); err != nil {
		return nil, fmt.Errorf("failed to save edited image: %w", err)
	}

	// Detect actual MIME type
	detectedMime := mimetype.Detect(imgBytes).String()

	file.Size = newSize
	file.MimeType = detectedMime
	file.Width = w
	file.Height = h
	file.Sha256 = newSha256
	file.SignVersion++
	file.UpdatedAt = time.Now()

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&file).Error; err != nil {
			return err
		}
		if sizeDiff != 0 {
			if err := tx.Model(&model.User{}).Where("id = ?", file.UserID).Update("used_bytes", gorm.Expr("used_bytes + ?", sizeDiff)).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.StoragePolicy{}).Where("id = ?", file.PolicyID).Update("used_bytes", gorm.Expr("used_bytes + ?", sizeDiff)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	user.UsedBytes += sizeDiff
	return &file, nil
}
