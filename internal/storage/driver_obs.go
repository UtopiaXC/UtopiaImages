package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("obs", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg OBSConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.Bucket == "" || cfg.Endpoint == "" {
			return nil, errors.New("bucket and endpoint are required")
		}
		return NewOBSStorageDriver(cfg)
	})
}

type OBSConfig struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	AccessKey       string `json:"access_key"`
	SecurityKey     string `json:"security_key"`
	RootDir         string `json:"root_dir,omitempty"`
	PublicURLPrefix string `json:"public_url_prefix"`
}

func (c *OBSConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		Endpoint        string `json:"endpoint"`
		Bucket          string `json:"bucket"`
		AccessKey       string `json:"access_key"`
		AccessKeyID     string `json:"access_key_id"`
		SecurityKey     string `json:"security_key"`
		SecretAccessKey string `json:"secret_access_key"`
		SecretKey       string `json:"secret_key"`
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
	c.Endpoint = strings.TrimSpace(raw.Endpoint)
	c.Bucket = strings.TrimSpace(raw.Bucket)
	c.AccessKey = strings.TrimSpace(raw.AccessKey)
	if c.AccessKey == "" {
		c.AccessKey = strings.TrimSpace(raw.AccessKeyID)
	}
	c.SecurityKey = strings.TrimSpace(raw.SecurityKey)
	if c.SecurityKey == "" {
		c.SecurityKey = strings.TrimSpace(raw.SecretAccessKey)
	}
	if c.SecurityKey == "" {
		c.SecurityKey = strings.TrimSpace(raw.SecretKey)
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

type OBSStorageDriver struct {
	cfg    OBSConfig
	client *obs.ObsClient
}

func NewOBSStorageDriver(cfg OBSConfig) (*OBSStorageDriver, error) {
	endpoint := cfg.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	client, err := obs.New(cfg.AccessKey, cfg.SecurityKey, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to init huawei obs client: %w", err)
	}

	return &OBSStorageDriver{
		cfg:    cfg,
		client: client,
	}, nil
}

func (d *OBSStorageDriver) resolveKey(key string) string {
	return strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
}

func (d *OBSStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	cleanKey := d.resolveKey(key)
	input := &obs.PutObjectInput{
		Bucket: d.cfg.Bucket,
		Key:    cleanKey,
		Body:   r,
	}
	if size > 0 {
		input.ContentLength = size
	}
	_, err := d.client.PutObject(input)
	return err
}

func (d *OBSStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cleanKey := d.resolveKey(key)
	input := &obs.GetObjectInput{
		Bucket: d.cfg.Bucket,
		Key:    cleanKey,
	}
	output, err := d.client.GetObject(input)
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}

func (d *OBSStorageDriver) Delete(ctx context.Context, key string) error {
	cleanKey := d.resolveKey(key)
	input := &obs.DeleteObjectInput{
		Bucket: d.cfg.Bucket,
		Key:    cleanKey,
	}
	_, err := d.client.DeleteObject(input)
	return err
}

func (d *OBSStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	cleanKey := d.resolveKey(key)
	input := &obs.GetObjectMetadataInput{
		Bucket: d.cfg.Bucket,
		Key:    cleanKey,
	}
	output, err := d.client.GetObjectMetadata(input)
	if err != nil {
		return FileInfo{}, err
	}

	return FileInfo{
		Size:         output.ContentLength,
		LastModified: output.LastModified,
		MimeType:     output.ContentType,
	}, nil
}

func (d *OBSStorageDriver) Test(ctx context.Context) error {
	_, err := d.client.HeadBucket(d.cfg.Bucket)
	return err
}

func (d *OBSStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *OBSStorageDriver) GetDirectURL(key string) (string, bool) {
	cleanKey := d.resolveKey(key)
	if d.cfg.PublicURLPrefix != "" {
		return strings.TrimRight(d.cfg.PublicURLPrefix, "/") + "/" + cleanKey, true
	}

	ep := d.cfg.Endpoint
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		ep = "https://" + ep
	}
	u, err := url.Parse(ep)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%s://%s.%s/%s", u.Scheme, d.cfg.Bucket, u.Host, cleanKey), true
}
