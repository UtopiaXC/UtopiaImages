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
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"
)

func init() {
	factory := func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg S3Config
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.Bucket == "" {
			return nil, errors.New("bucket is required")
		}
		return NewS3StorageDriver(cfg)
	}

	RegisterDriver("s3", factory)
	RegisterDriver("s3_compatible", factory)
	RegisterDriver("minio", factory)
}

type S3Config struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	ForcePathStyle  bool   `json:"force_path_style"`
	RootDir         string `json:"root_dir,omitempty"`
	PublicURLPrefix string `json:"public_url_prefix"`
}

func (c *S3Config) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		Endpoint        string `json:"endpoint"`
		Region          string `json:"region"`
		Bucket          string `json:"bucket"`
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		SecretKey       string `json:"secret_key"`
		ForcePathStyle  bool   `json:"force_path_style"`
		UsePathStyle    bool   `json:"use_path_style"`
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
	c.Region = strings.TrimSpace(raw.Region)
	c.Bucket = strings.TrimSpace(raw.Bucket)
	c.AccessKeyID = strings.TrimSpace(raw.AccessKeyID)
	c.SecretAccessKey = strings.TrimSpace(raw.SecretAccessKey)
	if c.SecretAccessKey == "" {
		c.SecretAccessKey = strings.TrimSpace(raw.SecretKey)
	}
	c.ForcePathStyle = raw.ForcePathStyle || raw.UsePathStyle
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

type S3StorageDriver struct {
	cfg    S3Config
	client *s3.Client
}

func NewS3StorageDriver(cfg S3Config) (*S3StorageDriver, error) {
	if cfg.Region == "" {
		cfg.Region = "auto"
	}

	customResolver := s3.EndpointResolverV2(nil)
	_ = customResolver

	awsCfg := aws.Config{
		Region: cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		),
	}

	var s3Options []func(*s3.Options)
	if cfg.Endpoint != "" {
		endpoint := cfg.Endpoint
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
		s3Options = append(s3Options, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	if cfg.ForcePathStyle {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Options...)

	return &S3StorageDriver{
		cfg:    cfg,
		client: client,
	}, nil
}

func (d *S3StorageDriver) resolveKey(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	if d.cfg.RootDir != "" {
		return path.Join(strings.Trim(d.cfg.RootDir, "/"), cleanKey)
	}
	return cleanKey
}

func (d *S3StorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(d.cfg.Bucket),
		Key:    aws.String(d.resolveKey(key)),
		Body:   r,
	}
	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}

	_, err := d.client.PutObject(ctx, input)
	return err
}

func (d *S3StorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(d.cfg.Bucket),
		Key:    aws.String(d.resolveKey(key)),
	}

	output, err := d.client.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}

func (d *S3StorageDriver) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(d.cfg.Bucket),
		Key:    aws.String(d.resolveKey(key)),
	}

	_, err := d.client.DeleteObject(ctx, input)
	return err
}

func (d *S3StorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(d.cfg.Bucket),
		Key:    aws.String(d.resolveKey(key)),
	}

	output, err := d.client.HeadObject(ctx, input)
	if err != nil {
		return FileInfo{}, err
	}

	var modTime time.Time
	if output.LastModified != nil {
		modTime = *output.LastModified
	}

	mimeType := ""
	if output.ContentType != nil {
		mimeType = *output.ContentType
	}

	size := int64(0)
	if output.ContentLength != nil {
		size = *output.ContentLength
	}

	return FileInfo{
		Size:         size,
		LastModified: modTime,
		MimeType:     mimeType,
	}, nil
}

func (d *S3StorageDriver) Test(ctx context.Context) error {
	_, err := d.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(d.cfg.Bucket),
	})
	return err
}

func (d *S3StorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *S3StorageDriver) GetDirectURL(key string) (string, bool) {
	cleanKey := d.resolveKey(key)

	if d.cfg.PublicURLPrefix != "" {
		return strings.TrimRight(d.cfg.PublicURLPrefix, "/") + "/" + cleanKey, true
	}

	if d.cfg.Endpoint != "" {
		endpoint := d.cfg.Endpoint
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
		u, err := url.Parse(endpoint)
		if err == nil {
			if d.cfg.ForcePathStyle {
				return fmt.Sprintf("%s/%s/%s", strings.TrimRight(endpoint, "/"), d.cfg.Bucket, cleanKey), true
			}
			return fmt.Sprintf("%s://%s.%s/%s", u.Scheme, d.cfg.Bucket, u.Host, cleanKey), true
		}
	}

	region := d.cfg.Region
	if region == "" || region == "auto" {
		region = "us-east-1"
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", d.cfg.Bucket, region, cleanKey), true
}
