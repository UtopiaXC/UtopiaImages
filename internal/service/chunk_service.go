package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	WatermarkMode       string    `json:"watermark_mode"`
	WatermarkTemplateID *uint     `json:"watermark_template_id"`
	HotlinkMode         string    `json:"hotlink_mode"`
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
	db            *gorm.DB
	cacheDir      string
	uploadService *UploadService
	importService *ImportService
	lock          sync.Mutex
	stopCleaner   chan struct{}
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
		db:            db,
		cacheDir:      cleanCache,
		uploadService: uploadService,
		importService: importService,
		stopCleaner:   make(chan struct{}),
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

func (s *ChunkService) getUploadDir(uploadID string) string {
	cleanID := filepath.Base(strings.TrimSpace(uploadID))
	return filepath.Join(s.cacheDir, "chunks", cleanID)
}

func (s *ChunkService) getMetaPath(uploadID string) string {
	return filepath.Join(s.getUploadDir(uploadID), "meta.json")
}

func (s *ChunkService) readMeta(uploadID string) (*ChunkUploadMeta, error) {
	metaPath := s.getMetaPath(uploadID)
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

func (s *ChunkService) saveMeta(meta *ChunkUploadMeta) error {
	dir := s.getUploadDir(meta.UploadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create chunk directory in cache: %w", err)
	}
	meta.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.getMetaPath(meta.UploadID), data, 0644)
}

// InitChunkUpload initializes a new chunk upload task
func (s *ChunkService) InitChunkUpload(user *model.User, meta ChunkUploadMeta) (*ChunkUploadMeta, []int, error) {
	if meta.TotalChunks <= 0 {
		return nil, nil, fmt.Errorf("total_chunks must be greater than 0")
	}
	if meta.TotalSize < 0 {
		return nil, nil, fmt.Errorf("total_size cannot be negative")
	}
	if meta.Filename == "" {
		return nil, nil, fmt.Errorf("filename cannot be empty")
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

	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if !isAdmin && meta.UserID != user.ID {
		return fmt.Errorf("permission denied: upload task belongs to another user")
	}

	if chunkIndex < 0 || chunkIndex >= meta.TotalChunks {
		return fmt.Errorf("chunk_index out of bounds (0 <= index < %d)", meta.TotalChunks)
	}

	chunkPath := filepath.Join(s.getUploadDir(uploadID), fmt.Sprintf("%05d.chunk", chunkIndex))
	tmpChunkPath := chunkPath + ".tmp"

	f, err := os.OpenFile(tmpChunkPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create chunk file in cache: %w", err)
	}

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpChunkPath)
		return fmt.Errorf("failed to write chunk data: %w", err)
	}
	_ = f.Close()

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

	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if !isAdmin && meta.UserID != user.ID {
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
	meta, err := s.readMeta(uploadID)
	if err == nil {
		isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
		if !isAdmin && meta.UserID != user.ID {
			return fmt.Errorf("permission denied")
		}
	}

	dir := s.getUploadDir(uploadID)
	assembledPath := filepath.Join(s.cacheDir, "chunks", uploadID+".assembled")

	_ = os.RemoveAll(dir)
	_ = os.Remove(assembledPath)
	slog.Info("Chunk upload task aborted, cache cleared", "upload_id", uploadID)
	return nil
}

// CompleteChunkUpload merges all chunks in cache, checks SHA-256, and dispatches to uploadService or importService
func (s *ChunkService) CompleteChunkUpload(ctx context.Context, user *model.User, uploadID string) (*model.FileRecord, *ArchiveImportResult, error) {
	meta, err := s.readMeta(uploadID)
	if err != nil {
		return nil, nil, err
	}

	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if !isAdmin && meta.UserID != user.ID {
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
		return nil, nil, fmt.Errorf("SHA-256校验失败：组装文件哈希 (%s) 与客户端哈希 (%s) 不符，文件损坏", actualSHA, meta.FileSHA256)
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

	record, err := s.uploadService.Upload(ctx, user, f, meta.Filename, info.Size(), UploadOptions{
		PolicyID:            meta.PolicyID,
		DisplayName:         meta.Filename,
		WatermarkMode:       meta.WatermarkMode,
		WatermarkTemplateID: meta.WatermarkTemplateID,
	})
	if err != nil {
		return nil, nil, err
	}

	return record, nil, nil
}
