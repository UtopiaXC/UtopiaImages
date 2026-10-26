package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site     SiteConfig     `yaml:"site"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
	Cache    CacheConfig    `yaml:"cache"`
	Security SecurityConfig `yaml:"security"`
	Policy   PolicyConfig   `yaml:"policy"`
	SMTP     SMTPConfig     `yaml:"smtp"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type SiteConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	BaseURL     string `yaml:"base_url"`
	Locale      string `yaml:"locale"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	BaseURL string `yaml:"base_url"`
	Mode    string `yaml:"mode"` // release | debug
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"` // sqlite | postgres | mysql
	DSN    string `yaml:"dsn"`
}

type StorageConfig struct {
	Driver string       `yaml:"driver"`
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

type SMTPConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	From       string `yaml:"from"`
	FromName   string `yaml:"from_name"`
	User       string `yaml:"user"`
	Password   string `yaml:"password"`
	Encryption string `yaml:"encryption"` // ssl | tls | none
}

type LoggingConfig struct {
	Level          string `yaml:"level"`        // debug | info | warn | error
	Dir            string `yaml:"dir"`          // logs
	MaxSizeMB      int    `yaml:"max_size_mb"`  // 20 MB
	MaxBackups     int    `yaml:"max_backups"`  // 10
	MaxAgeDays     int    `yaml:"max_age_days"` // 30
	Compress       bool   `yaml:"compress"`
	PermanentLevel string `yaml:"permanent_level"` // error | warn | none
}

// LoadConfig loads configuration from path and overrides with environment variables
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Site: SiteConfig{
			Name:        "Utopia Images",
			Description: "又一个Utopia Images自部署图床",
			BaseURL:     "http://localhost:8080",
			Locale:      "zh-CN",
		},
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			BaseURL: "http://localhost:8080",
			Mode:    "release",
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "data/db/utopiaimages.db",
		},
		Storage: StorageConfig{
			Driver: "local",
			Local: LocalStorage{
				BasePath: "data/storage",
			},
		},
		Cache: CacheConfig{
			Dir:       "data/cache",
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
		SMTP: SMTPConfig{
			Enabled:    false,
			Host:       "",
			Port:       465,
			From:       "",
			FromName:   "Utopia Images",
			User:       "",
			Password:   "",
			Encryption: "ssl",
		},
		Logging: LoggingConfig{
			Level:          "info",
			Dir:            "data/logs",
			MaxSizeMB:      20,
			MaxBackups:     10,
			MaxAgeDays:     30,
			Compress:       true,
			PermanentLevel: "error",
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

// SaveConfig writes the in-memory configuration back to yaml file
func SaveConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config dir: %w", err)
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func overrideWithEnv(cfg *Config) {
	if v := os.Getenv("UIMG_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("UIMG_SERVER_BASE_URL"); v != "" {
		cfg.Server.BaseURL = strings.TrimRight(v, "/")
		cfg.Site.BaseURL = cfg.Server.BaseURL
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
	if v := os.Getenv("UIMG_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
}

// EnsureDataDirectories ensures all required runtime data directories exist
func EnsureDataDirectories(cfg *Config) {
	dirs := []string{
		"data",
		"data/db",
		"data/cache",
		"data/storage",
		"data/logs",
	}
	if cfg != nil {
		if (cfg.Database.Driver == "sqlite" || cfg.Database.Driver == "sqlite3" || cfg.Database.Driver == "") && cfg.Database.DSN != "" {
			dirs = append(dirs, filepath.Dir(cfg.Database.DSN))
		}
		if cfg.Storage.Local.BasePath != "" {
			dirs = append(dirs, cfg.Storage.Local.BasePath)
		}
		if cfg.Cache.Dir != "" {
			dirs = append(dirs, cfg.Cache.Dir)
		}
		if cfg.Logging.Dir != "" {
			dirs = append(dirs, cfg.Logging.Dir)
		}
	}
	for _, d := range dirs {
		if d != "" && d != "." {
			_ = os.MkdirAll(d, 0755)
		}
	}
}
