package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
	Cache    CacheConfig    `yaml:"cache"`
	Security SecurityConfig `yaml:"security"`
	Policy   PolicyConfig   `yaml:"policy"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	BaseURL string `yaml:"base_url"`
	Mode    string `yaml:"mode"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"` // sqlite | postgres
	DSN    string `yaml:"dsn"`
}

type StorageConfig struct {
	Driver string      `yaml:"driver"`
	Local  LocalStorage `yaml:"local"`
}

type LocalStorage struct {
	BasePath string `yaml:"base_path"`
}

type CacheConfig struct {
	Dir       string `yaml:"dir"`
	MaxSizeMB int64  `yaml:"max_size_mb"`
}

type SecurityConfig struct {
	SecretKey          string        `yaml:"secret_key"`
	SessionTimeoutDays int           `yaml:"session_timeout_days"`
	RememberMeDays     int           `yaml:"remember_me_days"`
	MaxLoginAttempts   int           `yaml:"max_login_attempts"`
	LockoutMinutes     int           `yaml:"lockout_minutes"`
	Captcha            CaptchaConfig `yaml:"captcha"`
}

type CaptchaConfig struct {
	Provider           string `yaml:"provider"` // altcha | turnstile | none
	TurnstileSiteKey   string `yaml:"turnstile_site_key"`
	TurnstileSecretKey string `yaml:"turnstile_secret_key"`
}

type PolicyConfig struct {
	MaxSingleFileSize   int64 `yaml:"max_single_file_size"`
	MaxProcessImageSize int64 `yaml:"max_process_image_size"`
}

// LoadConfig loads configuration from path and overrides with environment variables
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			BaseURL: "http://localhost:8080",
			Mode:    "debug",
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "data/utopiaimages.db",
		},
		Storage: StorageConfig{
			Driver: "local",
			Local: LocalStorage{
				BasePath: "storage",
			},
		},
		Cache: CacheConfig{
			Dir:       "cache",
			MaxSizeMB: 5120,
		},
		Security: SecurityConfig{
			SecretKey:          "default-insecure-secret-key-change-me",
			SessionTimeoutDays: 7,
			RememberMeDays:     30,
			MaxLoginAttempts:   5,
			LockoutMinutes:     15,
			Captcha: CaptchaConfig{
				Provider: "altcha",
			},
		},
		Policy: PolicyConfig{
			MaxSingleFileSize:   52428800, // 50MB
			MaxProcessImageSize: 5242880,  // 5MB
		},
	}

	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	overrideWithEnv(cfg)
	return cfg, nil
}

func overrideWithEnv(cfg *Config) {
	if v := os.Getenv("UIMG_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("UIMG_SERVER_BASE_URL"); v != "" {
		cfg.Server.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("UIMG_SERVER_MODE"); v != "" {
		cfg.Server.Mode = v
	}
	if v := os.Getenv("UIMG_DB_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("UIMG_DB_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("UIMG_STORAGE_LOCAL_PATH"); v != "" {
		cfg.Storage.Local.BasePath = v
	}
	if v := os.Getenv("UIMG_SECURITY_SECRET_KEY"); v != "" {
		cfg.Security.SecretKey = v
	}
}
