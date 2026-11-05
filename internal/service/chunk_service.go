package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

type ChunkUploadMeta struct {
	UploadID            string    `json:"upload_id"`
	UserID              uint      `json:"user_id"`
	Filename            string    `json:"filename"`
	TotalSize           int64     `json:"total_size"`
	ChunkSize           int64     `json:"chunk_size"`
	TotalChunks         int       `json:"total_chunks"`
	FileSHA256          string    `json:"file_sha256"`
	UploadType          string    `json:"upload_type"` // "file" or "archive"
	PolicyID            uint      `json:"policy_id"`
	PolicyIDs           []uint    `json:"policy_ids,omitempty"`
	WatermarkMode       string    `json:"watermark_mode"`
	WatermarkTemplateID *uint     `json:"watermark_template_id"`
	HotlinkMode         string    `json:"hotlink_mode"`
	PublicMode          string    `json:"public_mode"`
	PreservePath        bool      `json:"preserve_path"`
	StripTopDir         bool      `json:"strip_top_dir"`
	ConflictAction      string    `json:"conflict_action"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ArchiveImportResult struct {
	Total    int `json:"total"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

type ChunkService struct {
	db               *gorm.DB
	cacheDir         string
	uploadService    *UploadService
	importService    *ImportService
	lock             sync.Mutex
	activeAssemblyMu sync.Mutex
	activeAssemblies map[string]bool
	stopCleaner      chan struct{}
}

func NewChunkService(
	db *gorm.DB,
	cacheDir string,
	uploadService *UploadService,
	importService *ImportService,
) *ChunkService {
	cleanCache := filepath.Clean(cacheDir)
	if cleanCache == "" || cleanCache == "." {
		cleanCache = "cache"
	}
	chunksDir := filepath.Join(cleanCache, "chunks")
	_ = os.MkdirAll(chunksDir, 0755)

	svc := &ChunkService{
		db:               db,
		cacheDir:         cleanCache,
		uploadService:    uploadService,
		importService:    importService,
		activeAssemblies: make(map[string]bool),
		stopCleaner:      make(chan struct{}),
	}

	// Start background cleaner for abandoned chunks (older than 24h)
	go svc.startCleanupWorker()

	return svc
}

func (s *ChunkService) SetDB(db *gorm.DB) {
	s.db = db
}

func (s *ChunkService) Stop() {
	select {
	case <-s.stopCleaner:
	default:
		close(s.stopCleaner)
	}
}

func (s *ChunkService) startCleanupWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Initial cleanup on boot
	s.PruneExpiredChunks(24 * time.Hour)

	for {
		select {
		case <-s.stopCleaner:
			return
		case <-ticker.C:
			s.PruneExpiredChunks(24 * time.Hour)
		}
	}
}

// PruneExpiredChunks removes chunk directories and assembled files older than maxAge
func (s *ChunkService) PruneExpiredChunks(maxAge time.Duration) {
	chunksBase := filepath.Join(s.cacheDir, "chunks")
	entries, err := os.ReadDir(chunksBase)
	if err != nil {
		return
	}

	now := time.Now()
	for _, entry := range entries {
		fullPath := filepath.Join(chunksBase, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			slog.Info("Pruning expired chunk upload in cache", "path", fullPath, "age", now.Sub(info.ModTime()).String())
			_ = os.RemoveAll(fullPath)
		}
	}
}

func isValidUploadID(id string) bool {
	id = strings.TrimSpace(id)
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func (s *ChunkService) getUploadDir(uploadID string) string {
	cleanID := filepath.Base(strings.TrimSpace(uploadID))
	if !isValidUploadID(cleanID) {
		return ""
	}
	return filepath.Join(s.cacheDir, "chunks", cleanID)
}

func (s *ChunkService) getMetaPath(uploadID string) string {
	dir := s.getUploadDir(uploadID)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "meta.json")
}

func (s *ChunkService) readMeta(uploadID string) (*ChunkUploadMeta, error) {
	if !isValidUploadID(uploadID) {
		return nil, errors.New("invalid upload_id")
	}
	metaPath := s.getMetaPath(uploadID)
	if metaPath == "" {
		return nil, errors.New("invalid upload_id")
	}
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("chunk upload task not found or expired: %w", err)
	}
	var meta ChunkUploadMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse chunk upload meta: %w", err)
	}
	return &meta, nil
}

const (
	MaxGlobalChunkCacheBytes = 10 * 1024 * 1024 * 1024 // 10 GB limit for in-progress chunk uploads
)

func (s *ChunkService) getGlobalChunksCacheSize() int64 {
	chunksBase := filepath.Join(s.cacheDir, "chunks")
	var totalSize int64
	entries, err := os.ReadDir(chunksBase)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if entry.IsDir() {
			subEntries, err := os.ReadDir(filepath.Join(chunksBase, entry.Name()))
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if subInfo, err := sub.Info(); err == nil {
					totalSize += subInfo.Size()
				}
			}
		} else {
			totalSize += info.Size()
		}
	}
	return totalSize
}

func (s *ChunkService) saveMeta(meta *ChunkUploadMeta) error {
	dir := s.getUploadDir(meta.UploadID)
	if dir == "" {
		return errors.New("invalid upload_id")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create chunk directory in cache: %w", err)
	}
	meta.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	metaPath := s.getMetaPath(meta.UploadID)
	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, metaPath)
}

// InitChunkUpload initializes a new chunk upload task
func (s *ChunkService) InitChunkUpload(user *model.User, meta ChunkUploadMeta) (*ChunkUploadMeta, []int, error) {
	if meta.TotalChunks <= 0 {
		return nil, nil, fmt.Errorf("total_chunks must be greater than 0")
	}
	if meta.TotalChunks > 2000 {
		return nil, nil, fmt.Errorf("total_chunks exceeds maximum allowed limit of 2000")
	}
	if meta.TotalSize <= 0 {
		return nil, nil, fmt.Errorf("total_size must be greater than 0")
	}
	if meta.Filename == "" {
		return nil, nil, fmt.Errorf("filename cannot be empty")
	}

	if meta.TotalChunks == 1 {
		if meta.ChunkSize <= 0 {
			meta.ChunkSize = meta.TotalSize
		}
		if meta.TotalSize > meta.ChunkSize {
			return nil, nil, fmt.Errorf("total_size (%d) exceeds chunk_size (%d)", meta.TotalSize, meta.ChunkSize)
		}
	} else {
		if meta.ChunkSize < 1024*1024 || meta.ChunkSize > 50*1024*1024 {
			return nil, nil, fmt.Errorf("chunk_size must be between 1MB and 50MB (got %d bytes)", meta.ChunkSize)
		}
		if int64(meta.TotalChunks-1)*meta.ChunkSize >= meta.TotalSize || meta.TotalSize > int64(meta.TotalChunks)*meta.ChunkSize {
			return nil, nil, fmt.Errorf("chunk parameters mathematically inconsistent: total_size (%d) not in range ((%d-1)*%d, %d*%d]",
				meta.TotalSize, meta.TotalChunks, meta.ChunkSize, meta.TotalChunks, meta.ChunkSize)
		}
	}

	// Check global cache limit
	if s.getGlobalChunksCacheSize()+meta.TotalSize > MaxGlobalChunkCacheBytes {
		return nil, nil, fmt.Errorf("chunk storage cache capacity exceeded (limit 10GB), please retry later")
	}

	// Validate role restrictions & user quota
	var role model.Role
	if user.Role != nil {
		role = *user.Role
	} else if s.db != nil {
		s.db.Where("id = ?", user.RoleID).First(&role)
	}

	if role.MaxSingleFileSizeMB > 0 {
		maxBytes := int64(role.MaxSingleFileSizeMB) * 1024 * 1024
		if meta.TotalSize > maxBytes {
			return nil, nil, fmt.Errorf("file size (%d bytes) exceeds role limit (%d bytes)", meta.TotalSize, maxBytes)
		}
	}
	if role.QuotaBytes > 0 && user.UsedBytes+meta.TotalSize > role.QuotaBytes {
		return nil, nil, fmt.Errorf("upload would exceed user storage quota")
	}

	// Limit active chunk uploads per user (max 10 active tasks)
	chunksBase := filepath.Join(s.cacheDir, "chunks")
	if entries, err := os.ReadDir(chunksBase); err == nil {
		activeCount := 0
		for _, e := range entries {
			if e.IsDir() {
				if m, err := s.readMeta(e.Name()); err == nil && m.UserID == user.ID {
					activeCount++
				}
			}
		}
		if activeCount >= 10 {
			return nil, nil, fmt.Errorf("active chunk upload tasks limit reached (max 10), please complete or cancel previous tasks")
		}
	}

	// Generate random 16-byte hex upload_id
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	uploadID := hex.EncodeToString(b)

	meta.UploadID = uploadID
	meta.UserID = user.ID
	meta.CreatedAt = time.Now()
	meta.UpdatedAt = time.Now()
	if meta.UploadType == "" {
		meta.UploadType = "file"
	}
	if meta.PublicMode == "" {
		meta.PublicMode = "inherit"
	}

	if err := s.saveMeta(&meta); err != nil {
		return nil, nil, err
	}

	return &meta, []int{}, nil
}

// SaveChunkPart writes an incoming chunk binary part to cache
func (s *ChunkService) SaveChunkPart(user *model.User, uploadID string, chunkIndex int, r io.Reader) error {
	meta, err := s.readMeta(uploadID)
	if err != nil {
		return err
	}

	if meta.UserID != user.ID {
		return fmt.Errorf("permission denied: upload task belongs to another user")
	}

	if chunkIndex < 0 || chunkIndex >= meta.TotalChunks {
		return fmt.Errorf("chunk_index out of bounds (0 <= index < %d)", meta.TotalChunks)
	}

	uploadDir := s.getUploadDir(uploadID)
	chunkPath := filepath.Join(uploadDir, fmt.Sprintf("%05d.chunk", chunkIndex))
	tmpChunkPath := chunkPath + ".tmp"

	var expectedPartSize int64
	if chunkIndex < meta.TotalChunks-1 {
		expectedPartSize = meta.ChunkSize
	} else {
		expectedPartSize = meta.TotalSize - int64(meta.TotalChunks-1)*meta.ChunkSize
	}
	if expectedPartSize <= 0 {
		return fmt.Errorf("invalid chunk configuration for index %d", chunkIndex)
	}

	// Cumulative size check in this upload directory
	var currentDirBytes int64
	if entries, err := os.ReadDir(uploadDir); err == nil {
		for _, e := range entries {
			if e.Name() != fmt.Sprintf("%05d.chunk", chunkIndex) && strings.HasSuffix(e.Name(), ".chunk") {
				if info, err := e.Info(); err == nil {
					currentDirBytes += info.Size()
				}
			}
		}
	}
	if currentDirBytes+expectedPartSize > meta.TotalSize {
		return fmt.Errorf("cumulative chunks size would exceed total file size (%d bytes)", meta.TotalSize)
	}

	f, err := os.OpenFile(tmpChunkPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create chunk file in cache: %w", err)
	}

	boundedReader := io.LimitReader(r, expectedPartSize+1)
	written, err := io.Copy(f, boundedReader)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmpChunkPath)
		return fmt.Errorf("failed to write chunk data: %w", err)
	}
	if written != expectedPartSize {
		_ = os.Remove(tmpChunkPath)
		return fmt.Errorf("chunk part size mismatch: expected %d bytes, got %d bytes", expectedPartSize, written)
	}

	if err := os.Rename(tmpChunkPath, chunkPath); err != nil {
		_ = os.Remove(tmpChunkPath)
		return fmt.Errorf("failed to finalize chunk file: %w", err)
	}

	_ = s.saveMeta(meta)
	return nil
}

// GetChunkStatus returns list of uploaded chunks for resuming
func (s *ChunkService) GetChunkStatus(user *model.User, uploadID string) (*ChunkUploadMeta, []int, error) {
	meta, err := s.readMeta(uploadID)
	if err != nil {
		return nil, nil, err
	}

	if meta.UserID != user.ID {
		return nil, nil, fmt.Errorf("permission denied")
	}

	dir := s.getUploadDir(uploadID)
	var uploaded []int
	for i := 0; i < meta.TotalChunks; i++ {
		chunkPath := filepath.Join(dir, fmt.Sprintf("%05d.chunk", i))
		if info, err := os.Stat(chunkPath); err == nil && info.Size() > 0 {
			uploaded = append(uploaded, i)
		}
	}

	return meta, uploaded, nil
}

// AbortChunkUpload cleans up cache directory and cancels task
func (s *ChunkService) AbortChunkUpload(user *model.User, uploadID string) error {
	if !isValidUploadID(uploadID) {
		return errors.New("invalid upload_id")
	}
	meta, err := s.readMeta(uploadID)
	if err != nil {
		return err
	}
	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if !isAdmin && meta.UserID != user.ID {
		return fmt.Errorf("permission denied")
	}

	dir := s.getUploadDir(uploadID)
	if dir == "" {
		return errors.New("invalid upload directory")
	}
	chunksRoot := filepath.Clean(filepath.Join(s.cacheDir, "chunks"))
	cleanDir := filepath.Clean(dir)
	if cleanDir == chunksRoot || cleanDir == filepath.Clean(s.cacheDir) || !strings.HasPrefix(cleanDir, chunksRoot+string(filepath.Separator)) {
		return errors.New("invalid chunk directory path")
	}

	assembledPath := filepath.Join(s.cacheDir, "chunks", uploadID+".assembled")

	_ = os.RemoveAll(cleanDir)
	_ = os.Remove(assembledPath)
	slog.Info("Chunk upload task aborted, cache cleared", "upload_id", uploadID)
	return nil
}

// CompleteChunkUpload merges all chunks in cache, checks SHA-256, and dispatches to uploadService or importService
func (s *ChunkService) CompleteChunkUpload(ctx context.Context, user *model.User, uploadID string) (*model.FileRecord, *ArchiveImportResult, error) {
	s.activeAssemblyMu.Lock()
	if s.activeAssemblies[uploadID] {
		s.activeAssemblyMu.Unlock()
		return nil, nil, fmt.Errorf("chunk upload assembly already in progress")
	}
	s.activeAssemblies[uploadID] = true
	s.activeAssemblyMu.Unlock()

	defer func() {
		s.activeAssemblyMu.Lock()
		delete(s.activeAssemblies, uploadID)
		s.activeAssemblyMu.Unlock()
	}()

	meta, err := s.readMeta(uploadID)
	if err != nil {
		return nil, nil, err
	}

	if meta.UserID != user.ID {
		return nil, nil, fmt.Errorf("permission denied")
	}

	dir := s.getUploadDir(uploadID)

	// Verify all chunks exist
	for i := 0; i < meta.TotalChunks; i++ {
		chunkPath := filepath.Join(dir, fmt.Sprintf("%05d.chunk", i))
		if _, err := os.Stat(chunkPath); err != nil {
			return nil, nil, fmt.Errorf("chunk #%d is missing", i)
		}
	}

	// Create assembled file in cache
	assembledPath := filepath.Join(s.cacheDir, "chunks", uploadID+".assembled")
	assembledFile, err := os.OpenFile(assembledPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create assembled file in cache: %w", err)
	}

	hasher := sha256.New()
	mw := io.MultiWriter(assembledFile, hasher)

	for i := 0; i < meta.TotalChunks; i++ {
		chunkPath := filepath.Join(dir, fmt.Sprintf("%05d.chunk", i))
		cf, err := os.Open(chunkPath)
		if err != nil {
			_ = assembledFile.Close()
			_ = os.Remove(assembledPath)
			return nil, nil, fmt.Errorf("failed to read chunk #%d: %w", i, err)
		}
		_, copyErr := io.Copy(mw, cf)
		_ = cf.Close()
		if copyErr != nil {
			_ = assembledFile.Close()
			_ = os.Remove(assembledPath)
			return nil, nil, fmt.Errorf("failed to stream chunk #%d into assembled file: %w", i, copyErr)
		}
	}
	_ = assembledFile.Close()

	// Verify SHA-256
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if meta.FileSHA256 != "" && !strings.EqualFold(actualSHA, meta.FileSHA256) {
		_ = os.Remove(assembledPath)
		_ = os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("SHA-256校验失败: 组装文件哈希与客户端哈希不符")
	}

	// Dispatch based on upload_type
	if meta.UploadType == "archive" {
		// Guaranteed deletion of assembled archive file and chunk cache after import
		defer func() {
			_ = os.Remove(assembledPath)
			_ = os.RemoveAll(dir)
		}()

		total, imported, skipped, err := s.importService.ImportArchive(ctx, user, assembledPath, meta.Filename, meta.PolicyID, ImportOptions{
			PreservePath:        meta.PreservePath,
			StripTopDir:         meta.StripTopDir,
			ConflictAction:      meta.ConflictAction,
			WatermarkMode:       meta.WatermarkMode,
			WatermarkTemplateID: meta.WatermarkTemplateID,
			HotlinkMode:         meta.HotlinkMode,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, &ArchiveImportResult{Total: total, Imported: imported, Skipped: skipped}, nil
	}

	// Regular file upload
	defer func() {
		_ = os.Remove(assembledPath)
		_ = os.RemoveAll(dir)
	}()

	f, err := os.Open(assembledPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open assembled file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to stat assembled file: %w", err)
	}

	pIDs := meta.PolicyIDs
	if len(pIDs) == 0 && meta.PolicyID > 0 {
		pIDs = []uint{meta.PolicyID}
	}

	record, err := s.uploadService.Upload(ctx, user, f, meta.Filename, info.Size(), UploadOptions{
		PolicyID:            meta.PolicyID,
		PolicyIDs:           pIDs,
		DisplayName:         meta.Filename,
		WatermarkMode:       meta.WatermarkMode,
		WatermarkTemplateID: meta.WatermarkTemplateID,
		HotlinkMode:         meta.HotlinkMode,
		PublicMode:          meta.PublicMode,
	})
	if err != nil {
		return nil, nil, err
	}

	return record, nil, nil
}
