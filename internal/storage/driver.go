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
	Test(ctx context.Context) error
	// GetLocalPath returns physical file path if stored locally on disk
	GetLocalPath(key string) (string, bool)
}

// DirectLinkProvider is implemented by drivers that can provide direct public access URLs
type DirectLinkProvider interface {
	GetDirectURL(key string) (string, bool)
}

// TokenRefresher is implemented by drivers that need to persist refreshed tokens back to storage policy
type TokenRefresher interface {
	SetTokenRefreshCallback(fn func(newConfig string) error)
}
