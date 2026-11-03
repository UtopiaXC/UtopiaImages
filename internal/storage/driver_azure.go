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

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("azure_blob", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg AzureBlobConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.ContainerName == "" {
			return nil, errors.New("container_name is required")
		}
		return NewAzureBlobStorageDriver(cfg)
	})
}

type AzureBlobConfig struct {
	ConnectionString string `json:"connection_string"`
	AccountName      string `json:"account_name"`
	AccountKey       string `json:"account_key"`
	ContainerName    string `json:"container_name"`
	RootDir          string `json:"root_dir,omitempty"`
	PublicURLPrefix  string `json:"public_url_prefix"`
}

func (c *AzureBlobConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		ConnectionString string `json:"connection_string"`
		AccountName      string `json:"account_name"`
		AccountKey       string `json:"account_key"`
		ContainerName    string `json:"container_name"`
		RootDir          string `json:"root_dir"`
		BasePath         string `json:"base_path"`
		RootPath         string `json:"root_path"`
		PublicURLPrefix  string `json:"public_url_prefix"`
		CustomDomain     string `json:"custom_domain"`
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.ConnectionString = strings.TrimSpace(raw.ConnectionString)
	c.AccountName = strings.TrimSpace(raw.AccountName)
	c.AccountKey = strings.TrimSpace(raw.AccountKey)
	c.ContainerName = strings.TrimSpace(raw.ContainerName)
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

type AzureBlobStorageDriver struct {
	cfg    AzureBlobConfig
	client *azblob.Client
}

func NewAzureBlobStorageDriver(cfg AzureBlobConfig) (*AzureBlobStorageDriver, error) {
	var client *azblob.Client
	var err error

	if strings.TrimSpace(cfg.ConnectionString) != "" {
		client, err = azblob.NewClientFromConnectionString(cfg.ConnectionString, nil)
	} else if cfg.AccountName != "" && cfg.AccountKey != "" {
		cred, credErr := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey)
		if credErr != nil {
			return nil, fmt.Errorf("invalid azure credentials: %w", credErr)
		}
		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", cfg.AccountName)
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	} else {
		return nil, errors.New("either connection_string or account_name and account_key is required")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to init azure blob client: %w", err)
	}

	return &AzureBlobStorageDriver{
		cfg:    cfg,
		client: client,
	}, nil
}

func (d *AzureBlobStorageDriver) resolveKey(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	if d.cfg.RootDir != "" {
		return path.Join(strings.Trim(d.cfg.RootDir, "/"), cleanKey)
	}
	return cleanKey
}

func (d *AzureBlobStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	cleanKey := d.resolveKey(key)
	_, err := d.client.UploadStream(ctx, d.cfg.ContainerName, cleanKey, r, nil)
	return err
}

func (d *AzureBlobStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cleanKey := d.resolveKey(key)
	resp, err := d.client.DownloadStream(ctx, d.cfg.ContainerName, cleanKey, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *AzureBlobStorageDriver) Delete(ctx context.Context, key string) error {
	cleanKey := d.resolveKey(key)
	_, err := d.client.DeleteBlob(ctx, d.cfg.ContainerName, cleanKey, nil)
	return err
}

func (d *AzureBlobStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	cleanKey := d.resolveKey(key)
	blobClient := d.client.ServiceClient().NewContainerClient(d.cfg.ContainerName).NewBlobClient(cleanKey)
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		return FileInfo{}, err
	}

	var size int64
	if props.ContentLength != nil {
		size = *props.ContentLength
	}

	var modTime time.Time
	if props.LastModified != nil {
		modTime = *props.LastModified
	}

	mimeType := ""
	if props.ContentType != nil {
		mimeType = *props.ContentType
	}

	return FileInfo{
		Size:         size,
		LastModified: modTime,
		MimeType:     mimeType,
	}, nil
}

func (d *AzureBlobStorageDriver) Test(ctx context.Context) error {
	containerClient := d.client.ServiceClient().NewContainerClient(d.cfg.ContainerName)
	_, err := containerClient.GetProperties(ctx, nil)
	return err
}

func (d *AzureBlobStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *AzureBlobStorageDriver) GetDirectURL(key string) (string, bool) {
	cleanKey := d.resolveKey(key)
	if d.cfg.PublicURLPrefix != "" {
		return strings.TrimRight(d.cfg.PublicURLPrefix, "/") + "/" + cleanKey, true
	}
	if d.cfg.AccountName != "" {
		return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", d.cfg.AccountName, d.cfg.ContainerName, cleanKey), true
	}
	return "", false
}
