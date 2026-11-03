package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"google.golang.org/api/option"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("gcs", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg GCSConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.Bucket == "" {
			return nil, errors.New("bucket is required")
		}
		return NewGCSStorageDriver(ctx, cfg)
	})
}

type GCSConfig struct {
	Bucket          string `json:"bucket"`
	CredentialsJSON string `json:"credentials_json"`
	RootDir         string `json:"root_dir,omitempty"`
	PublicURLPrefix string `json:"public_url_prefix"`
}

func (c *GCSConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		Bucket          string `json:"bucket"`
		CredentialsJSON string `json:"credentials_json"`
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
	c.Bucket = strings.TrimSpace(raw.Bucket)
	c.CredentialsJSON = strings.TrimSpace(raw.CredentialsJSON)
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

type GCSStorageDriver struct {
	cfg    GCSConfig
	client *storage.Client
}

func NewGCSStorageDriver(ctx context.Context, cfg GCSConfig) (*GCSStorageDriver, error) {
	var opts []option.ClientOption
	if strings.TrimSpace(cfg.CredentialsJSON) != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(cfg.CredentialsJSON)))
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to init google cloud storage client: %w", err)
	}

	return &GCSStorageDriver{
		cfg:    cfg,
		client: client,
	}, nil
}

func (d *GCSStorageDriver) resolveKey(key string) string {
	return strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
}

func (d *GCSStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	cleanKey := d.resolveKey(key)
	wc := d.client.Bucket(d.cfg.Bucket).Object(cleanKey).NewWriter(ctx)
	if _, err := io.Copy(wc, r); err != nil {
		_ = wc.Close()
		return err
	}
	return wc.Close()
}

func (d *GCSStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cleanKey := d.resolveKey(key)
	return d.client.Bucket(d.cfg.Bucket).Object(cleanKey).NewReader(ctx)
}

func (d *GCSStorageDriver) Delete(ctx context.Context, key string) error {
	cleanKey := d.resolveKey(key)
	err := d.client.Bucket(d.cfg.Bucket).Object(cleanKey).Delete(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	return err
}

func (d *GCSStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	cleanKey := d.resolveKey(key)
	attrs, err := d.client.Bucket(d.cfg.Bucket).Object(cleanKey).Attrs(ctx)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Size:         attrs.Size,
		LastModified: attrs.Updated,
		MimeType:     attrs.ContentType,
	}, nil
}

func (d *GCSStorageDriver) Test(ctx context.Context) error {
	_, err := d.client.Bucket(d.cfg.Bucket).Attrs(ctx)
	return err
}

func (d *GCSStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *GCSStorageDriver) GetDirectURL(key string) (string, bool) {
	cleanKey := d.resolveKey(key)
	if d.cfg.PublicURLPrefix != "" {
		return strings.TrimRight(d.cfg.PublicURLPrefix, "/") + "/" + cleanKey, true
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", d.cfg.Bucket, cleanKey), true
}
