package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	_ "golang.org/x/image/webp"
	image "image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
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
	db            *gorm.DB
	storage       storage.StorageDriver
	driverManager *storage.PolicyDriverManager
	processor     imaging.ImageProcessor
	cache         *cache.DerivedCache
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

func (s *UploadService) SetDriverManager(dm *storage.PolicyDriverManager) {
	s.driverManager = dm
}

func (s *UploadService) GetDriver(ctx context.Context, policyID uint) (storage.StorageDriver, error) {
	return s.getDriver(ctx, policyID)
}

func (s *UploadService) getDriver(ctx context.Context, policyID uint) (storage.StorageDriver, error) {
	if policyID > 0 && s.driverManager != nil {
		drv, _, err := s.driverManager.GetDriver(ctx, policyID)
		if err != nil {
			return nil, fmt.Errorf("storage driver unavailable for policy %d: %w", policyID, err)
		}
		return drv, nil
	}
	if s.storage != nil {
		return s.storage, nil
	}
	return nil, fmt.Errorf("storage driver unavailable for policy %d", policyID)
}

type UploadOptions struct {
	PolicyID            uint
	PolicyIDs           []uint
	DisplayName         string
	WatermarkMode       string // inherit / on / off
	WatermarkTemplateID *uint
	HotlinkMode         string // inherit / on / off
	PublicMode          string // inherit / public / private
}

func (s *UploadService) getMaxSingleFileSize() int64 {
	var setting model.Setting
	if err := s.db.Where("key = ?", "max_single_file_size").First(&setting).Error; err == nil {
		if mb, err := strconv.ParseInt(strings.TrimSpace(setting.Value), 10, 64); err == nil {
			if mb <= 0 {
				return 0 // 0 means unlimited
			}
			return mb * 1024 * 1024
		}
	}
	return 50 * 1024 * 1024 // 50MB default
}

// Upload processes and stores an incoming file stream in its original form via true disk streaming
func (s *UploadService) Upload(ctx context.Context, user *model.User, r io.Reader, originalFilename string, declaredSize int64, opts UploadOptions) (*model.FileRecord, error) {
	maxSingleFileSize := s.getMaxSingleFileSize()
	if maxSingleFileSize > 0 && declaredSize > maxSingleFileSize {
		return nil, fmt.Errorf("file size %d exceeds system single file limit of %d bytes", declaredSize, maxSingleFileSize)
	}

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

	// Determine max allowed bytes considering user role quota
	var role model.Role
	if user.Role != nil {
		role = *user.Role
	} else {
		s.db.Where("id = ?", user.RoleID).First(&role)
	}
	maxAllowedBytes := maxSingleFileSize
	if role.MaxSingleFileSizeMB > 0 {
		roleMaxBytes := int64(role.MaxSingleFileSizeMB) * 1024 * 1024
		if maxAllowedBytes <= 0 || roleMaxBytes < maxAllowedBytes {
			maxAllowedBytes = roleMaxBytes
		}
	}
	if maxAllowedBytes <= 0 {
		maxAllowedBytes = 100 * 1024 * 1024 * 1024 // 100GB safety ceiling when unlimited
	}
	if role.QuotaBytes > 0 {
		remainingUserQuota := role.QuotaBytes - user.UsedBytes
		if remainingUserQuota <= 0 {
			return nil, fmt.Errorf("user storage quota exceeded: current %d >= limit %d", user.UsedBytes, role.QuotaBytes)
		}
		if remainingUserQuota < maxAllowedBytes {
			maxAllowedBytes = remainingUserQuota
		}
	}

	// 2. Stream into a temporary disk file bounded by LimitReader while computing SHA-256 (Constant O(1) RAM)
	tempFile, err := os.CreateTemp("", "uimg-upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for upload: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hasher)

	// Write the initial header bytes
	if _, err := multiWriter.Write(headerBuf); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	remainingAllowed := maxAllowedBytes - int64(n)
	if remainingAllowed < 0 {
		return nil, fmt.Errorf("file exceeds maximum allowed size (%d bytes)", maxAllowedBytes)
	}

	// Stream remaining content bounded by remainingAllowed + 1
	limitedReader := io.LimitReader(r, remainingAllowed+1)
	copied, err := io.Copy(multiWriter, limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read full payload: %w", err)
	}
	actualSize := int64(n) + copied
	if maxSingleFileSize > 0 && actualSize > maxSingleFileSize {
		return nil, fmt.Errorf("file size %d exceeds allowed limit of %d bytes", actualSize, maxSingleFileSize)
	}
	if role.MaxSingleFileSizeMB > 0 && actualSize > int64(role.MaxSingleFileSizeMB)*1024*1024 {
		return nil, fmt.Errorf("file size %d exceeds role single file limit of %d MB", actualSize, role.MaxSingleFileSizeMB)
	}
	if role.QuotaBytes > 0 && user.UsedBytes+actualSize > role.QuotaBytes {
		return nil, fmt.Errorf("user storage quota exceeded: current %d + upload %d > limit %d",
			user.UsedBytes, actualSize, role.QuotaBytes)
	}
	fileSha256 := hex.EncodeToString(hasher.Sum(nil))

	// 4. Resolve Storage Policies permitted for user's Role
	var candidatePolicies []model.StoragePolicy
	requestedIDs := make([]uint, 0)
	if len(opts.PolicyIDs) > 0 {
		requestedIDs = append(requestedIDs, opts.PolicyIDs...)
	} else if opts.PolicyID > 0 {
		requestedIDs = append(requestedIDs, opts.PolicyID)
	}

	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")

	if len(requestedIDs) > 0 {
		if isAdmin {
			s.db.Where("id IN ?", requestedIDs).Find(&candidatePolicies)
		} else {
			var permittedIDs []uint
			s.db.Table("role_policies").
				Where("role_id = ? AND policy_id IN ?", user.RoleID, requestedIDs).
				Pluck("policy_id", &permittedIDs)
			if len(permittedIDs) > 0 {
				s.db.Where("id IN ?", permittedIDs).Find(&candidatePolicies)
			}
		}
		if len(candidatePolicies) == 0 {
			return nil, errors.New("无权使用指定的存储策略")
		}
	} else {
		// When no policies requested, find default policy for user's role
		var defaultPolicyID uint
		s.db.Table("role_policies").
			Joins("JOIN storage_policies ON storage_policies.id = role_policies.policy_id").
			Where("role_policies.role_id = ? AND storage_policies.is_default = 1", user.RoleID).
			Select("storage_policies.id").Scan(&defaultPolicyID)
		if defaultPolicyID > 0 {
			var p model.StoragePolicy
			if err := s.db.Where("id = ?", defaultPolicyID).First(&p).Error; err == nil {
				candidatePolicies = append(candidatePolicies, p)
			}
		}
		if len(candidatePolicies) == 0 {
			var firstPolicyID uint
			s.db.Table("role_policies").Where("role_id = ?", user.RoleID).Select("policy_id").Scan(&firstPolicyID)
			if firstPolicyID > 0 {
				var p model.StoragePolicy
				if err := s.db.Where("id = ?", firstPolicyID).First(&p).Error; err == nil {
					candidatePolicies = append(candidatePolicies, p)
				}
			}
		}
		if len(candidatePolicies) == 0 {
			if isAdmin {
				var p model.StoragePolicy
				s.db.Where("is_default = 1").First(&p)
				if p.ID == 0 {
					s.db.First(&p)
				}
				if p.ID > 0 {
					candidatePolicies = append(candidatePolicies, p)
				}
			} else {
				return nil, errors.New("当前角色未分配任何存储策略，请联系管理员分配")
			}
		}
	}
	if len(candidatePolicies) == 0 {
		return nil, errors.New("无可用存储策略")
	}

	// Filter policies passing single file size limit and capacity check
	finalSize := actualSize
	var validPolicies []model.StoragePolicy
	var policyErrors []string
	var failedPolicies []model.PolicyUploadFailure

	for _, p := range candidatePolicies {
		if p.MaxSingleFileSizeMB > 0 && finalSize > p.MaxSingleFileSizeMB*1024*1024 {
			errMsg := fmt.Sprintf("单文件大小超限（上限 %d MB）", p.MaxSingleFileSizeMB)
			policyErrors = append(policyErrors, fmt.Sprintf("策略「%s」%s", p.Name, errMsg))
			failedPolicies = append(failedPolicies, model.PolicyUploadFailure{
				PolicyID:   p.ID,
				PolicyName: p.Name,
				Error:      errMsg,
			})
			continue
		}
		if p.MaxCapacityBytes > 0 && p.UsedBytes+finalSize > p.MaxCapacityBytes {
			errMsg := "存储容量已满"
			policyErrors = append(policyErrors, fmt.Sprintf("策略「%s」%s", p.Name, errMsg))
			failedPolicies = append(failedPolicies, model.PolicyUploadFailure{
				PolicyID:   p.ID,
				PolicyName: p.Name,
				Error:      errMsg,
			})
			continue
		}
		validPolicies = append(validPolicies, p)
	}

	if len(validPolicies) == 0 {
		return nil, fmt.Errorf("所有指定策略均不可用: %s", strings.Join(policyErrors, "; "))
	}

	finalMime := detectedMime
	var imgW, imgH int
	if isImage && s.processor != nil {
		if _, err := tempFile.Seek(0, io.SeekStart); err == nil {
			w, h, _ := s.processor.GetDimensions(tempFile)
			imgW = w
			imgH = h
		}
	}

	type uploadSuccess struct {
		policy     model.StoragePolicy
		publicPath string
		storageKey string
	}
	var successes []uploadSuccess
	var lastErr error

	for _, p := range validPolicies {
		if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
			lastErr = err
			failedPolicies = append(failedPolicies, model.PolicyUploadFailure{
				PolicyID:   p.ID,
				PolicyName: p.Name,
				Error:      "读取临时数据失败",
			})
			continue
		}
		drv, err := s.getDriver(ctx, p.ID)
		if err != nil {
			lastErr = fmt.Errorf("策略「%s」初始化失败: %w", p.Name, err)
			failedPolicies = append(failedPolicies, model.PolicyUploadFailure{
				PolicyID:   p.ID,
				PolicyName: p.Name,
				Error:      fmt.Sprintf("初始化驱动失败: %v", err),
			})
			continue
		}
		renderedName := s.renderNamingRule(p.NamingRule, originalFilename, finalExt, fileSha256)
		pubPath := s.buildPublicPath(user, p, renderedName)
		storKey := pubPath

		if err := drv.Put(ctx, storKey, tempFile, finalSize); err != nil {
			slog.Warn("Failed to upload to storage policy", "policy_id", p.ID, "policy_name", p.Name, "error", err)
			lastErr = fmt.Errorf("策略「%s」上传失败: %w", p.Name, err)
			failedPolicies = append(failedPolicies, model.PolicyUploadFailure{
				PolicyID:   p.ID,
				PolicyName: p.Name,
				Error:      fmt.Sprintf("上传失败: %v", err),
			})
			continue
		}

		successes = append(successes, uploadSuccess{
			policy:     p,
			publicPath: pubPath,
			storageKey: storKey,
		})
	}

	if len(successes) == 0 {
		return nil, lastErr
	}

	// Total physical size across all successfully saved replicas
	totalAddedBytes := int64(len(successes)) * finalSize
	if role.QuotaBytes > 0 && user.UsedBytes+totalAddedBytes > role.QuotaBytes {
		// Clean up uploaded physical files
		for _, succ := range successes {
			if drv, err := s.getDriver(ctx, succ.policy.ID); err == nil {
				_ = drv.Delete(ctx, succ.storageKey)
			}
		}
		return nil, fmt.Errorf("用户存储配额超限（总物理占用 %d 超过限额 %d 字节）", user.UsedBytes+totalAddedBytes, role.QuotaBytes)
	}

	displayName := opts.DisplayName
	if displayName == "" {
		displayName = originalFilename
	}
	if opts.PublicMode == "" {
		opts.PublicMode = "inherit"
	}
	if opts.HotlinkMode == "" {
		opts.HotlinkMode = "inherit"
	}

	primary := successes[0]
	fileRecord := &model.FileRecord{
		UserID:              user.ID,
		PolicyID:            primary.policy.ID,
		PublicPath:          primary.publicPath,
		StorageKey:          primary.storageKey,
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
		HotlinkMode:         opts.HotlinkMode,
		PublicMode:          opts.PublicMode,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	var createdReplicas []*model.FilePolicyReplica
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if role.QuotaBytes > 0 {
			var currentUser model.User
			if err := tx.Select("used_bytes").Where("id = ?", user.ID).First(&currentUser).Error; err != nil {
				return err
			}
			if currentUser.UsedBytes+totalAddedBytes > role.QuotaBytes {
				return errors.New("user storage quota exceeded")
			}
		}

		if err := tx.Create(fileRecord).Error; err != nil {
			return err
		}

		for _, succ := range successes {
			polCopy := succ.policy
			replica := &model.FilePolicyReplica{
				FileID:     fileRecord.ID,
				PolicyID:   succ.policy.ID,
				PolicyName: succ.policy.Name,
				Policy:     &polCopy,
				PublicPath: succ.publicPath,
				StorageKey: succ.storageKey,
				Size:       finalSize,
				Status:     "ready",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if err := tx.Create(replica).Error; err != nil {
				return err
			}
			createdReplicas = append(createdReplicas, replica)

			polUpdate := tx.Model(&model.StoragePolicy{}).Where("id = ?", succ.policy.ID)
			if succ.policy.MaxCapacityBytes > 0 {
				polUpdate = polUpdate.Where("used_bytes + ? <= ?", finalSize, succ.policy.MaxCapacityBytes)
			}
			resPol := polUpdate.Update("used_bytes", gorm.Expr("used_bytes + ?", finalSize))
			if resPol.Error != nil {
				return resPol.Error
			}
			if succ.policy.MaxCapacityBytes > 0 && resPol.RowsAffected == 0 {
				return fmt.Errorf("storage policy '%s' capacity exceeded", succ.policy.Name)
			}
		}

		userUpdate := tx.Model(&model.User{}).Where("id = ?", user.ID)
		if role.QuotaBytes > 0 {
			userUpdate = userUpdate.Where("used_bytes + ? <= ?", totalAddedBytes, role.QuotaBytes)
		}
		resUser := userUpdate.Updates(map[string]any{
			"used_bytes": gorm.Expr("used_bytes + ?", totalAddedBytes),
			"file_count": gorm.Expr("file_count + 1"),
		})
		if resUser.Error != nil {
			return resUser.Error
		}
		if resUser.RowsAffected == 0 {
			return errors.New("user storage quota exceeded")
		}
		return nil
	})

	if err != nil {
		for _, succ := range successes {
			if drv, dErr := s.getDriver(ctx, succ.policy.ID); dErr == nil {
				_ = drv.Delete(ctx, succ.storageKey)
			}
		}
		return nil, fmt.Errorf("failed to save file record: %w", err)
	}

	user.UsedBytes += totalAddedBytes
	fileRecord.Replicas = createdReplicas
	fileRecord.FailedPolicies = failedPolicies
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
	clean := pathclean.CleanPath(raw)

	// Ensure public path does not shadow top-level system reserved routes
	firstSegment := clean
	if idx := strings.Index(clean, "/"); idx != -1 {
		firstSegment = clean[:idx]
	}
	if pathclean.IsReservedWord(firstSegment) {
		clean = "f/" + clean
	}

	return clean
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

	cleanOrig := strings.TrimSuffix(filepath.Base(origName), filepath.Ext(origName))
	rule = strings.ReplaceAll(rule, "{filename}", cleanOrig)

	return rule
}

type DeleteOptions struct {
	DeleteRemote bool
	ForceDB      bool
}

type RemoteDeleteError struct {
	Errors []string
}

func (e *RemoteDeleteError) Error() string {
	return strings.Join(e.Errors, "; ")
}

func (s *UploadService) DeleteFile(ctx context.Context, fileID, userID uint, isAdmin bool, opts ...DeleteOptions) error {
	opt := DeleteOptions{
		DeleteRemote: true,
		ForceDB:      false,
	}
	if len(opts) > 0 {
		opt = opts[0]
	}

	var file model.FileRecord
	query := s.db.Where("id = ?", fileID)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&file).Error; err != nil {
		return errors.New("file not found")
	}

	var replicas []model.FilePolicyReplica
	s.db.Preload("Policy").Where("file_id = ?", file.ID).Find(&replicas)

	policyDeductions := make(map[uint]int64)
	var totalDeduction int64
	var remoteErrors []string

	for _, rep := range replicas {
		if opt.DeleteRemote {
			if drv, err := s.getDriver(ctx, rep.PolicyID); err == nil {
				if err := drv.Delete(ctx, rep.StorageKey); err != nil {
					polName := rep.PolicyName
					if polName == "" && rep.Policy != nil {
						polName = rep.Policy.Name
					}
					slog.Warn("Failed to delete physical file from replica", "policy_id", rep.PolicyID, "storage_key", rep.StorageKey, "error", err)
					remoteErrors = append(remoteErrors, fmt.Sprintf("策略「%s」删除物理文件失败: %v", polName, err))
				}
			} else {
				remoteErrors = append(remoteErrors, fmt.Sprintf("策略驱动获取失败: %v", err))
			}
		}
		policyDeductions[rep.PolicyID] += rep.Size
		totalDeduction += rep.Size
	}

	if len(replicas) == 0 {
		if opt.DeleteRemote {
			if drv, err := s.getDriver(ctx, file.PolicyID); err == nil {
				if err := drv.Delete(ctx, file.StorageKey); err != nil {
					remoteErrors = append(remoteErrors, fmt.Sprintf("主存储删除物理文件失败: %v", err))
				}
			}
		}
		totalDeduction = file.Size
		policyDeductions[file.PolicyID] += file.Size
	}

	if file.SourceKey != "" {
		if opt.DeleteRemote {
			if drv, err := s.getDriver(ctx, file.PolicyID); err == nil {
				if err := drv.Delete(ctx, file.SourceKey); err != nil {
					remoteErrors = append(remoteErrors, fmt.Sprintf("源文件删除失败: %v", err))
				}
			}
		}
		totalDeduction += file.SourceSize
		policyDeductions[file.PolicyID] += file.SourceSize
	}

	if len(remoteErrors) > 0 && !opt.ForceDB {
		return &RemoteDeleteError{Errors: remoteErrors}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Unscoped().Delete(&file)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("file not found or already deleted")
		}
		if err := tx.Where("file_id = ?", file.ID).Delete(&model.FilePolicyReplica{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", file.UserID).Updates(map[string]any{
			"used_bytes": gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", totalDeduction, totalDeduction),
			"file_count": gorm.Expr("CASE WHEN file_count >= 1 THEN file_count - 1 ELSE 0 END"),
		}).Error; err != nil {
			return err
		}
		for pid, size := range policyDeductions {
			if err := tx.Model(&model.StoragePolicy{}).Where("id = ?", pid).Update("used_bytes", gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", size, size)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SyncFileToPolicies backs up an existing file into one or more target storage policies
func (s *UploadService) SyncFileToPolicies(ctx context.Context, user *model.User, fileID uint, targetPolicyIDs []uint, isAdmin bool) ([]*model.FilePolicyReplica, error) {
	var file model.FileRecord
	query := s.db.Where("id = ?", fileID)
	if !isAdmin {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		return nil, errors.New("file not found")
	}

	var existingReplicas []model.FilePolicyReplica
	s.db.Where("file_id = ?", file.ID).Find(&existingReplicas)
	existingMap := make(map[uint]bool)
	for _, r := range existingReplicas {
		existingMap[r.PolicyID] = true
	}

	var toAddIDs []uint
	for _, pid := range targetPolicyIDs {
		if !existingMap[pid] {
			toAddIDs = append(toAddIDs, pid)
		}
	}
	if len(toAddIDs) == 0 {
		return nil, nil
	}

	var targetPolicies []model.StoragePolicy
	if !isAdmin {
		var permittedIDs []uint
		s.db.Table("role_policies").Where("role_id = ? AND policy_id IN ?", user.RoleID, toAddIDs).Pluck("policy_id", &permittedIDs)
		if len(permittedIDs) > 0 {
			s.db.Where("id IN ?", permittedIDs).Find(&targetPolicies)
		}
	} else {
		s.db.Where("id IN ?", toAddIDs).Find(&targetPolicies)
	}

	if len(targetPolicies) == 0 {
		return nil, errors.New("no permitted target storage policy available")
	}

	var validPolicies []model.StoragePolicy
	for _, p := range targetPolicies {
		if p.MaxSingleFileSizeMB > 0 && file.Size > p.MaxSingleFileSizeMB*1024*1024 {
			continue
		}
		if p.MaxCapacityBytes > 0 && p.UsedBytes+file.Size > p.MaxCapacityBytes {
			continue
		}
		validPolicies = append(validPolicies, p)
	}
	if len(validPolicies) == 0 {
		return nil, errors.New("target storage policies cannot accommodate file size or capacity")
	}

	var srcReader io.ReadCloser
	var srcErr error
	if drv, err := s.getDriver(ctx, file.PolicyID); err == nil {
		srcReader, srcErr = drv.Get(ctx, file.StorageKey)
	}
	if srcErr != nil || srcReader == nil {
		for _, rep := range existingReplicas {
			if drv, err := s.getDriver(ctx, rep.PolicyID); err == nil {
				srcReader, srcErr = drv.Get(ctx, rep.StorageKey)
				if srcErr == nil && srcReader != nil {
					break
				}
			}
		}
	}
	if srcReader == nil {
		return nil, fmt.Errorf("failed to read source file from existing storage replicas: %v", srcErr)
	}
	defer srcReader.Close()

	tmpF, err := os.CreateTemp("", "uimg-sync-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = tmpF.Close()
		_ = os.Remove(tmpF.Name())
	}()

	if _, err := io.Copy(tmpF, srcReader); err != nil {
		return nil, fmt.Errorf("failed to buffer source file: %w", err)
	}

	var fileOwner model.User
	if file.UserID == user.ID {
		fileOwner = *user
	} else {
		if err := s.db.Preload("Role").Where("id = ?", file.UserID).First(&fileOwner).Error; err != nil {
			fileOwner = *user
		}
	}
	if fileOwner.Role != nil && fileOwner.Role.QuotaBytes > 0 && fileOwner.UsedBytes+int64(len(validPolicies))*file.Size > fileOwner.Role.QuotaBytes {
		return nil, fmt.Errorf("user storage quota exceeded")
	}

	ext := strings.TrimPrefix(filepath.Ext(file.PublicPath), ".")
	var createdReplicas []*model.FilePolicyReplica
	var totalSuccessBytes int64

	for _, p := range validPolicies {
		if _, err := tmpF.Seek(0, io.SeekStart); err != nil {
			continue
		}
		drv, err := s.getDriver(ctx, p.ID)
		if err != nil {
			continue
		}

		renderedName := s.renderNamingRule(p.NamingRule, file.DisplayName, ext, file.Sha256)
		pubPath := s.buildPublicPath(&fileOwner, p, renderedName)
		storKey := pubPath

		if err := drv.Put(ctx, storKey, tmpF, file.Size); err != nil {
			slog.Warn("Failed to sync file to target policy", "file_id", file.ID, "policy_id", p.ID, "error", err)
			continue
		}

		polCopy := p
		replica := &model.FilePolicyReplica{
			FileID:     file.ID,
			PolicyID:   p.ID,
			PolicyName: p.Name,
			Policy:     &polCopy,
			PublicPath: pubPath,
			StorageKey: storKey,
			Size:       file.Size,
			Status:     "ready",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		err = s.db.Transaction(func(tx *gorm.DB) error {
			if fileOwner.Role != nil && fileOwner.Role.QuotaBytes > 0 {
				var curUser model.User
				if err := tx.Select("used_bytes").Where("id = ?", file.UserID).First(&curUser).Error; err != nil {
					return err
				}
				if curUser.UsedBytes+file.Size > fileOwner.Role.QuotaBytes {
					return errors.New("user storage quota exceeded")
				}
			}
			if err := tx.Create(replica).Error; err != nil {
				return err
			}
			polUp := tx.Model(&model.StoragePolicy{}).Where("id = ?", p.ID)
			if p.MaxCapacityBytes > 0 {
				polUp = polUp.Where("used_bytes + ? <= ?", file.Size, p.MaxCapacityBytes)
			}
			resPol := polUp.Update("used_bytes", gorm.Expr("used_bytes + ?", file.Size))
			if resPol.Error != nil {
				return resPol.Error
			}
			if p.MaxCapacityBytes > 0 && resPol.RowsAffected == 0 {
				return fmt.Errorf("storage policy '%s' capacity exceeded", p.Name)
			}
			userUp := tx.Model(&model.User{}).Where("id = ?", file.UserID)
			if fileOwner.Role != nil && fileOwner.Role.QuotaBytes > 0 {
				userUp = userUp.Where("used_bytes + ? <= ?", file.Size, fileOwner.Role.QuotaBytes)
			}
			resU := userUp.Update("used_bytes", gorm.Expr("used_bytes + ?", file.Size))
			if resU.Error != nil {
				return resU.Error
			}
			if resU.RowsAffected == 0 {
				return errors.New("user storage quota exceeded")
			}
			return nil
		})

		if err != nil {
			_ = drv.Delete(ctx, storKey)
			continue
		}

		createdReplicas = append(createdReplicas, replica)
		totalSuccessBytes += file.Size
	}

	if len(createdReplicas) == 0 {
		return nil, errors.New("failed to sync file to target storage policies")
	}

	user.UsedBytes += totalSuccessBytes
	return createdReplicas, nil
}

// RemoveFileFromPolicy removes a single replica from a specified storage policy
func (s *UploadService) RemoveFileFromPolicy(ctx context.Context, user *model.User, fileID uint, policyID uint, isAdmin bool, opts ...DeleteOptions) error {
	opt := DeleteOptions{
		DeleteRemote: true,
		ForceDB:      false,
	}
	if len(opts) > 0 {
		opt = opts[0]
	}

	var file model.FileRecord
	query := s.db.Where("id = ?", fileID)
	if !isAdmin {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		return errors.New("file not found")
	}

	var targetReplica model.FilePolicyReplica
	if err := s.db.Where("file_id = ? AND policy_id = ?", file.ID, policyID).First(&targetReplica).Error; err != nil {
		return nil
	}

	storageKeyToDelete := targetReplica.StorageKey

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var currentReplicas []model.FilePolicyReplica
		if err := tx.Where("file_id = ?", file.ID).Find(&currentReplicas).Error; err != nil {
			return err
		}

		if len(currentReplicas) <= 1 {
			return errors.New("该存储副本为文件唯一备份，不可移除。如需销毁文件请使用删除操作")
		}

		var repToDelete *model.FilePolicyReplica
		var otherReplica *model.FilePolicyReplica
		for i := range currentReplicas {
			if currentReplicas[i].PolicyID == policyID {
				repToDelete = &currentReplicas[i]
			} else if otherReplica == nil {
				otherReplica = &currentReplicas[i]
			}
		}

		if repToDelete == nil {
			return nil
		}

		if err := tx.Delete(repToDelete).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", file.UserID).Update("used_bytes", gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", repToDelete.Size, repToDelete.Size)).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.StoragePolicy{}).Where("id = ?", policyID).Update("used_bytes", gorm.Expr("CASE WHEN used_bytes >= ? THEN used_bytes - ? ELSE 0 END", repToDelete.Size, repToDelete.Size)).Error; err != nil {
			return err
		}

		if file.PolicyID == policyID && otherReplica != nil {
			return tx.Model(&file).Updates(map[string]any{
				"policy_id":   otherReplica.PolicyID,
				"public_path": otherReplica.PublicPath,
				"storage_key": otherReplica.StorageKey,
			}).Error
		}
		return nil
	})
	if err != nil {
		return err
	}

	if opt.DeleteRemote && storageKeyToDelete != "" {
		if drv, err := s.getDriver(ctx, policyID); err == nil {
			if err := drv.Delete(ctx, storageKeyToDelete); err != nil {
				slog.Warn("Failed to delete physical replica from storage driver", "policy_id", policyID, "storage_key", storageKeyToDelete, "error", err)
				if !opt.ForceDB {
					return &RemoteDeleteError{Errors: []string{fmt.Sprintf("远程存储删除物理副本失败: %v", err)}}
				}
			}
		}
	}

	return nil
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
	if err != nil {
		return nil, fmt.Errorf("invalid image content: %w", err)
	}
	w, h := imgCfg.Width, imgCfg.Height

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
			HotlinkMode:         file.HotlinkMode,
			PublicMode:          file.PublicMode,
		})
	}

	// Overwrite original
	sizeDiff := newSize - file.Size

	var fileOwner model.User
	if file.UserID == user.ID {
		fileOwner = *user
	} else {
		if err := s.db.Preload("Role").Where("id = ?", file.UserID).First(&fileOwner).Error; err != nil {
			fileOwner = *user
		}
	}
	var role model.Role
	if fileOwner.Role != nil {
		role = *fileOwner.Role
	} else {
		s.db.Where("id = ?", fileOwner.RoleID).First(&role)
	}
	if role.MaxSingleFileSizeMB > 0 && newSize > int64(role.MaxSingleFileSizeMB)*1024*1024 {
		return nil, fmt.Errorf("edited image size exceeds role single file limit (%d MB)", role.MaxSingleFileSizeMB)
	}
	if sizeDiff > 0 && role.QuotaBytes > 0 && fileOwner.UsedBytes+sizeDiff > role.QuotaBytes {
		return nil, fmt.Errorf("user storage quota exceeded: requires %d bytes, available %d bytes",
			sizeDiff, role.QuotaBytes-fileOwner.UsedBytes)
	}

	// Check storage policy capacity limit for all existing ready replicas
	var replicas []model.FilePolicyReplica
	s.db.Where("file_id = ? AND status = 'ready'", file.ID).Find(&replicas)
	replicaPolicyIDs := make(map[uint]bool)
	replicaPolicyIDs[file.PolicyID] = true
	for _, rep := range replicas {
		replicaPolicyIDs[rep.PolicyID] = true
	}

	if sizeDiff > 0 {
		for pid := range replicaPolicyIDs {
			var p model.StoragePolicy
			if err := s.db.Where("id = ?", pid).First(&p).Error; err == nil {
				if p.MaxCapacityBytes > 0 && p.UsedBytes+sizeDiff > p.MaxCapacityBytes {
					return nil, fmt.Errorf("storage policy '%s' capacity exceeded", p.Name)
				}
			}
		}
	}

	drv, err := s.getDriver(ctx, file.PolicyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage driver: %w", err)
	}

	if err := drv.Put(ctx, file.StorageKey, bytes.NewReader(imgBytes), newSize); err != nil {
		return nil, fmt.Errorf("failed to save edited image: %w", err)
	}

	// Update any other replicas with new content
	for _, rep := range replicas {
		if rep.PolicyID != file.PolicyID {
			if rDrv, err := s.getDriver(ctx, rep.PolicyID); err == nil {
				if err := rDrv.Put(ctx, rep.StorageKey, bytes.NewReader(imgBytes), newSize); err != nil {
					slog.Warn("Failed to update replica during file edit", "replica_id", rep.ID, "policy_id", rep.PolicyID, "error", err)
					_ = s.db.Model(&rep).Updates(map[string]any{"status": "failed", "error_msg": err.Error()})
					delete(replicaPolicyIDs, rep.PolicyID)
				}
			}
		}
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
		if sizeDiff > 0 {
			if role.QuotaBytes > 0 {
				var currentUser model.User
				if err := tx.Select("used_bytes").Where("id = ?", file.UserID).First(&currentUser).Error; err != nil {
					return err
				}
				if currentUser.UsedBytes+sizeDiff > role.QuotaBytes {
					return errors.New("user storage quota exceeded")
				}
			}
		}

		if err := tx.Save(&file).Error; err != nil {
			return err
		}
		_ = tx.Model(&model.FilePolicyReplica{}).
			Where("file_id = ? AND status = 'ready'", file.ID).
			Updates(map[string]any{"size": newSize, "updated_at": time.Now()})
		if sizeDiff != 0 {
			if err := tx.Model(&model.User{}).Where("id = ?", file.UserID).Update("used_bytes", gorm.Expr("used_bytes + ?", sizeDiff)).Error; err != nil {
				return err
			}
			for pid := range replicaPolicyIDs {
				polUp := tx.Model(&model.StoragePolicy{}).Where("id = ?", pid)
				if sizeDiff > 0 {
					var pol model.StoragePolicy
					if err := tx.Where("id = ?", pid).First(&pol).Error; err == nil && pol.MaxCapacityBytes > 0 {
						polUp = polUp.Where("used_bytes + ? <= ?", sizeDiff, pol.MaxCapacityBytes)
					}
				}
				resPol := polUp.Update("used_bytes", gorm.Expr("CASE WHEN used_bytes + ? >= 0 THEN used_bytes + ? ELSE 0 END", sizeDiff, sizeDiff))
				if resPol.Error != nil {
					return resPol.Error
				}
				if sizeDiff > 0 && resPol.RowsAffected == 0 {
					return fmt.Errorf("storage policy capacity exceeded")
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if file.UserID == user.ID {
		user.UsedBytes += sizeDiff
	}
	return &file, nil
}
