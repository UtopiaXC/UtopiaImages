package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("webdav", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg WebDAVConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.URL == "" {
			return nil, errors.New("webdav url is required")
		}
		return NewWebDAVStorageDriver(cfg)
	})
}

type WebDAVConfig struct {
	URL             string `json:"url"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	BasePath        string `json:"base_path"`
	PublicURLPrefix string `json:"public_url_prefix"`
}

func (c *WebDAVConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		URL             string `json:"url"`
		Endpoint        string `json:"endpoint"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		BasePath        string `json:"base_path"`
		RootPath        string `json:"root_path"`
		PublicURLPrefix string `json:"public_url_prefix"`
		DirectURL       string `json:"direct_url"`
		CustomDomain    string `json:"custom_domain"`
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.URL = strings.TrimSpace(raw.URL)
	if c.URL == "" {
		c.URL = strings.TrimSpace(raw.Endpoint)
	}
	c.Username = strings.TrimSpace(raw.Username)
	c.Password = raw.Password
	c.BasePath = strings.TrimSpace(raw.BasePath)
	if c.BasePath == "" {
		c.BasePath = strings.TrimSpace(raw.RootPath)
	}
	c.PublicURLPrefix = strings.TrimSpace(raw.PublicURLPrefix)
	if c.PublicURLPrefix == "" {
		c.PublicURLPrefix = strings.TrimSpace(raw.DirectURL)
	}
	if c.PublicURLPrefix == "" {
		c.PublicURLPrefix = strings.TrimSpace(raw.CustomDomain)
	}
	return nil
}

type WebDAVStorageDriver struct {
	cfg        WebDAVConfig
	httpClient *http.Client
}

func NewWebDAVStorageDriver(cfg WebDAVConfig) (*WebDAVStorageDriver, error) {
	parsed, err := url.Parse(cfg.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid webdav url: %s", cfg.URL)
	}

	cfg.URL = strings.TrimRight(cfg.URL, "/")
	cfg.BasePath = strings.Trim(cfg.BasePath, "/")
	cfg.PublicURLPrefix = strings.TrimRight(cfg.PublicURLPrefix, "/")

	return &WebDAVStorageDriver{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (d *WebDAVStorageDriver) buildFileURL(key string) string {
	cleanKey := strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
	return d.cfg.URL + "/" + cleanKey
}

func (d *WebDAVStorageDriver) ensureDirectories(ctx context.Context, fileURL string) {
	u, err := url.Parse(fileURL)
	if err != nil {
		return
	}

	rootURL, err := url.Parse(d.cfg.URL)
	if err != nil {
		return
	}

	relPath := strings.TrimPrefix(u.Path, rootURL.Path)
	relPath = strings.Trim(relPath, "/")
	parts := strings.Split(relPath, "/")
	if len(parts) <= 1 {
		return
	}

	curr := rootURL.String()
	for i := 0; i < len(parts)-1; i++ {
		curr = curr + "/" + parts[i]
		req, err := http.NewRequestWithContext(ctx, "MKCOL", curr, nil)
		if err != nil {
			continue
		}
		if d.cfg.Username != "" || d.cfg.Password != "" {
			req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
		}
		resp, err := d.httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}
}

func (d *WebDAVStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	fullURL := d.buildFileURL(key)
	d.ensureDirectories(ctx, fullURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fullURL, r)
	if err != nil {
		return err
	}
	if size > 0 {
		req.ContentLength = size
	}
	if d.cfg.Username != "" || d.cfg.Password != "" {
		req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webdav put failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (d *WebDAVStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullURL := d.buildFileURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if d.cfg.Username != "" || d.cfg.Password != "" {
		req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("webdav get failed (status %d)", resp.StatusCode)
	}
	return resp.Body, nil
}

func (d *WebDAVStorageDriver) Delete(ctx context.Context, key string) error {
	fullURL := d.buildFileURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fullURL, nil)
	if err != nil {
		return err
	}
	if d.cfg.Username != "" || d.cfg.Password != "" {
		req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("webdav delete failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (d *WebDAVStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	fullURL := d.buildFileURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fullURL, nil)
	if err != nil {
		return FileInfo{}, err
	}
	if d.cfg.Username != "" || d.cfg.Password != "" {
		req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return FileInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FileInfo{}, fmt.Errorf("webdav stat failed (status %d)", resp.StatusCode)
	}

	var modTime time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		modTime, _ = http.ParseTime(lm)
	}

	return FileInfo{
		Size:         resp.ContentLength,
		LastModified: modTime,
		MimeType:     resp.Header.Get("Content-Type"),
	}, nil
}

func (d *WebDAVStorageDriver) Test(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", d.cfg.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "0")
	if d.cfg.Username != "" || d.cfg.Password != "" {
		req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}

	// Fallback to OPTIONS
	optReq, err := http.NewRequestWithContext(ctx, http.MethodOptions, d.cfg.URL, nil)
	if err == nil {
		if d.cfg.Username != "" || d.cfg.Password != "" {
			optReq.SetBasicAuth(d.cfg.Username, d.cfg.Password)
		}
		if optResp, err := d.httpClient.Do(optReq); err == nil {
			defer optResp.Body.Close()
			if optResp.StatusCode >= 200 && optResp.StatusCode < 400 {
				return nil
			}
		}
	}

	return fmt.Errorf("webdav test failed with status %d", resp.StatusCode)
}

func (d *WebDAVStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *WebDAVStorageDriver) GetDirectURL(key string) (string, bool) {
	if d.cfg.PublicURLPrefix == "" {
		return "", false
	}
	cleanKey := strings.TrimLeft(key, "/")
	if d.cfg.BasePath != "" {
		cleanKey = path.Join(d.cfg.BasePath, cleanKey)
	}
	return d.cfg.PublicURLPrefix + "/" + cleanKey, true
}
