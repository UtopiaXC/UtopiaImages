package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	case "mysql":
		dialector = mysql.Open(cfg.Database.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: appLogger.NewGormLogger(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto Migrate tables
	err = db.AutoMigrate(
		&model.Role{},
		&model.StoragePolicy{},
		&model.RolePolicy{},
		&model.User{},
		&model.FileRecord{},
		&model.TemporaryLink{},
		&model.Session{},
		&model.ApiToken{},
		&model.TOTPSecret{},
		&model.WebAuthnCredential{},
		&model.RecoveryCode{},
		&model.WatermarkTemplate{},
		&model.ImagePreset{},
		&model.ImportJob{},
		&model.Setting{},
		&model.PasswordResetToken{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to auto migrate database tables: %w", err)
	}

	// Compatibility fix for legacy require2_fa column in users table
	if db.Migrator().HasColumn(&model.User{}, "require2_fa") && !db.Migrator().HasColumn(&model.User{}, "require_2fa") {
		_ = db.Migrator().RenameColumn(&model.User{}, "require2_fa", "require_2fa")
	}

	// Seed defaults
	if err := seedDefaults(db); err != nil {
		return nil, fmt.Errorf("failed to seed database defaults: %w", err)
	}

	return db, nil
}

func seedDefaults(db *gorm.DB) error {
	// 1. Built-in Roles (admin, user)
	var roleCount int64
	db.Model(&model.Role{}).Count(&roleCount)
	if roleCount == 0 {
		defaultRoles := []model.Role{
			{
				ID:          1,
				Name:        "admin",
				DisplayName: "系统管理员",
				QuotaBytes:  0, // 0 = unlimited
				IsSystem:    true,
			},
			{
				ID:          2,
				Name:        "user",
				DisplayName: "普通用户",
				QuotaBytes:  10737418240, // 10 GB
				IsSystem:    true,
			},
		}
		for _, r := range defaultRoles {
			if err := db.FirstOrCreate(&r, model.Role{ID: r.ID}).Error; err != nil {
				return err
			}
		}
	}
	// Ensure admin role quota is always 0 (unlimited)
	db.Model(&model.Role{}).Where("id = 1 OR name = 'admin'").Update("quota_bytes", 0)

	// 2. Default Local Storage Policies (Requirement 4: admin: uploads, user: {username}/uploads)
	var policyCount int64
	db.Model(&model.StoragePolicy{}).Count(&policyCount)
	if policyCount == 0 {
		adminPolicy := model.StoragePolicy{
			ID:               1,
			Name:             "管理员本地存储",
			Driver:           "local",
			Config:           `{"base_path":"data/storage"}`,
			MaxCapacityBytes: 0, // unlimited
			IsDefault:        true,
			PathPrefix:       "uploads",
			NamingRule:       "{Y}/{m}/{d}/{random:16}.{ext}",
		}
		userPolicy := model.StoragePolicy{
			ID:               2,
			Name:             "用户本地存储",
			Driver:           "local",
			Config:           `{"base_path":"data/storage"}`,
			MaxCapacityBytes: 10737418240, // 10 GB (Requirement 4)
			IsDefault:        true,
			PathPrefix:       "{username}/uploads",
			NamingRule:       "{Y}/{m}/{d}/{random:16}.{ext}",
		}
		if err := db.Create(&adminPolicy).Error; err != nil {
			return err
		}
		if err := db.Create(&userPolicy).Error; err != nil {
			return err
		}

		// Link policies: Role 1 -> Policy 1, Role 2 -> Policy 2
		db.FirstOrCreate(&model.RolePolicy{RoleID: 1, PolicyID: 1}, model.RolePolicy{RoleID: 1, PolicyID: 1})
		db.FirstOrCreate(&model.RolePolicy{RoleID: 2, PolicyID: 2}, model.RolePolicy{RoleID: 2, PolicyID: 2})
	}

	// Ensure policy base paths and capacities: User local storage is 10G, Admin is unlimited (0)
	db.Model(&model.StoragePolicy{}).Where("config = '{\"base_path\":\"storage\"}'").Update("config", `{"base_path":"data/storage"}`)
	db.Model(&model.StoragePolicy{}).Where("id = 1 OR name = '管理员本地存储'").Update("max_capacity_bytes", 0)
	db.Model(&model.StoragePolicy{}).Where("id = 2 OR name = '用户本地存储'").Update("max_capacity_bytes", 10737418240)

	// Ensure Role 1 has Policy 1, Role 2 has Policy 2 if existing
	var adminP, userP model.StoragePolicy
	if err := db.Where("id = 1").First(&adminP).Error; err == nil {
		db.FirstOrCreate(&model.RolePolicy{RoleID: 1, PolicyID: adminP.ID}, model.RolePolicy{RoleID: 1, PolicyID: adminP.ID})
	}
	if err := db.Where("id = 2").First(&userP).Error; err == nil {
		db.FirstOrCreate(&model.RolePolicy{RoleID: 2, PolicyID: userP.ID}, model.RolePolicy{RoleID: 2, PolicyID: userP.ID})
	}

	// Default System Settings
	db.FirstOrCreate(&model.Setting{Key: "allow_registration", Value: "false"}, model.Setting{Key: "allow_registration"})
	db.FirstOrCreate(&model.Setting{Key: "default_register_role_id", Value: "2"}, model.Setting{Key: "default_register_role_id"})
	db.FirstOrCreate(&model.Setting{Key: "log_permanent_level", Value: "error"}, model.Setting{Key: "log_permanent_level"})

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

// ConnectTargetDB connects to a specified database driver and dsn (used for setup wizard testing)
func ConnectTargetDB(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch strings.ToLower(driver) {
	case "sqlite", "sqlite3":
		dir := filepath.Dir(dsn)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create sqlite dir: %w", err)
			}
		}
		if !strings.Contains(dsn, "?") {
			dsn += "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
		}
		dialector = sqlite.Open(dsn)
	case "postgres", "postgresql":
		dialector = postgres.Open(dsn)
	case "mysql":
		dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}

	return gorm.Open(dialector, &gorm.Config{
		Logger: appLogger.NewGormLogger(),
	})
}
