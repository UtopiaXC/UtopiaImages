package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

var (
	githubRepoLocksMu sync.Mutex
	githubRepoLocks   = make(map[string]*sync.Mutex)
)

func getRepoLock(owner, repo, branch string) *sync.Mutex {
	githubRepoLocksMu.Lock()
	defer githubRepoLocksMu.Unlock()
	key := fmt.Sprintf("%s/%s@%s", owner, repo, branch)
	lock, ok := githubRepoLocks[key]
	if !ok {
		lock = &sync.Mutex{}
		githubRepoLocks[key] = lock
	}
	return lock
}

func init() {
	RegisterDriver("github", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		var cfg GitHubConfig
		if policy.Config != "" {
			_ = json.Unmarshal([]byte(policy.Config), &cfg)
		}
		if cfg.Token == "" || cfg.Owner == "" || cfg.Repo == "" {
			return nil, errors.New("token, owner, and repo are required for github driver")
		}
		return NewGitHubStorageDriver(cfg)
	})
}

type GitHubConfig struct {
	Token        string `json:"token"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	PathPrefix   string `json:"path_prefix"`
	CDNType      string `json:"cdn_type"` // "jsdelivr", "raw", "custom"
	CustomDomain string `json:"custom_domain"`
}

func (c *GitHubConfig) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		Token           string `json:"token"`
		Owner           string `json:"owner"`
		Repo            string `json:"repo"`
		Branch          string `json:"branch"`
		PathPrefix      string `json:"path_prefix"`
		CDNType         string `json:"cdn_type"`
		CustomDomain    string `json:"custom_domain"`
		CDNCustomURL    string `json:"cdn_custom_url"`
		PublicURLPrefix string `json:"public_url_prefix"`
	}
	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.Token = strings.TrimSpace(raw.Token)
	c.Owner = strings.TrimSpace(raw.Owner)
	c.Repo = strings.TrimSpace(raw.Repo)
	c.Branch = strings.TrimSpace(raw.Branch)
	c.PathPrefix = strings.TrimSpace(raw.PathPrefix)
	c.CDNType = strings.TrimSpace(raw.CDNType)
	c.CustomDomain = strings.TrimSpace(raw.CustomDomain)
	if c.CustomDomain == "" {
		c.CustomDomain = strings.TrimSpace(raw.CDNCustomURL)
	}
	if c.CustomDomain == "" {
		c.CustomDomain = strings.TrimSpace(raw.PublicURLPrefix)
	}
	return nil
}

type GitHubStorageDriver struct {
	cfg        GitHubConfig
	httpClient *http.Client
	lock       *sync.Mutex
}

func NewGitHubStorageDriver(cfg GitHubConfig) (*GitHubStorageDriver, error) {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.CDNType == "" {
		cfg.CDNType = "jsdelivr"
	}

	return &GitHubStorageDriver{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		lock: getRepoLock(cfg.Owner, cfg.Repo, cfg.Branch),
	}, nil
}

func (d *GitHubStorageDriver) resolvePath(key string) string {
	return strings.TrimLeft(path.Clean("/"+strings.TrimLeft(key, "/\\")), "/")
}

func (d *GitHubStorageDriver) getFileSHA(ctx context.Context, fullPath string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", d.cfg.Owner, d.cfg.Repo, fullPath, d.cfg.Branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github get contents error (status %d)", resp.StatusCode)
	}

	var res struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.SHA, nil
}

func (d *GitHubStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	fullPath := d.resolvePath(key)
	existingSHA, _ := d.getFileSHA(ctx, fullPath)

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(data)

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", d.cfg.Owner, d.cfg.Repo, fullPath)
	payload := map[string]any{
		"message": fmt.Sprintf("Upload %s", path.Base(fullPath)),
		"content": encoded,
		"branch":  d.cfg.Branch,
	}
	if existingSHA != "" {
		payload["sha"] = existingSHA
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("github upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (d *GitHubStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := d.resolvePath(key)
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", d.cfg.Owner, d.cfg.Repo, d.cfg.Branch, fullPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("github raw get failed (status %d)", resp.StatusCode)
	}
	return resp.Body, nil
}

func (d *GitHubStorageDriver) Delete(ctx context.Context, key string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	fullPath := d.resolvePath(key)
	sha, err := d.getFileSHA(ctx, fullPath)
	if err != nil {
		return err
	}
	if sha == "" {
		return nil // already absent
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", d.cfg.Owner, d.cfg.Repo, fullPath)
	payload := map[string]any{
		"message": fmt.Sprintf("Delete %s", path.Base(fullPath)),
		"sha":     sha,
		"branch":  d.cfg.Branch,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("github delete failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (d *GitHubStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	fullPath := d.resolvePath(key)
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", d.cfg.Owner, d.cfg.Repo, fullPath, d.cfg.Branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return FileInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return FileInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FileInfo{}, fmt.Errorf("github stat failed (status %d)", resp.StatusCode)
	}

	var res struct {
		Size int64 `json:"size"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	return FileInfo{
		Size:         res.Size,
		LastModified: time.Now(),
	}, nil
}

func (d *GitHubStorageDriver) Test(ctx context.Context) error {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", d.cfg.Owner, d.cfg.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github repo check failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (d *GitHubStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}

func (d *GitHubStorageDriver) GetDirectURL(key string) (string, bool) {
	fullPath := d.resolvePath(key)
	switch strings.ToLower(d.cfg.CDNType) {
	case "raw":
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", d.cfg.Owner, d.cfg.Repo, d.cfg.Branch, fullPath), true
	case "custom":
		if d.cfg.CustomDomain != "" {
			domain := strings.TrimRight(d.cfg.CustomDomain, "/")
			if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
				domain = "https://" + domain
			}
			return fmt.Sprintf("%s/%s", domain, fullPath), true
		}
	}
	// Default to jsDelivr
	return fmt.Sprintf("https://cdn.jsdelivr.net/gh/%s/%s@%s/%s", d.cfg.Owner, d.cfg.Repo, d.cfg.Branch, fullPath), true
}
