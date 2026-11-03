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
	"github.com/tencentyun/cos-go-sdk-v5"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("cos", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg COSConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.BucketURL == "" {
			return nil, errors.New("bucket_url is required")
		}
		return NewCOSStorageDriver(cfg)
	})
}

type COSConfig struct {
	BucketURL       string `json:"bucket_url"`
	SecretID        string `json:"secret_id"`
	SecretKey       string `json:"secret_key"`
	RootDir         string `json:"root_dir,omitempty"`
	PublicURLPrefix string `json:"public_url_prefix"`
}

func (c *COSConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		BucketURL       string `json:"bucket_url"`
		Bucket          string `json:"bucket"`
		Region          string `json:"region"`
		SecretID        string `json:"secret_id"`
		AccessKeyID     string `json:"access_key_id"`
		SecretKey       string `json:"secret_key"`
		SecretAccessKey string `json:"secret_access_key"`
		RootDir         string `json:"root_dir"`
		BasePath        string `json:"base_path"`
		RootPath        string `json:"root_path"`
		PublicURLPrefix string `json:"public_url_prefix"`
		CustomDomain    string `json:"custom_domain"`
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.BucketURL = strings.TrimSpace(raw.BucketURL)
	if c.BucketURL == "" && raw.Bucket != "" && raw.Region != "" {
		c.BucketURL = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", strings.TrimSpace(raw.Bucket), strings.TrimSpace(raw.Region))
	}
	c.SecretID = strings.TrimSpace(raw.SecretID)
	if c.SecretID == "" {
		c.SecretID = strings.TrimSpace(raw.AccessKeyID)
	}
	c.SecretKey = strings.TrimSpace(raw.SecretKey)
	if c.SecretKey == "" {
		c.SecretKey = strings.TrimSpace(raw.SecretAccessKey)
	}
	c.RootDir = strings.Trim(strings.TrimSpace(raw.RootDir), "/")
	if c.RootDir == "" {
		c.RootDir = strings.Trim(strings.TrimSpace(raw.BasePath), "/")
	}
	if c.RootDir == "" {
		c.RootDir = strings.Trim(strings.TrimSpace(raw.RootPath), "/")
	}
	c.PublicURLPrefix = strings.TrimSpace(raw.PublicURLPrefix)
	if c.PublicURLPrefix == "" {
		c.PublicURLPrefix = strings.TrimSpace(raw.CustomDomain)
	}
	return nil
}

type COSStorageDriver struct {
	cfg    COSConfig
	client *cos.Client
}

func NewCOSStorageDriver(cfg COSConfig) (*COSStorageDriver, error) {
	bucketURL := cfg.BucketURL
	if !strings.HasPrefix(bucketURL, "http://") && !strings.HasPrefix(bucketURL, "https://") {
		bucketURL = "https://" + bucketURL
	}

	u, err := url.Parse(bucketURL)
	if err != nil {
		return nil, fmt.Errorf("invalid cos bucket url: %w", err)
	}

	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Timeout: 60 * time.Second,
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	})

	return &COSStorageDriver{
		cfg:    cfg,
		client: client,
	}, nil
}

func (d *COSStorageDriver) resolveKey(key string) string {
	return strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
}

func (d *COSStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	cleanKey := d.resolveKey(key)
	var opt *cos.ObjectPutOptions
	if size > 0 {
		opt = &cos.ObjectPutOptions{
			ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
				ContentLength: size,
			},
		}
	}
	_, err := d.client.Object.Put(ctx, cleanKey, r, opt)
	return err
}

func (d *COSStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cleanKey := d.resolveKey(key)
	resp, err := d.client.Object.Get(ctx, cleanKey, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *COSStorageDriver) Delete(ctx context.Context, key string) error {
	cleanKey := d.resolveKey(key)
	_, err := d.client.Object.Delete(ctx, cleanKey, nil)
	return err
}

func (d *COSStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	cleanKey := d.resolveKey(key)
	resp, err := d.client.Object.Head(ctx, cleanKey, nil)
	if err != nil {
		return FileInfo{}, err
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

func (d *COSStorageDriver) Test(ctx context.Context) error {
	_, err := d.client.Bucket.Head(ctx, nil)
	return err
}

func (d *COSStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *COSStorageDriver) GetDirectURL(key string) (string, bool) {
	cleanKey := d.resolveKey(key)
	if d.cfg.PublicURLPrefix != "" {
		return strings.TrimRight(d.cfg.PublicURLPrefix, "/") + "/" + cleanKey, true
	}
	bucketURL := strings.TrimRight(d.cfg.BucketURL, "/")
	if !strings.HasPrefix(bucketURL, "http://") && !strings.HasPrefix(bucketURL, "https://") {
		bucketURL = "https://" + bucketURL
	}
	return bucketURL + "/" + cleanKey, true
}
