package http

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/common"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/mailer"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingsController struct {
	db             *gorm.DB
	derivedCache   *cache.DerivedCache
	cfg            *config.Config
	configPath     string
	passkeyManager *auth.PasskeyManager
}

func NewSettingsController(db *gorm.DB, dc *cache.DerivedCache, cfg *config.Config, configPath string, pm *auth.PasskeyManager) *SettingsController {
	return &SettingsController{
		db:             db,
		derivedCache:   dc,
		cfg:            cfg,
		configPath:     configPath,
		passkeyManager: pm,
	}
}

func (ctrl *SettingsController) SetDB(db *gorm.DB) {
	ctrl.db = db
}

func (ctrl *SettingsController) SetPasskeyManager(pm *auth.PasskeyManager) {
	ctrl.passkeyManager = pm
}

func (ctrl *SettingsController) RegisterRoutes(rg *gin.RouterGroup) {
	// 1. User preferences & profile settings
	me := rg.Group("/me")
	me.Use(auth.RequireAuth())
	{
		me.GET("/settings", ctrl.GetUserSettings)
		me.POST("/settings", ctrl.SaveUserSettings)
		me.GET("/quota", ctrl.GetUserQuota)
		me.GET("/policies", ctrl.GetUserPolicies)
		me.PATCH("/profile", ctrl.UpdateProfile)
		me.POST("/password", auth.RequireSessionAuth(), ctrl.ChangePassword)
	}

	// 2. Admin system settings
	admin := rg.Group("/admin")
	admin.Use(auth.RequireAuth(), auth.RequireAdmin())
	{
		admin.GET("/settings", ctrl.GetSystemSettings)
		admin.POST("/settings", ctrl.SaveSystemSettings)
		admin.GET("/cache/stats", ctrl.GetCacheStats)
		admin.POST("/cache/clear", ctrl.ClearCache)
		admin.POST("/cache/prune", ctrl.PruneCache)
		admin.POST("/favicon", ctrl.UploadFavicon)
		admin.DELETE("/favicon", ctrl.DeleteFavicon)
		admin.POST("/smtp/test", ctrl.TestSMTPConnection)
	}
}

func (ctrl *SettingsController) fetchPoliciesForUser(user *model.User) []model.StoragePolicy {
	var policies []model.StoragePolicy
	if user == nil {
		return policies
	}

	isAdmin := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if isAdmin {
		ctrl.db.Order("id ASC").Find(&policies)
		return policies
	}

	// 1. Policies specifically assigned to user's role OR policies with no role restrictions
	ctrl.db.Table("storage_policies").
		Where("id IN (SELECT policy_id FROM role_policies WHERE role_id = ?) OR id NOT IN (SELECT policy_id FROM role_policies)", user.RoleID).
		Order("id ASC").
		Find(&policies)

	// 2. Fallback: if none matched, return default policies, or all policies
	if len(policies) == 0 {
		ctrl.db.Where("is_default = ?", true).Order("id ASC").Find(&policies)
		if len(policies) == 0 {
			ctrl.db.Order("id ASC").Find(&policies)
		}
	}
	return policies
}

func (ctrl *SettingsController) GetUserSettings(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)

	var user model.User
	ctrl.db.Preload("Role").Where("id = ?", userID).First(&user)

	// Fetch user storage policies permitted for user's role
	policies := ctrl.fetchPoliciesForUser(&user)

	// Fetch user watermarks
	var watermarks []model.WatermarkTemplate
	ctrl.db.Where("owner_id = ? OR owner_id IS NULL", userID).Find(&watermarks)

	resp := gin.H{
		"watermark_mode":          ctrl.getSettingVal("user_wm_mode_"+strconvUint(userID), "inherit"),
		"watermark_template_id":   ctrl.getSettingVal("user_wm_tmpl_"+strconvUint(userID), ""),
		"default_link_format":     ctrl.getSettingVal("user_link_fmt_"+strconvUint(userID), "markdown"),
		"default_policy_id":       ctrl.getSettingVal("user_default_policy_"+strconvUint(userID), "1"),
		"hotlink_enabled":         user.HotlinkEnabled,
		"hotlink_allow_empty":     user.HotlinkAllowEmpty,
		"hotlink_allowed_domains": user.HotlinkAllowedDomains,
		"available_policies":      policies,
		"available_watermarks":    watermarks,
	}

	c.JSON(http.StatusOK, resp)
}

func (ctrl *SettingsController) SaveUserSettings(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)

	var req struct {
		WatermarkMode         string  `json:"watermark_mode"`
		WatermarkTemplateID   string  `json:"watermark_template_id"`
		DefaultLinkFormat     string  `json:"default_link_format"`
		DefaultPolicyID       string  `json:"default_policy_id"`
		HotlinkEnabled        *bool   `json:"hotlink_enabled"`
		HotlinkAllowEmpty     *bool   `json:"hotlink_allow_empty"`
		HotlinkAllowedDomains *string `json:"hotlink_allowed_domains"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.WatermarkMode != "" {
		if req.WatermarkMode != "on" && req.WatermarkMode != "off" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark_mode, must be on or off"})
			return
		}
		ctrl.setSettingVal("user_wm_mode_"+strconvUint(userID), req.WatermarkMode)
	}
	if req.WatermarkTemplateID != "" {
		ctrl.setSettingVal("user_wm_tmpl_"+strconvUint(userID), req.WatermarkTemplateID)
	}
	if req.WatermarkMode != "" || req.WatermarkTemplateID != "" {
		if ctrl.derivedCache != nil {
			ctrl.derivedCache.Clear()
		}
		ctrl.db.Model(&model.FileRecord{}).Where("user_id = ?", userID).Update("sign_version", gorm.Expr("sign_version + 1"))
	}
	if req.DefaultLinkFormat != "" {
		ctrl.setSettingVal("user_link_fmt_"+strconvUint(userID), req.DefaultLinkFormat)
	}
	if req.DefaultPolicyID != "" {
		ctrl.setSettingVal("user_default_policy_"+strconvUint(userID), req.DefaultPolicyID)
	}

	// Update user hotlink settings on model.User
	userUpdates := map[string]any{}
	if req.HotlinkEnabled != nil {
		userUpdates["hotlink_enabled"] = *req.HotlinkEnabled
	}
	if req.HotlinkAllowEmpty != nil {
		userUpdates["hotlink_allow_empty"] = *req.HotlinkAllowEmpty
	}
	if req.HotlinkAllowedDomains != nil {
		userUpdates["hotlink_allowed_domains"] = *req.HotlinkAllowedDomains
	}
	if len(userUpdates) > 0 {
		ctrl.db.Model(&model.User{}).Where("id = ?", userID).Updates(userUpdates)
	}

	c.JSON(http.StatusOK, gin.H{"message": "User settings updated"})
}

func (ctrl *SettingsController) GetUserQuota(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var current model.User
	ctrl.db.Where("id = ?", user.ID).First(&current)

	var role model.Role
	ctrl.db.Where("id = ?", current.RoleID).First(&role)

	var totalSize int64
	var count int64
	ctrl.db.Model(&model.FileRecord{}).Where("user_id = ? AND deleted_at IS NULL", user.ID).Select("COALESCE(SUM(size), 0)").Scan(&totalSize)
	ctrl.db.Model(&model.FileRecord{}).Where("user_id = ? AND deleted_at IS NULL", user.ID).Count(&count)

	if current.UsedBytes != totalSize || current.FileCount != count {
		ctrl.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"used_bytes": totalSize,
			"file_count": count,
		})
		current.UsedBytes = totalSize
		current.FileCount = count
	}

	c.JSON(http.StatusOK, gin.H{
		"used_bytes":  current.UsedBytes,
		"quota_bytes": role.QuotaBytes,
		"file_count":  current.FileCount,
		"role_name":   role.DisplayName,
	})
}

func (ctrl *SettingsController) GetUserPolicies(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user, ok := uVal.(*model.User)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	policies := ctrl.fetchPoliciesForUser(user)
	c.JSON(http.StatusOK, policies)
}

func (ctrl *SettingsController) UpdateProfile(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)

	var req struct {
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Email != "" {
		em := strings.TrimSpace(req.Email)
		updates["email"] = &em
	}

	if len(updates) > 0 {
		if err := ctrl.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": common.SanitizeError(err)})
			return
		}
	}

	var user model.User
	ctrl.db.Where("id = ?", userID).First(&user)
	c.JSON(http.StatusOK, user)
}

func (ctrl *SettingsController) ChangePassword(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify old password
	valid, err := auth.VerifyPassword(req.OldPassword, user.PasswordHash)
	if err != nil || !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Current password incorrect"})
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash new password"})
		return
	}

	ctrl.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"password_hash": newHash,
		"token_version": gorm.Expr("token_version + 1"),
	})

	// Revoke all existing sessions
	ctrl.db.Where("user_id = ?", user.ID).Delete(&model.Session{})

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

var allowedSystemSettings = map[string]bool{
	"site_name":                 true,
	"site_description":          true,
	"base_url":                  true,
	"max_single_file_size":      true,
	"max_process_image_size":    true,
	"captcha_provider":          true,
	"session_timeout_days":      true,
	"require_2fa":               true,
	"enable_format_conversion":  true,
	"default_convert_format":    true,
	"convert_quality":           true,
	"enable_hotlink":            true,
	"hotlink_allow_empty":       true,
	"hotlink_allowed_domains":   true,
	"hotlink_action":            true,
	"log_level":                 true,
	"log_permanent_level":       true,
	"log_max_size_mb":           true,
	"log_max_backups":           true,
	"log_max_age_days":          true,
	"derived_cache_max_size_mb": true,
	"derived_cache_ttl_days":    true,
	"allow_registration":        true,
	"default_register_role_id":  true,
	"restrict_domains":          true,
	"cdn_cache_control":         true,
	"smtp_enabled":              true,
	"smtp_host":                 true,
	"smtp_port":                 true,
	"smtp_encryption":           true,
	"smtp_from":                 true,
	"smtp_from_name":            true,
	"smtp_user":                 true,
	"smtp_password":             true,
}

func (ctrl *SettingsController) GetSystemSettings(c *gin.Context) {
	var settings []model.Setting
	ctrl.db.Find(&settings)

	res := map[string]string{
		"site_name":                 "Utopia Images",
		"site_description":          "Next-generation self-hosted image & static asset host",
		"base_url":                  ResolveBaseURL(c, ctrl.db, ""),
		"max_single_file_size":      "50", // MB
		"max_process_image_size":    "5",  // MB
		"captcha_provider":          "altcha",
		"session_timeout_days":      "7",
		"require_2fa":               "false",
		"enable_format_conversion":  "true",
		"default_convert_format":    "webp",
		"convert_quality":           "85",
		"enable_hotlink":            "false",
		"hotlink_allow_empty":       "true",
		"hotlink_allowed_domains":   "",
		"hotlink_action":            "forbidden",
		"restrict_domains":          "false",
		"cdn_cache_control":         "false",
		"log_level":                 appLogger.GetLevel(),
		"log_permanent_level":       appLogger.GetPermanentLevel(),
		"log_max_size_mb":           "20",
		"log_max_backups":           "10",
		"log_max_age_days":          "30",
		"derived_cache_max_size_mb": "5120",
		"derived_cache_ttl_days":    "0",
		"allow_registration":        "false",
		"default_register_role_id":  "2",
		"smtp_enabled":              "false",
		"smtp_host":                 "",
		"smtp_port":                 "465",
		"smtp_encryption":           "ssl",
		"smtp_from":                 "",
		"smtp_from_name":            "Utopia Images",
		"smtp_user":                 "",
		"smtp_password":             "",
	}

	for _, s := range settings {
		res[s.Key] = s.Value
	}

	if ctrl.cfg != nil {
		if ctrl.cfg.Cache.MaxSizeMB >= 0 {
			res["derived_cache_max_size_mb"] = strconv.FormatInt(ctrl.cfg.Cache.MaxSizeMB, 10)
		}
		if ctrl.cfg.Cache.TTLDays >= 0 {
			res["derived_cache_ttl_days"] = strconv.Itoa(ctrl.cfg.Cache.TTLDays)
		}
		if ctrl.cfg.SMTP.Host != "" {
			res["smtp_host"] = ctrl.cfg.SMTP.Host
		}
		if ctrl.cfg.SMTP.Port > 0 {
			res["smtp_port"] = strconv.Itoa(ctrl.cfg.SMTP.Port)
		}
		if ctrl.cfg.SMTP.From != "" {
			res["smtp_from"] = ctrl.cfg.SMTP.From
		}
		if ctrl.cfg.SMTP.FromName != "" {
			res["smtp_from_name"] = ctrl.cfg.SMTP.FromName
		}
		if ctrl.cfg.SMTP.User != "" {
			res["smtp_user"] = ctrl.cfg.SMTP.User
		}
		if ctrl.cfg.SMTP.Password != "" {
			res["smtp_password"] = ctrl.cfg.SMTP.Password
		}
		if ctrl.cfg.SMTP.Encryption != "" {
			res["smtp_encryption"] = ctrl.cfg.SMTP.Encryption
		}
		res["smtp_enabled"] = strconv.FormatBool(ctrl.cfg.SMTP.Enabled)
	}

	if res["smtp_password"] != "" {
		res["smtp_password"] = "********"
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *SettingsController) SaveSystemSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	// Requirement 5: System must have SMTP configured to enable registration
	if val, ok := req["allow_registration"]; ok && (val == "true" || val == "1") {
		smtpEnabled := ctrl.cfg != nil && ctrl.cfg.SMTP.Enabled && ctrl.cfg.SMTP.Host != ""
		if reqSmtpEnabled, exists := req["smtp_enabled"]; exists {
			smtpEnabled = (reqSmtpEnabled == "true" || reqSmtpEnabled == "1") && req["smtp_host"] != ""
		}
		if !smtpEnabled {
			common.RespondError(c, http.StatusBadRequest, common.ErrRegistrationSMTPRequired)
			return
		}
	}

	for k, v := range req {
		if !allowedSystemSettings[k] || k == "site_favicon_binary" {
			continue
		}
		if k == "smtp_password" && v == "********" {
			continue
		}
		ctrl.setSettingVal(k, v)
	}

	if lvl, ok := req["log_level"]; ok && lvl != "" {
		appLogger.SetLevel(lvl)
		if ctrl.cfg != nil {
			ctrl.cfg.Logging.Level = lvl
		}
	}
	if permLvl, ok := req["log_permanent_level"]; ok && permLvl != "" {
		appLogger.SetPermanentLevel(permLvl)
		if ctrl.cfg != nil {
			ctrl.cfg.Logging.PermanentLevel = permLvl
		}
	}
	if maxSize, ok := req["log_max_size_mb"]; ok && maxSize != "" {
		if v, err := strconv.Atoi(maxSize); err == nil && ctrl.cfg != nil {
			ctrl.cfg.Logging.MaxSizeMB = v
		}
	}
	if maxBackups, ok := req["log_max_backups"]; ok && maxBackups != "" {
		if v, err := strconv.Atoi(maxBackups); err == nil && ctrl.cfg != nil {
			ctrl.cfg.Logging.MaxBackups = v
		}
	}
	if maxAge, ok := req["log_max_age_days"]; ok && maxAge != "" {
		if v, err := strconv.Atoi(maxAge); err == nil && ctrl.cfg != nil {
			ctrl.cfg.Logging.MaxAgeDays = v
		}
	}
	if siteName, ok := req["site_name"]; ok && siteName != "" && ctrl.cfg != nil {
		ctrl.cfg.Site.Name = siteName
	}
	if siteDesc, ok := req["site_description"]; ok && ctrl.cfg != nil {
		ctrl.cfg.Site.Description = siteDesc
	}
	if baseURL, ok := req["base_url"]; ok && baseURL != "" && ctrl.cfg != nil {
		ctrl.cfg.Site.BaseURL = baseURL
		ctrl.cfg.Server.BaseURL = baseURL
		if ctrl.passkeyManager != nil {
			_ = ctrl.passkeyManager.UpdateBaseURL(baseURL)
		}
	}

	if ctrl.cfg != nil {
		if val, ok := req["smtp_enabled"]; ok {
			ctrl.cfg.SMTP.Enabled = (val == "true" || val == "1")
		}
		if val, ok := req["smtp_host"]; ok {
			ctrl.cfg.SMTP.Host = val
		}
		if val, ok := req["smtp_port"]; ok && val != "" {
			if port, err := strconv.Atoi(val); err == nil {
				ctrl.cfg.SMTP.Port = port
			}
		}
		if val, ok := req["smtp_encryption"]; ok {
			ctrl.cfg.SMTP.Encryption = val
		}
		if val, ok := req["smtp_from"]; ok {
			ctrl.cfg.SMTP.From = val
		}
		if val, ok := req["smtp_from_name"]; ok {
			ctrl.cfg.SMTP.FromName = val
		}
		if val, ok := req["smtp_user"]; ok {
			ctrl.cfg.SMTP.User = val
		}
		if val, ok := req["smtp_password"]; ok && val != "********" {
			ctrl.cfg.SMTP.Password = val
		}
		if val, ok := req["derived_cache_max_size_mb"]; ok {
			ctrl.setSettingVal("derived_cache_max_size_mb", val)
			if mb, err := strconv.ParseInt(val, 10, 64); err == nil {
				ctrl.cfg.Cache.MaxSizeMB = mb
			}
		}
		if val, ok := req["derived_cache_ttl_days"]; ok {
			ctrl.setSettingVal("derived_cache_ttl_days", val)
			if days, err := strconv.Atoi(val); err == nil {
				ctrl.cfg.Cache.TTLDays = days
			}
		}
		if ctrl.derivedCache != nil {
			ctrl.derivedCache.SetLimits(ctrl.cfg.Cache.MaxSizeMB, ctrl.cfg.Cache.TTLDays)
			ctrl.derivedCache.PruneIfNecessary()
		}
	}

	if ctrl.cfg != nil && ctrl.configPath != "" {
		_ = config.SaveConfig(ctrl.configPath, ctrl.cfg)
	}

	c.JSON(http.StatusOK, gin.H{"message": "System settings saved successfully"})
}

func (ctrl *SettingsController) TestSMTPConnection(c *gin.Context) {
	var req struct {
		Host       string `json:"host" binding:"required"`
		Port       int    `json:"port" binding:"required"`
		From       string `json:"from"`
		User       string `json:"user"`
		Password   string `json:"password"`
		Encryption string `json:"encryption"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写完整的 SMTP 主机和端口"})
		return
	}

	password := req.Password
	if password == "********" || password == "" {
		if ctrl.cfg != nil && ctrl.cfg.SMTP.Password != "" {
			password = ctrl.cfg.SMTP.Password
		} else {
			password = ctrl.getSettingVal("smtp_password", "")
		}
	}

	testCfg := config.SMTPConfig{
		Enabled:    true,
		Host:       req.Host,
		Port:       req.Port,
		From:       req.From,
		User:       req.User,
		Password:   password,
		Encryption: req.Encryption,
	}

	if err := mailer.TestConnection(testCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "SMTP 服务器连接测试失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "SMTP 服务器连接测试成功",
	})
}

func (ctrl *SettingsController) ClearCache(c *gin.Context) {
	count := 0
	if ctrl.derivedCache != nil {
		count = ctrl.derivedCache.Clear()
	}
	ctrl.db.Model(&model.FileRecord{}).Where("1 = 1").Update("sign_version", gorm.Expr("sign_version + 1"))
	c.JSON(http.StatusOK, gin.H{
		"message": "Derived image cache cleared",
		"cleared": count,
	})
}

func (ctrl *SettingsController) GetCacheStats(c *gin.Context) {
	var totalBytes int64
	var fileCount int
	if ctrl.derivedCache != nil {
		totalBytes, fileCount, _ = ctrl.derivedCache.GetStats()
	}
	maxMBStr := ctrl.getSettingVal("derived_cache_max_size_mb", "5120")
	ttlDaysStr := ctrl.getSettingVal("derived_cache_ttl_days", "0")
	maxMB, _ := strconv.ParseInt(maxMBStr, 10, 64)
	ttlDays, _ := strconv.Atoi(ttlDaysStr)

	c.JSON(http.StatusOK, gin.H{
		"usage_bytes": totalBytes,
		"file_count":  fileCount,
		"max_size_mb": maxMB,
		"ttl_days":    ttlDays,
	})
}

func (ctrl *SettingsController) PruneCache(c *gin.Context) {
	maxMBStr := ctrl.getSettingVal("derived_cache_max_size_mb", "5120")
	ttlDaysStr := ctrl.getSettingVal("derived_cache_ttl_days", "0")
	maxMB, _ := strconv.ParseInt(maxMBStr, 10, 64)
	ttlDays, _ := strconv.Atoi(ttlDaysStr)

	var count int
	var freedBytes int64
	if ctrl.derivedCache != nil {
		maxBytes := maxMB * 1024 * 1024
		var maxAge time.Duration
		if ttlDays > 0 {
			maxAge = time.Duration(ttlDays) * 24 * time.Hour
		}
		count, freedBytes, _ = ctrl.derivedCache.Prune(maxBytes, maxAge)
	}

	c.JSON(http.StatusOK, gin.H{
		"cleared_count": count,
		"freed_bytes":   freedBytes,
		"message":       "Cache pruned successfully",
	})
}

func (ctrl *SettingsController) getSettingVal(key, def string) string {
	var s model.Setting
	if err := ctrl.db.Where("key = ?", key).First(&s).Error; err != nil {
		return def
	}
	return s.Value
}

func (ctrl *SettingsController) setSettingVal(key, val string) {
	if key == "site_favicon_binary" {
		return
	}
	var s model.Setting
	if err := ctrl.db.Where("key = ?", key).First(&s).Error; err == nil {
		s.Value = val
		ctrl.db.Save(&s)
	} else {
		s = model.Setting{
			Key:   key,
			Value: val,
		}
		ctrl.db.Save(&s)
	}
}

func strconvUint(u uint) string {
	return strconv.FormatUint(uint64(u), 10)
}

func (ctrl *SettingsController) UploadFavicon(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	if fileHeader.Size > 2*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Favicon file size must be less than 2MB"})
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read uploaded file"})
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file content"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	contentType := "image/x-icon"
	switch ext {
	case ".ico":
		contentType = "image/x-icon"
	case ".svg":
		contentType = "image/svg+xml"
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".webp":
		contentType = "image/webp"
	default:
		contentType = http.DetectContentType(data)
	}

	s := model.Setting{
		Key:         "site_favicon_binary",
		Value:       contentType,
		BinaryValue: data,
		ContentType: contentType,
		UpdatedAt:   time.Now(),
	}
	if err := ctrl.db.Save(&s).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save favicon to database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Favicon updated successfully",
		"url":     fmt.Sprintf("/favicon.ico?v=%d", time.Now().UnixNano()),
	})
}

func (ctrl *SettingsController) DeleteFavicon(c *gin.Context) {
	ctrl.db.Where("key = 'site_favicon_binary'").Delete(&model.Setting{})
	c.JSON(http.StatusOK, gin.H{
		"message": "Favicon restored to default",
		"url":     fmt.Sprintf("/favicon.svg?v=%d", time.Now().UnixNano()),
	})
}
