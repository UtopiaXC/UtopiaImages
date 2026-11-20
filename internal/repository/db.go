package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		&model.ImageCallStat{},
		&model.ImageCallVisitor{},
		&model.FilePolicyReplica{},
		&model.FileBlob{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to auto migrate database tables: %w", err)
	}

	// Remove legacy unique constraint on storage policy name to allow duplicate display names
	if db.Migrator().HasIndex(&model.StoragePolicy{}, "idx_storage_policies_name") {
		_ = db.Migrator().DropIndex(&model.StoragePolicy{}, "idx_storage_policies_name")
	}

	// Compatibility fix for legacy require2_fa column in users table
	if db.Migrator().HasColumn(&model.User{}, "require2_fa") && !db.Migrator().HasColumn(&model.User{}, "require_2fa") {
		_ = db.Migrator().RenameColumn(&model.User{}, "require2_fa", "require_2fa")
	}

	// Backfill existing FileRecord entries into FilePolicyReplica
	var unmigratedFiles []model.FileRecord
	_ = db.Unscoped().
		Where("id NOT IN (SELECT file_id FROM file_policy_replicas)").
		FindInBatches(&unmigratedFiles, 500, func(tx *gorm.DB, batch int) error {
			replicas := make([]model.FilePolicyReplica, 0, len(unmigratedFiles))
			for _, f := range unmigratedFiles {
				replicas = append(replicas, model.FilePolicyReplica{
					FileID:     f.ID,
					PolicyID:   f.PolicyID,
					PublicPath: f.PublicPath,
					StorageKey: f.StorageKey,
					Size:       f.Size,
					Status:     "ready",
					CreatedAt:  f.CreatedAt,
					UpdatedAt:  f.UpdatedAt,
				})
			}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&replicas).Error
		}).Error

	// Seed defaults
	if err := SeedDefaults(db); err != nil {
		return nil, fmt.Errorf("failed to seed database defaults: %w", err)
	}

	// Seed Demo defaults (fixed admin, guest, registration & square enabled, SMTP disabled)
	if err := SeedDemoDefaults(db); err != nil {
		return nil, fmt.Errorf("failed to seed demo defaults: %w", err)
	}

	return db, nil
}

// SeedDefaults initializes default roles, policies, image presets and system settings
func SeedDefaults(db *gorm.DB) error {
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

	// Migrate legacy policy base path config if needed
	db.Model(&model.StoragePolicy{}).Where("config = '{\"base_path\":\"storage\"}'").Update("config", `{"base_path":"data/storage"}`)

	// Ensure Role 1 has Policy 1, Role 2 has Policy 2 if existing
	var adminP, userP model.StoragePolicy
	if err := db.Where("id = 1").First(&adminP).Error; err == nil {
		db.FirstOrCreate(&model.RolePolicy{RoleID: 1, PolicyID: adminP.ID}, model.RolePolicy{RoleID: 1, PolicyID: adminP.ID})
	}
	if err := db.Where("id = 2").First(&userP).Error; err == nil {
		db.FirstOrCreate(&model.RolePolicy{RoleID: 2, PolicyID: userP.ID}, model.RolePolicy{RoleID: 2, PolicyID: userP.ID})
	}

	// Default System Settings
	db.FirstOrCreate(&model.Setting{Key: "allow_registration", Value: "true"}, model.Setting{Key: "allow_registration"})
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

// SeedDemoDefaults initializes the fixed demo administrator, guest account, and demo settings.
func SeedDemoDefaults(db *gorm.DB) error {
	// 1. Ensure fixed admin user (username: admin, password: admin, email: example@example.com)
	adminHash, err := auth.HashPassword("admin")
	if err != nil {
		return fmt.Errorf("failed to hash demo admin password: %w", err)
	}
	adminEmail := "example@example.com"

	var admin model.User
	err = db.Unscoped().Where("id = 1 OR LOWER(username) = 'admin'").Order("id ASC").First(&admin).Error
	if err != nil {
		admin = model.User{
			ID:           1,
			Username:     "admin",
			Nickname:     "管理员",
			Email:        &adminEmail,
			PasswordHash: adminHash,
			RoleID:       1,
			Status:       "active",
			Require2FA:   false,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := db.Create(&admin).Error; err != nil {
			return err
		}
	} else {
		if err := db.Unscoped().Model(&admin).Updates(map[string]any{
			"username":              "admin",
			"nickname":              "管理员",
			"email":                 &adminEmail,
			"password_hash":         adminHash,
			"role_id":               1,
			"status":                "active",
			"require_2fa":           false,
			"failed_login_attempts": 0,
			"lockout_until":         nil,
			"deleted_at":            nil,
		}).Error; err != nil {
			return err
		}
	}

	// Ensure only this single admin user exists in the system
	db.Model(&model.User{}).Where("role_id = 1 AND id != ?", admin.ID).Update("role_id", 2)
	// Ensure demo admin has no 2FA or passkeys
	db.Where("user_id = ?", admin.ID).Delete(&model.TOTPSecret{})
	db.Where("user_id = ?", admin.ID).Delete(&model.WebAuthnCredential{})
	db.Where("user_id = ?", admin.ID).Delete(&model.RecoveryCode{})

	// 2. Ensure guest role and guest user exist (guest upload enabled by default)
	var guestRole model.Role
	if err := db.Where("name = ?", "guest").First(&guestRole).Error; err != nil {
		guestRole = model.Role{
			Name:                "guest",
			DisplayName:         "游客",
			QuotaBytes:          1073741824, // 1 GB
			AllowedFormats:      "jpg,jpeg,png,gif,heif,webp",
			MaxSingleFileSizeMB: 5,
			IsSystem:            false,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}
		if err := db.Create(&guestRole).Error; err != nil {
			return err
		}
		var targetPolicyID uint
		var userRole model.Role
		if db.Preload("Policies").Where("name = 'user' OR id = 2").First(&userRole).Error == nil && len(userRole.Policies) > 0 {
			targetPolicyID = userRole.Policies[0].ID
		} else {
			var defaultPolicy model.StoragePolicy
			if db.Order("id ASC").First(&defaultPolicy).Error == nil {
				targetPolicyID = defaultPolicy.ID
			}
		}
		if targetPolicyID > 0 {
			db.FirstOrCreate(&model.RolePolicy{RoleID: guestRole.ID, PolicyID: targetPolicyID}, model.RolePolicy{RoleID: guestRole.ID, PolicyID: targetPolicyID})
		}
	}

	var guestUser model.User
	if err := db.Where("username = ?", "guest").First(&guestUser).Error; err != nil {
		guestUser = model.User{
			Username:     "guest",
			Nickname:     "游客",
			RoleID:       guestRole.ID,
			PasswordHash: "$2a$10$LockedGuestAccountNoPasswordAllowed0000000000000000",
			Status:       "active",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := db.Create(&guestUser).Error; err != nil {
			return err
		}
	}

	// 3. Default open settings for Demo (guest upload, square enabled)
	defaultOnSettings := []model.Setting{
		{Key: "enable_guest_upload", Value: "true"},
		{Key: "enable_square", Value: "true"},
		{Key: "default_open_page", Value: "guest_upload"},
		{Key: "captcha_provider", Value: "altcha"},
	}
	for _, s := range defaultOnSettings {
		db.FirstOrCreate(&s, model.Setting{Key: s.Key})
	}

	// 4. Locked demo settings (System tab + SMTP disabled + security/file bounds + registration defaults)
	enforcedSettings := []model.Setting{
		{Key: "smtp_enabled", Value: "false"},
		{Key: "smtp_host", Value: ""},
		{Key: "smtp_port", Value: "465"},
		{Key: "smtp_encryption", Value: "ssl"},
		{Key: "smtp_from", Value: ""},
		{Key: "smtp_from_name", Value: "Utopia Images"},
		{Key: "smtp_user", Value: ""},
		{Key: "smtp_password", Value: ""},
		{Key: "require_2fa", Value: "false"},
		{Key: "allow_registration", Value: "true"},
		{Key: "default_register_role_id", Value: "2"},
		{Key: "site_name", Value: "Utopia Images"},
		{Key: "site_description", Value: "又一个Utopia Images自部署图床"},
		{Key: "restrict_domains", Value: "false"},
		{Key: "log_level", Value: "info"},
		{Key: "log_permanent_level", Value: "error"},
		{Key: "log_max_size_mb", Value: "20"},
		{Key: "log_max_backups", Value: "10"},
		{Key: "log_max_age_days", Value: "30"},
		{Key: "derived_cache_max_size_mb", Value: "5120"},
		{Key: "derived_cache_ttl_days", Value: "0"},
		{Key: "global_call_stat", Value: "true"},
		{Key: "max_single_file_size", Value: "50"},
		{Key: "max_process_image_size", Value: "5"},
	}
	for _, s := range enforcedSettings {
		s.UpdatedAt = time.Now()
		db.Save(&s)
	}
	db.Where("key = 'site_favicon_binary'").Delete(&model.Setting{})
	db.FirstOrCreate(&model.Setting{
		Key:   "demo_last_reset_at",
		Value: strconv.FormatInt(time.Now().UnixMilli(), 10),
	}, model.Setting{Key: "demo_last_reset_at"})

	return nil
}

// GetDemoLastResetAt returns the persisted last reset timestamp from the database.
func GetDemoLastResetAt(db *gorm.DB) (time.Time, bool) {
	if db == nil {
		return time.Time{}, false
	}
	var s model.Setting
	if err := db.Where("key = 'demo_last_reset_at'").First(&s).Error; err != nil {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(s.Value), 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

// ResetDatabase wipes and reinitializes the database back to its initial demo state.
func ResetDatabase(cfg *config.Config, curDB *gorm.DB) (*gorm.DB, error) {
	driver := strings.ToLower(cfg.Database.Driver)
	if driver == "" {
		driver = "sqlite"
	}

	if curDB != nil {
		if driver != "sqlite" && driver != "sqlite3" {
			_ = curDB.Migrator().DropTable(
				&model.ImageCallVisitor{},
				&model.ImageCallStat{},
				&model.FilePolicyReplica{},
				&model.FileBlob{},
				&model.TemporaryLink{},
				&model.FileRecord{},
				&model.ImportJob{},
				&model.WatermarkTemplate{},
				&model.ImagePreset{},
				&model.PasswordResetToken{},
				&model.RecoveryCode{},
				&model.WebAuthnCredential{},
				&model.TOTPSecret{},
				&model.ApiToken{},
				&model.Session{},
				&model.Setting{},
				&model.RolePolicy{},
				&model.User{},
				&model.StoragePolicy{},
				&model.Role{},
			)
		}
		if sqlDB, err := curDB.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	}

	if driver == "sqlite" || driver == "sqlite3" {
		dsn := cfg.Database.DSN
		if dsn != "" {
			_ = os.Remove(dsn)
			_ = os.Remove(dsn + "-wal")
			_ = os.Remove(dsn + "-shm")
			_ = os.Remove(dsn + "-journal")
		}
	}

	return InitDB(cfg)
}

// ConnectTargetDB connects to a specified database driver and dsn (used for setup wizard testing)
func ConnectTargetDB(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch strings.ToLower(driver) {
	case "sqlite", "sqlite3":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(dsn)), "file:") {
			return nil, fmt.Errorf("invalid sqlite dsn: file uri scheme not allowed")
		}
		cleanDSN := filepath.Clean(dsn)
		if strings.Contains(cleanDSN, "..") {
			return nil, fmt.Errorf("invalid sqlite dsn path: directory traversal not allowed")
		}
		if filepath.IsAbs(cleanDSN) {
			absData, _ := filepath.Abs("data")
			rel, err := filepath.Rel(absData, cleanDSN)
			if err != nil || strings.HasPrefix(rel, "..") {
				return nil, fmt.Errorf("sqlite database path must be within data directory")
			}
		} else {
			slash := filepath.ToSlash(cleanDSN)
			if slash != "data" && !strings.HasPrefix(slash, "data/") {
				return nil, fmt.Errorf("sqlite database path must be within data directory: %s", cleanDSN)
			}
		}
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
