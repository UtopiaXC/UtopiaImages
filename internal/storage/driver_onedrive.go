package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterDriver("onedrive", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg OneDriveConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.ClientID == "" || cfg.RefreshToken == "" {
			return nil, errors.New("client_id and refresh_token are required for onedrive driver")
		}
		return NewOneDriveStorageDriver(cfg)
	})
}

type OneDriveConfig struct {
	AccountType  string `json:"account_type"` // "personal", "business", "china"
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	TokenExpires int64  `json:"token_expires"`
	DriveID      string `json:"drive_id"`
	BasePath     string `json:"base_path"`
}

func (c *OneDriveConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		AccountType  string `json:"account_type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
		TokenExpires int64  `json:"token_expires"`
		DriveID      string `json:"drive_id"`
		BasePath     string `json:"base_path"`
		RootPath     string `json:"root_path"`
		RootDir      string `json:"root_dir"`
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.AccountType = strings.TrimSpace(raw.AccountType)
	c.ClientID = strings.TrimSpace(raw.ClientID)
	c.ClientSecret = strings.TrimSpace(raw.ClientSecret)
	c.RefreshToken = strings.TrimSpace(raw.RefreshToken)
	c.AccessToken = strings.TrimSpace(raw.AccessToken)
	c.TokenExpires = raw.TokenExpires
	c.DriveID = strings.TrimSpace(raw.DriveID)
	c.BasePath = strings.Trim(strings.TrimSpace(raw.BasePath), "/")
	if c.BasePath == "" {
		c.BasePath = strings.Trim(strings.TrimSpace(raw.RootPath), "/")
	}
	if c.BasePath == "" {
		c.BasePath = strings.Trim(strings.TrimSpace(raw.RootDir), "/")
	}
	return nil
}

type OneDriveStorageDriver struct {
	cfg            OneDriveConfig
	mu             sync.Mutex
	httpClient     *http.Client
	onTokenRefresh func(newConfig string) error
}

func NewOneDriveStorageDriver(cfg OneDriveConfig) (*OneDriveStorageDriver, error) {
	if cfg.AccountType == "" {
		cfg.AccountType = "personal"
	}

	return &OneDriveStorageDriver{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (d *OneDriveStorageDriver) SetTokenRefreshCallback(fn func(newConfig string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onTokenRefresh = fn
}

func (d *OneDriveStorageDriver) tokenEndpoint() string {
	switch strings.ToLower(d.cfg.AccountType) {
	case "business", "work", "organization", "organizations":
		return "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	case "china":
		return "https://login.partner.microsoftonline.cn/common/oauth2/v2.0/token"
	default:
		return "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
	}
}

func (d *OneDriveStorageDriver) graphRoot() string {
	if strings.ToLower(d.cfg.AccountType) == "china" {
		return "https://microsoftgraph.chinacloudapi.cn/v1.0"
	}
	return "https://graph.microsoft.com/v1.0"
}

func (d *OneDriveStorageDriver) ensureAccessToken(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	if d.cfg.AccessToken != "" && d.cfg.TokenExpires > now+300 {
		return d.cfg.AccessToken, nil
	}

	form := url.Values{}
	form.Set("client_id", d.cfg.ClientID)
	if d.cfg.ClientSecret != "" {
		form.Set("client_secret", d.cfg.ClientSecret)
	}
	form.Set("refresh_token", d.cfg.RefreshToken)
	form.Set("grant_type", "refresh_token")

	tokenURL := d.tokenEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to refresh onedrive token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("onedrive token refresh failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenRes struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return "", err
	}

	d.cfg.AccessToken = tokenRes.AccessToken
	if tokenRes.RefreshToken != "" {
		d.cfg.RefreshToken = tokenRes.RefreshToken
	}
	d.cfg.TokenExpires = now + tokenRes.ExpiresIn

	if d.onTokenRefresh != nil {
		newCfgBytes, _ := json.Marshal(d.cfg)
		_ = d.onTokenRefresh(string(newCfgBytes))
	}

	return d.cfg.AccessToken, nil
}

func (d *OneDriveStorageDriver) buildItemURL(key string, suffix string) string {
	cleanKey := strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
	if d.cfg.BasePath != "" {
		cleanBase := strings.Trim(path.Clean("/"+strings.Trim(d.cfg.BasePath, "/\\")), "/")
		if cleanBase != "" && cleanBase != "." {
			cleanKey = cleanBase + "/" + cleanKey
		}
	}
	prefix := "/me/drive"
	if d.cfg.DriveID != "" {
		prefix = fmt.Sprintf("/drives/%s", d.cfg.DriveID)
	}

	if cleanKey == "" || cleanKey == "." {
		return fmt.Sprintf("%s%s/root%s", d.graphRoot(), prefix, suffix)
	}
	return fmt.Sprintf("%s%s/root:/%s%s", d.graphRoot(), prefix, cleanKey, suffix)
}

func (d *OneDriveStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	token, err := d.ensureAccessToken(ctx)
	if err != nil {
		return err
	}

	// For files <= 4MB, use simple direct PUT
	if size > 0 && size <= 4*1024*1024 {
		apiURL := d.buildItemURL(key, ":/content")
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, r)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = size

		resp, err := d.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			return fmt.Errorf("onedrive simple upload failed (status %d): %s", resp.StatusCode, string(body))
		}
		return nil
	}

	// For large files (> 4MB) or stream with unknown size, create upload session
	createSessionURL := d.buildItemURL(key, ":/createUploadSession")
	sessionReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createSessionURL, bytes.NewReader([]byte(`{"item":{"@microsoft.graph.conflictBehavior":"replace"}}`)))
	if err != nil {
		return err
	}
	sessionReq.Header.Set("Authorization", "Bearer "+token)
	sessionReq.Header.Set("Content-Type", "application/json")

	sessionResp, err := d.httpClient.Do(sessionReq)
	if err != nil {
		return err
	}
	defer sessionResp.Body.Close()

	if sessionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(sessionResp.Body, 1024))
		return fmt.Errorf("onedrive create upload session failed (status %d): %s", sessionResp.StatusCode, string(body))
	}

	var sessionRes struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.NewDecoder(sessionResp.Body).Decode(&sessionRes); err != nil {
		return err
	}

	// Chunk size must be a multiple of 320 KiB (327,680 bytes)
	chunkSize := 10 * 320 * 1024 // ~3.2 MB
	buf := make([]byte, chunkSize)
	var offset int64 = 0

	for {
		n, readErr := io.ReadFull(r, buf)
		if n > 0 {
			chunkData := buf[:n]
			end := offset + int64(n) - 1

			rangeHeader := fmt.Sprintf("bytes %d-%d/*", offset, end)
			if size > 0 {
				rangeHeader = fmt.Sprintf("bytes %d-%d/%d", offset, end, size)
			}

			chunkReq, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionRes.UploadURL, bytes.NewReader(chunkData))
			if err != nil {
				return err
			}
			chunkReq.Header.Set("Content-Range", rangeHeader)
			chunkReq.Header.Set("Content-Length", fmt.Sprintf("%d", n))

			chunkResp, err := d.httpClient.Do(chunkReq)
			if err != nil {
				return fmt.Errorf("onedrive chunk upload failed at offset %d: %w", offset, err)
			}
			_ = chunkResp.Body.Close()

			if chunkResp.StatusCode != http.StatusOK && chunkResp.StatusCode != http.StatusCreated && chunkResp.StatusCode != http.StatusAccepted {
				return fmt.Errorf("onedrive chunk upload failed at offset %d (status %d)", offset, chunkResp.StatusCode)
			}

			offset += int64(n)
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	return nil
}

func (d *OneDriveStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	token, err := d.ensureAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	apiURL := d.buildItemURL(key, ":/content")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("onedrive get failed (status %d)", resp.StatusCode)
	}
	return resp.Body, nil
}

func (d *OneDriveStorageDriver) Delete(ctx context.Context, key string) error {
	token, err := d.ensureAccessToken(ctx)
	if err != nil {
		return err
	}

	apiURL := d.buildItemURL(key, "")
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("onedrive delete failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (d *OneDriveStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	token, err := d.ensureAccessToken(ctx)
	if err != nil {
		return FileInfo{}, err
	}

	apiURL := d.buildItemURL(key, "")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return FileInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return FileInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FileInfo{}, fmt.Errorf("onedrive stat failed (status %d)", resp.StatusCode)
	}

	var res struct {
		Size                 int64  `json:"size"`
		LastModifiedDateTime string `json:"lastModifiedDateTime"`
		File                 *struct {
			MimeType string `json:"mimeType"`
		} `json:"file"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return FileInfo{}, err
	}

	var modTime time.Time
	if res.LastModifiedDateTime != "" {
		modTime, _ = time.Parse(time.RFC3339, res.LastModifiedDateTime)
	}

	mimeType := ""
	if res.File != nil {
		mimeType = res.File.MimeType
	}

	return FileInfo{
		Size:         res.Size,
		LastModified: modTime,
		MimeType:     mimeType,
	}, nil
}

func (d *OneDriveStorageDriver) Test(ctx context.Context) error {
	token, err := d.ensureAccessToken(ctx)
	if err != nil {
		return err
	}

	prefix := "/me/drive"
	if d.cfg.DriveID != "" {
		prefix = fmt.Sprintf("/drives/%s", d.cfg.DriveID)
	}
	apiURL := fmt.Sprintf("%s%s", d.graphRoot(), prefix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("onedrive test failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (d *OneDriveStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}
