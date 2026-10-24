package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch strings.ToLower(cfg.Database.Driver) {
	case "sqlite", "sqlite3":
		dir := filepath.Dir(cfg.Database.DSN)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create sqlite dir: %w", err)
			}
		}
		// Enable WAL mode and busy timeout for concurrent read/write stability
		dsn := cfg.Database.DSN
		if !strings.Contains(dsn, "?") {
			dsn += "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
		}
		dialector = sqlite.Open(dsn)
	case "postgres", "postgresql":
		dialector = postgres.Open(cfg.Database.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	logLevel := logger.Warn
	if cfg.Server.Mode == "debug" {
		logLevel = logger.Info
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto Migrate tables
	err = db.AutoMigrate(
		&model.User{},
		&model.Group{},
		&model.Session{},
		&model.ApiToken{},
		&model.TOTPSecret{},
		&model.WebAuthnCredential{},
		&model.RecoveryCode{},
		&model.StorageBackend{},
		&model.StoragePolicy{},
		&model.FileRecord{},
		&model.WatermarkTemplate{},
		&model.ImagePreset{},
		&model.ImportJob{},
		&model.Setting{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to auto migrate database tables: %w", err)
	}

	// Seed defaults
	if err := seedDefaults(db); err != nil {
		return nil, fmt.Errorf("failed to seed database defaults: %w", err)
	}

	return db, nil
}

func seedDefaults(db *gorm.DB) error {
	// 1. Built-in User Groups
	var count int64
	db.Model(&model.Group{}).Count(&count)
	if count == 0 {
		defaultGroups := []model.Group{
			{
				ID:                  1,
				Name:                "Administrator",
				IsBuiltin:           true,
				Permissions:         `["*"]`,
				DefaultStorageLimit: 0, // 0 = unlimited
			},
			{
				ID:                  2,
				Name:                "Uploader",
				IsBuiltin:           true,
				Permissions:         `["file.upload","file.manage_own","image.convert","watermark.custom","token.create","import.use"]`,
				DefaultStorageLimit: 10737418240, // 10 GB
			},
		}
		for _, g := range defaultGroups {
			if err := db.FirstOrCreate(&g, model.Group{ID: g.ID}).Error; err != nil {
				return err
			}
		}
	}

	// 2. Default Local Storage Backend
	var backendCount int64
	db.Model(&model.StorageBackend{}).Count(&backendCount)
	if backendCount == 0 {
		defaultBackend := model.StorageBackend{
			ID:        1,
			Name:      "Local Storage",
			Driver:    "local",
			Config:    `{"base_path":"storage"}`,
			IsDefault: true,
		}
		if err := db.Create(&defaultBackend).Error; err != nil {
			return err
		}
	}

	// 3. Default Image Presets
	var presetCount int64
	db.Model(&model.ImagePreset{}).Count(&presetCount)
	if presetCount == 0 {
		defaultPresets := []model.ImagePreset{
			{Name: "thumb", Width: 200, Height: 200, Fit: "cover", Format: "webp", Quality: 80},
			{Name: "small", Width: 480, Height: 0, Fit: "contain", Format: "webp", Quality: 85},
			{Name: "medium", Width: 1080, Height: 0, Fit: "contain", Format: "webp", Quality: 85},
			{Name: "large", Width: 1920, Height: 0, Fit: "contain", Format: "webp", Quality: 88},
		}
		for _, p := range defaultPresets {
			db.Create(&p)
		}
	}

	return nil
}
