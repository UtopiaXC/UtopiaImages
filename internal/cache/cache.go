package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sync/singleflight"
)

type DerivedCache struct {
	baseDir string
	group   singleflight.Group
}

func NewDerivedCache(baseDir string) (*DerivedCache, error) {
	cleanDir := filepath.Clean(baseDir)
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}
	return &DerivedCache{baseDir: cleanDir}, nil
}

// GenerateKey generates a deterministic SHA-256 hash for image derivative
func (c *DerivedCache) GenerateKey(publicPath, params, watermarkVersion string) string {
	raw := fmt.Sprintf("%s|%s|%s", publicPath, params, watermarkVersion)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (c *DerivedCache) getFilePath(key, ext string) string {
	if len(key) < 4 {
		return filepath.Join(c.baseDir, key+ext)
	}
	sub1 := key[:2]
	sub2 := key[2:4]
	return filepath.Join(c.baseDir, sub1, sub2, key+ext)
}

// Get returns path to cached file if it exists
func (c *DerivedCache) Get(key, ext string) (string, bool) {
	filePath := c.getFilePath(key, ext)
	if _, err := os.Stat(filePath); err == nil {
		return filePath, true
	}
	return "", false
}

// Put writes processed bytes atomically to cache
func (c *DerivedCache) Put(key, ext string, data []byte) (string, error) {
	filePath := c.getFilePath(key, ext)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(dir, "cache-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	tmp.Close()

	if err := os.Rename(tmpName, filePath); err != nil {
		os.Remove(tmpName)
		return "", err
	}

	return filePath, nil
}

// ComputeSingleFlight deduplicates identical concurrent image processing requests
func (c *DerivedCache) ComputeSingleFlight(key string, fn func() ([]byte, string, error)) ([]byte, string, error) {
	res, err, _ := c.group.Do(key, func() (any, error) {
		data, mime, err := fn()
		if err != nil {
			return nil, err
		}
		return struct {
			data []byte
			mime string
		}{data: data, mime: mime}, nil
	})

	if err != nil {
		return nil, "", err
	}

	val := res.(struct {
		data []byte
		mime string
	})
	return val.data, val.mime, nil
}

// Clear removes all files in the cache directory
func (c *DerivedCache) Clear() int {
	entries, err := os.ReadDir(c.baseDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		os.RemoveAll(filepath.Join(c.baseDir, entry.Name()))
		count++
	}
	return count
}
