package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
)

type LocalStorageDriver struct {
	basePath string
}

func NewLocalStorageDriver(basePath string) (*LocalStorageDriver, error) {
	cleanBase := filepath.Clean(basePath)
	if err := os.MkdirAll(cleanBase, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory %s: %w", cleanBase, err)
	}
	return &LocalStorageDriver{basePath: cleanBase}, nil
}

func (d *LocalStorageDriver) resolvePath(key string) (string, error) {
	cleaned := pathclean.CleanPath(key)
	if cleaned == "" || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("invalid storage key: %s", key)
	}

	absBase, err := filepath.Abs(d.basePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute base path: %w", err)
	}

	full := filepath.Join(absBase, filepath.FromSlash(cleaned))
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute full path: %w", err)
	}

	rel, err := filepath.Rel(absBase, absFull)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path traversal attempt detected: %s", key)
	}

	return absFull, nil
}

func (d *LocalStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	fullPath, err := d.resolvePath(key)
	if err != nil {
		return err
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Atomic write via temp file + rename
	tempFile, err := os.CreateTemp(dir, "upload-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempName := tempFile.Name()

	cleanup := func() {
		tempFile.Close()
		os.Remove(tempName)
	}

	if _, err := io.Copy(tempFile, r); err != nil {
		cleanup()
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempName)
		return err
	}

	// Atomic rename
	if err := os.Rename(tempName, fullPath); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (d *LocalStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath, err := d.resolvePath(key)
	if err != nil {
		return nil, err
	}
	return os.Open(fullPath)
}

func (d *LocalStorageDriver) Delete(ctx context.Context, key string) error {
	fullPath, err := d.resolvePath(key)
	if err != nil {
		return err
	}
	err = os.Remove(fullPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (d *LocalStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	fullPath, err := d.resolvePath(key)
	if err != nil {
		return FileInfo{}, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Size:         info.Size(),
		LastModified: info.ModTime(),
	}, nil
}

func (d *LocalStorageDriver) GetLocalPath(key string) (string, bool) {
	fullPath, err := d.resolvePath(key)
	if err != nil {
		return "", false
	}
	return fullPath, true
}
