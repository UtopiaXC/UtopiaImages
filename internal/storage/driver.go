package storage

import (
	"context"
	"io"
	"time"
)

type FileInfo struct {
	Size         int64
	LastModified time.Time
	MimeType     string
}

type StorageDriver interface {
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (FileInfo, error)
	// GetLocalPath returns physical file path if stored locally on disk
	GetLocalPath(key string) (string, bool)
}
