package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("oss", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg OSSConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.Bucket == "" || cfg.Endpoint == "" {
			return nil, errors.New("bucket and endpoint are required")
		}
		return NewOSSStorageDriver(cfg)
	})
}

type OSSConfig struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	RootDir         string `json:"root_dir,omitempty"`
	PublicURLPrefix string `json:"public_url_prefix"`
}

func (c *OSSConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		Endpoint        string `json:"endpoint"`
		Bucket          string `json:"bucket"`
		AccessKeyID     string `json:"access_key_id"`
		AccessKeySecret string `json:"access_key_secret"`
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
	c.Endpoint = strings.TrimSpace(raw.Endpoint)
	c.Bucket = strings.TrimSpace(raw.Bucket)
	c.AccessKeyID = strings.TrimSpace(raw.AccessKeyID)
	c.AccessKeySecret = strings.TrimSpace(raw.AccessKeySecret)
	if c.AccessKeySecret == "" {
		c.AccessKeySecret = strings.TrimSpace(raw.SecretAccessKey)
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

type OSSStorageDriver struct {
	cfg    OSSConfig
	client *oss.Client
	bucket *oss.Bucket
}

func normalizeOSSEndpoint(ep, bucket string) (string, bool) {
	cleaned := strings.TrimPrefix(ep, "https://")
	cleaned = strings.TrimPrefix(cleaned, "http://")
	cleaned = strings.TrimRight(cleaned, "/")

	if bucket != "" && strings.HasPrefix(cleaned, bucket+".") {
		cleaned = strings.TrimPrefix(cleaned, bucket+".")
	}

	isCname := false
	if strings.Contains(cleaned, ".oss-accesspoint.") {
		if strings.HasSuffix(bucket, "-ossalias") {
			isCname = true
		} else {
			// Extract region from access point domain: <ap-name>.<region>.oss-accesspoint.aliyuncs.com
			parts := strings.Split(cleaned, ".")
			for i, p := range parts {
				if p == "oss-accesspoint" && i > 0 {
					region := parts[i-1]
					suffix := strings.Join(parts[i+1:], ".")
					cleaned = region + "." + suffix
					break
				}
			}
		}
	}
	return cleaned, isCname
}

func NewOSSStorageDriver(cfg OSSConfig) (*OSSStorageDriver, error) {
	normHost, isCname := normalizeOSSEndpoint(cfg.Endpoint, cfg.Bucket)
	endpoint := "https://" + normHost

	var clientOpts []oss.ClientOption
	if isCname {
		clientOpts = append(clientOpts, oss.UseCname(true))
	}

	client, err := oss.New(endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to init aliyun oss client: %w", err)
	}

	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliyun oss bucket: %w", err)
	}

	return &OSSStorageDriver{
		cfg:    cfg,
		client: client,
		bucket: bucket,
	}, nil
}

func (d *OSSStorageDriver) resolveKey(key string) string {
	return strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
}

func (d *OSSStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	cleanKey := d.resolveKey(key)
	return d.bucket.PutObject(cleanKey, r)
}

func (d *OSSStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cleanKey := d.resolveKey(key)
	return d.bucket.GetObject(cleanKey)
}

func (d *OSSStorageDriver) Delete(ctx context.Context, key string) error {
	cleanKey := d.resolveKey(key)
	return d.bucket.DeleteObject(cleanKey)
}

func (d *OSSStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	cleanKey := d.resolveKey(key)
	header, err := d.bucket.GetObjectMeta(cleanKey)
	if err != nil {
		return FileInfo{}, err
	}

	var size int64
	var modTime time.Time
	if cl := header.Get("Content-Length"); cl != "" {
		_, _ = fmt.Sscanf(cl, "%d", &size)
	}
	if lm := header.Get("Last-Modified"); lm != "" {
		modTime, _ = time.Parse(time.RFC1123, lm)
	}

	return FileInfo{
		Size:         size,
		LastModified: modTime,
		MimeType:     header.Get("Content-Type"),
	}, nil
}

func (d *OSSStorageDriver) Test(ctx context.Context) error {
	exists, err := d.client.IsBucketExist(d.cfg.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", d.cfg.Bucket)
	}
	return nil
}

func (d *OSSStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *OSSStorageDriver) GetDirectURL(key string) (string, bool) {
	cleanKey := d.resolveKey(key)
	if d.cfg.PublicURLPrefix != "" {
		return strings.TrimRight(d.cfg.PublicURLPrefix, "/") + "/" + cleanKey, true
	}

	normHost, isCname := normalizeOSSEndpoint(d.cfg.Endpoint, d.cfg.Bucket)
	if normHost == "" {
		return "", false
	}
	if isCname || strings.HasPrefix(normHost, d.cfg.Bucket+".") {
		return fmt.Sprintf("https://%s/%s", normHost, cleanKey), true
	}
	return fmt.Sprintf("https://%s.%s/%s", d.cfg.Bucket, normHost, cleanKey), true
}
