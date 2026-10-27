package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type DerivedCache struct {
	baseDir   string
	group     singleflight.Group
	maxSizeMB atomic.Int64 // 0 = unlimited
	ttlDays   atomic.Int64 // 0 = unlimited
	isPruning atomic.Bool
}

func NewDerivedCache(baseDir string) (*DerivedCache, error) {
	cleanDir := filepath.Clean(baseDir)
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}
	return &DerivedCache{baseDir: cleanDir}, nil
}

// SetLimits updates cache capacity and retention limits
func (c *DerivedCache) SetLimits(maxSizeMB int64, ttlDays int) {
	if maxSizeMB < 0 {
		maxSizeMB = 0
	}
	if ttlDays < 0 {
		ttlDays = 0
	}
	c.maxSizeMB.Store(maxSizeMB)
	c.ttlDays.Store(int64(ttlDays))
}

// GetLimits returns the current cache limits
func (c *DerivedCache) GetLimits() (maxSizeMB int64, ttlDays int) {
	return c.maxSizeMB.Load(), int(c.ttlDays.Load())
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

// Get returns path to cached file if it exists and touches access time for LRU
func (c *DerivedCache) Get(key, ext string) (string, bool) {
	filePath := c.getFilePath(key, ext)
	if _, err := os.Stat(filePath); err == nil {
		now := time.Now()
		_ = os.Chtimes(filePath, now, now)
		return filePath, true
	}
	return "", false
}

// Put writes processed bytes atomically to cache and triggers pruning if needed
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

	// Trigger asynchronous pruning if thresholds are exceeded
	c.PruneIfNecessary()

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

// GetStats returns current total cached bytes and total file count
func (c *DerivedCache) GetStats() (totalBytes int64, fileCount int, err error) {
	err = filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && !strings.HasPrefix(info.Name(), "cache-") {
			totalBytes += info.Size()
			fileCount++
		}
		return nil
	})
	return totalBytes, fileCount, err
}

type cacheFileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// Prune cleans up expired cache files and oldest files if capacity is exceeded
func (c *DerivedCache) Prune(maxBytes int64, maxAge time.Duration) (clearedCount int, freedBytes int64, err error) {
	var files []cacheFileInfo
	var totalSize int64

	now := time.Now()
	var expirationCutoff time.Time
	if maxAge > 0 {
		expirationCutoff = now.Add(-maxAge)
	}

	// 1. Scan all cached files
	_ = filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Clean up leftover tmp files older than 10 minutes
		if strings.HasPrefix(info.Name(), "cache-") {
			if now.Sub(info.ModTime()) > 10*time.Minute {
				_ = os.Remove(path)
			}
			return nil
		}

		// 2. Immediate eviction for expired files
		if maxAge > 0 && info.ModTime().Before(expirationCutoff) {
			if removeErr := os.Remove(path); removeErr == nil {
				clearedCount++
				freedBytes += info.Size()
				return nil
			}
		}

		files = append(files, cacheFileInfo{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		totalSize += info.Size()
		return nil
	})

	// 3. If capacity limit is specified and current size exceeds limit, remove oldest files (LRU)
	if maxBytes > 0 && totalSize > maxBytes {
		// Sort by modTime ascending (oldest first)
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.Before(files[j].modTime)
		})

		for _, f := range files {
			if totalSize <= maxBytes {
				break
			}
			if removeErr := os.Remove(f.path); removeErr == nil {
				clearedCount++
				freedBytes += f.size
				totalSize -= f.size
			}
		}
	}

	return clearedCount, freedBytes, nil
}

// PruneIfNecessary triggers non-blocking background pruning based on current limits
func (c *DerivedCache) PruneIfNecessary() {
	maxMB := c.maxSizeMB.Load()
	ttl := c.ttlDays.Load()

	// If both are unlimited, no automatic eviction needed
	if maxMB <= 0 && ttl <= 0 {
		return
	}

	if !c.isPruning.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer c.isPruning.Store(false)
		maxBytes := maxMB * 1024 * 1024
		var maxAge time.Duration
		if ttl > 0 {
			maxAge = time.Duration(ttl) * 24 * time.Hour
		}
		_, _, _ = c.Prune(maxBytes, maxAge)
	}()
}

// Clear removes all files in the cache directory
func (c *DerivedCache) Clear() int {
	entries, err := os.ReadDir(c.baseDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(c.baseDir, entry.Name()))
		count++
	}
	return count
}
