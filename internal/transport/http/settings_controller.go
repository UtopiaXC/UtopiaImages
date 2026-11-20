package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/common"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingsController struct {
	db             *gorm.DB
	derivedCache   *cache.DerivedCache
	cfg            *config.Config
	configPath     string
	passkeyManager *auth.PasskeyManager
	uploadService  *service.UploadService
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

func (ctrl *SettingsController) SetUploadService(us *service.UploadService) {
	ctrl.uploadService = us
}

func (ctrl *SettingsController) RegisterRoutes(rg *gin.RouterGroup) {
	// 1. User preferences & profile settings
	me := rg.Group("/me")
	me.Use(auth.RequireAuth())
	{
		me.GET("/settings", ctrl.GetUserSettings)
		me.POST("/settings", auth.RequireSessionAuth(), ctrl.SaveUserSettings)
		me.GET("/quota", ctrl.GetUserQuota)
		me.GET("/policies", ctrl.GetUserPolicies)
		me.PATCH("/profile", auth.RequireSessionAuth(), ctrl.UpdateProfile)
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

	// Only return policies specifically permitted for user's role
	ctrl.db.Table("storage_policies").
		Where("id IN (SELECT policy_id FROM role_policies WHERE role_id = ?)", user.RoleID).
		Order("id ASC").
		Find(&policies)

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
	ctrl.db.Where("owner_id = ?", userID).Find(&watermarks)

	resp := gin.H{
		"watermark_mode":           ctrl.getSettingVal("user_wm_mode_"+strconvUint(userID), "inherit"),
		"watermark_template_id":    ctrl.getSettingVal("user_wm_tmpl_"+strconvUint(userID), ""),
		"default_link_format":      ctrl.getSettingVal("user_link_fmt_"+strconvUint(userID), "url"),
		"default_policy_id":        ctrl.getSettingVal("user_default_policy_"+strconvUint(userID), "1"),
		"default_landing_view":     ctrl.getSettingVal("user_landing_view_"+strconvUint(userID), "visitor"),
		"global_call_stat_enabled": ctrl.getSettingVal("global_call_stat", "true") == "true",
		"call_stat_enabled":        ctrl.getSettingVal("user_call_stat_"+strconvUint(userID), "false") == "true",
		"hotlink_enabled":          user.HotlinkEnabled,
		"hotlink_allow_empty":      user.HotlinkAllowEmpty,
		"hotlink_allowed_domains":  user.HotlinkAllowedDomains,
		"default_public":           user.DefaultPublic,
		"available_policies":       policies,
		"available_watermarks":     watermarks,
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
		DefaultLandingView    string  `json:"default_landing_view"`
		CallStatEnabled       *bool   `json:"call_stat_enabled"`
		HotlinkEnabled        *bool   `json:"hotlink_enabled"`
		HotlinkAllowEmpty     *bool   `json:"hotlink_allow_empty"`
		HotlinkAllowedDomains *string `json:"hotlink_allowed_domains"`
		DefaultPublic         *bool   `json:"default_public"`
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
		tmplID := ParseUintSafe(req.WatermarkTemplateID, 0)
		if tmplID > 0 {
			var count int64
			ctrl.db.Model(&model.WatermarkTemplate{}).
				Where("id = ? AND (owner_id = ? OR is_system = ?)", tmplID, userID, true).
				Count(&count)
			if count == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "无效或无权使用的水印模板"})
				return
			}
		}
		ctrl.setSettingVal("user_wm_tmpl_"+strconvUint(userID), req.WatermarkTemplateID)
	}
	if req.WatermarkMode != "" || req.WatermarkTemplateID != "" {
		ctrl.db.Model(&model.FileRecord{}).Where("user_id = ?", userID).Update("sign_version", gorm.Expr("sign_version + 1"))
	}
	if req.DefaultLinkFormat != "" {
		ctrl.setSettingVal("user_link_fmt_"+strconvUint(userID), req.DefaultLinkFormat)
	}
	if req.DefaultPolicyID != "" {
		ctrl.setSettingVal("user_default_policy_"+strconvUint(userID), req.DefaultPolicyID)
	}
	if req.DefaultLandingView != "" {
		ctrl.setSettingVal("user_landing_view_"+strconvUint(userID), req.DefaultLandingView)
	}
	if req.CallStatEnabled != nil {
		val := "false"
		if *req.CallStatEnabled {
			val = "true"
		}
		ctrl.setSettingVal("user_call_stat_"+strconvUint(userID), val)
	}

	// Update user settings on model.User
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
	if req.DefaultPublic != nil {
		userUpdates["default_public"] = *req.DefaultPublic
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
	ctrl.db.Model(&model.FileRecord{}).
		Joins("LEFT JOIN file_policy_replicas ON file_policy_replicas.file_id = file_records.id AND file_policy_replicas.status = 'ready'").
		Where("file_records.user_id = ? AND file_records.deleted_at IS NULL", user.ID).
		Select("COALESCE(SUM(COALESCE(file_policy_replicas.size, file_records.size)), 0)").
		Scan(&totalSize)
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
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := ctrl.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if service.IsDemoAdminUser(&user) {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoAdminLocked, "演示管理员账号禁止修改个人资料")
		return
	}

	updates := map[string]any{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Email != "" {
		em := strings.TrimSpace(req.Email)
		currentEmail := ""
		if user.Email != nil {
			currentEmail = *user.Email
		}
		if !strings.EqualFold(em, currentEmail) {
			// Changing email requires password verification if password is set
			if user.PasswordHash != "" {
				if req.Password == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "修改绑定邮箱需输入当前账户密码进行安全验证"})
					return
				}
				ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
				if err != nil || !ok {
					c.JSON(http.StatusBadRequest, gin.H{"error": "当前账户密码验证失败"})
					return
				}
			}
			updates["email"] = &em
		}
	}

	if len(updates) > 0 {
		if err := ctrl.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": common.SanitizeError(err)})
			return
		}
	}

	ctrl.db.Where("id = ?", userID).First(&user)
	c.JSON(http.StatusOK, user)
}

func (ctrl *SettingsController) ChangePassword(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	if service.IsDemoAdminUser(user) {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoAdminLocked, "演示管理员账号禁止修改登录密码")
		return
	}

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

	err = ctrl.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"password_hash": newHash,
			"token_version": gorm.Expr("token_version + 1"),
		}).Error; err != nil {
			return err
		}
		// Revoke all existing sessions and API tokens
		if err := tx.Where("user_id = ?", user.ID).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&model.ApiToken{}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

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
	"global_call_stat":          true,
	"smtp_enabled":              true,
	"smtp_host":                 true,
	"smtp_port":                 true,
	"smtp_encryption":           true,
	"smtp_from":                 true,
	"smtp_from_name":            true,
	"smtp_user":                 true,
	"smtp_password":             true,
	"enable_guest_upload":       true,
	"enable_square":             true,
	"default_open_page":         true,
	"guest_watermark_id":        true,
}

func (ctrl *SettingsController) GetSystemSettings(c *gin.Context) {
	var settings []model.Setting
	ctrl.db.Find(&settings)

	res := map[string]string{
		"site_name":                 "Utopia Images",
		"site_description":          "又一个Utopia Images自部署图床",
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
		"global_call_stat":          "true",
		"log_level":                 "info",
		"log_permanent_level":       "ERROR",
		"log_max_size_mb":           "20",
		"log_max_backups":           "10",
		"log_max_age_days":          "30",
		"derived_cache_max_size_mb": "5120",
		"derived_cache_ttl_days":    "0",
		"allow_registration":        "true",
		"default_register_role_id":  "2",
		"smtp_enabled":              "false",
		"smtp_host":                 "",
		"smtp_port":                 "465",
		"smtp_encryption":           "ssl",
		"smtp_from":                 "",
		"smtp_from_name":            "Utopia Images",
		"smtp_user":                 "",
		"smtp_password":             "",
		"enable_guest_upload":       "true",
		"enable_square":             "true",
		"default_open_page":         "guest_upload",
		"guest_watermark_id":        "",
	}

	for _, s := range settings {
		if allowedSystemSettings[s.Key] {
			res[s.Key] = s.Value
		}
	}

	// Enforce demo locked defaults in response
	res["site_name"] = "Utopia Images"
	res["site_description"] = "又一个Utopia Images自部署图床"
	res["restrict_domains"] = "false"
	res["log_level"] = "info"
	res["log_permanent_level"] = "ERROR"
	res["log_max_size_mb"] = "20"
	res["log_max_backups"] = "10"
	res["log_max_age_days"] = "30"
	res["derived_cache_max_size_mb"] = "5120"
	res["derived_cache_ttl_days"] = "0"
	res["global_call_stat"] = "true"
	res["allow_registration"] = "true"
	res["default_register_role_id"] = "2"
	res["max_single_file_size"] = "50"
	res["max_process_image_size"] = "5"
	res["require_2fa"] = "false"
	res["smtp_enabled"] = "false"
	res["smtp_host"] = ""
	res["smtp_port"] = "465"
	res["smtp_encryption"] = "ssl"
	res["smtp_from"] = ""
	res["smtp_from_name"] = "Utopia Images"
	res["smtp_user"] = ""
	res["smtp_password"] = ""

	c.JSON(http.StatusOK, res)
}

var demoLockedSystemTabDefaults = map[string]string{
	"site_name":                 "Utopia Images",
	"site_description":          "又一个Utopia Images自部署图床",
	"restrict_domains":          "false",
	"log_level":                 "info",
	"log_permanent_level":       "error",
	"log_max_size_mb":           "20",
	"log_max_backups":           "10",
	"log_max_age_days":          "30",
	"derived_cache_max_size_mb": "5120",
	"derived_cache_ttl_days":    "0",
	"global_call_stat":          "true",
}

func isDemoBoolFalse(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return s == "" || s == "false" || s == "0"
}

func isDemoBoolTrue(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return s == "true" || s == "1"
}

func (ctrl *SettingsController) SaveSystemSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	// Demo validation 1: System tab options are all locked to defaults except base_url
	for k, defVal := range demoLockedSystemTabDefaults {
		if reqVal, ok := req[k]; ok {
			if k == "restrict_domains" {
				if !isDemoBoolFalse(reqVal) {
					common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改系统选项卡配置（仅允许设置对外访问域名）")
					return
				}
			} else if k == "global_call_stat" {
				if !isDemoBoolTrue(reqVal) {
					common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改系统选项卡配置（仅允许设置对外访问域名）")
					return
				}
			} else if k == "log_level" || k == "log_permanent_level" {
				if !strings.EqualFold(strings.TrimSpace(reqVal), defVal) {
					common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改系统选项卡配置（仅允许设置对外访问域名）")
					return
				}
			} else if strings.TrimSpace(reqVal) != defVal {
				common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改系统选项卡配置（仅允许设置对外访问域名）")
				return
			}
			delete(req, k)
		}
	}

	// Demo validation 2: SMTP is completely disabled and decoupled from registration
	if val, ok := req["smtp_enabled"]; ok && !isDemoBoolFalse(val) {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSMTPDisabled, "演示环境已禁用 SMTP 邮件服务")
		return
	}
	for _, smtpKey := range []string{"smtp_enabled", "smtp_host", "smtp_port", "smtp_encryption", "smtp_from", "smtp_from_name", "smtp_user", "smtp_password"} {
		delete(req, smtpKey)
	}

	// Demo validation 3: Enforce safe file limits, disable global forced 2FA, and disallow Turnstile without keys
	if v, ok := req["require_2fa"]; ok && !isDemoBoolFalse(v) {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止开启全站强制两步验证")
		return
	}
	delete(req, "require_2fa")

	if v, ok := req["max_single_file_size"]; ok && strings.TrimSpace(v) != "50" {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改单文件大小上限")
		return
	}
	delete(req, "max_single_file_size")

	if v, ok := req["max_process_image_size"]; ok && strings.TrimSpace(v) != "5" {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改动态处理上限")
		return
	}
	delete(req, "max_process_image_size")

	if v, ok := req["captcha_provider"]; ok && strings.EqualFold(strings.TrimSpace(v), "turnstile") {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境不支持切换为 Turnstile 验证码")
		return
	}

	if v, ok := req["allow_registration"]; ok && !isDemoBoolTrue(v) {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境固定开启用户自主注册")
		return
	}
	delete(req, "allow_registration")

	if defaultRoleStr, ok := req["default_register_role_id"]; ok && strings.TrimSpace(defaultRoleStr) != "" && strings.TrimSpace(defaultRoleStr) != "2" {
		common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改新注册用户默认角色")
		return
	}
	delete(req, "default_register_role_id")

	// Auto-adjust default_open_page when guest_upload or square status changes
	enableGuest := req["enable_guest_upload"] == "true" || req["enable_guest_upload"] == "1"
	if _, exists := req["enable_guest_upload"]; !exists {
		enableGuest = ctrl.getSettingVal("enable_guest_upload", "false") == "true"
	}
	enableSquare := req["enable_square"] == "true" || req["enable_square"] == "1"
	if _, exists := req["enable_square"]; !exists {
		enableSquare = ctrl.getSettingVal("enable_square", "false") == "true"
	}
	curDefault, existsDefault := req["default_open_page"]
	if !existsDefault {
		curDefault = ctrl.getSettingVal("default_open_page", "guest_upload")
	}

	if !enableGuest && !enableSquare {
		req["default_open_page"] = "login"
	} else if !enableGuest && (curDefault == "guest_upload" || curDefault == "guest_gallery") {
		if enableSquare {
			req["default_open_page"] = "square"
		} else {
			req["default_open_page"] = "login"
		}
	} else if !enableSquare && curDefault == "square" {
		if enableGuest {
			req["default_open_page"] = "guest_upload"
		} else {
			req["default_open_page"] = "login"
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

	if guestUploadVal, exists := req["enable_guest_upload"]; exists {
		cleanGuest := req["clean_guest_data"] == "true" || req["clean_guest_data"] == "1"
		_ = ctrl.handleGuestUploadChange(guestUploadVal, cleanGuest)
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
	common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSMTPDisabled, "演示环境已禁用 SMTP 邮件服务")
}

func (ctrl *SettingsController) ClearCache(c *gin.Context) {
	common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改系统选项卡配置")
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
	common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoSystemSettingLocked, "演示环境禁止修改系统选项卡配置")
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
	common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoFaviconLocked, "演示环境禁止修改网站图标")
}

func (ctrl *SettingsController) DeleteFavicon(c *gin.Context) {
	common.RespondDemoError(c, http.StatusForbidden, common.ErrDemoFaviconLocked, "演示环境禁止修改网站图标")
}

func (ctrl *SettingsController) handleGuestUploadChange(enableVal string, cleanGuestData bool) error {
	if enableVal == "true" || enableVal == "1" {
		var guestRole model.Role
		err := ctrl.db.Where("name = ?", "guest").First(&guestRole).Error
		if err != nil {
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
			if err := ctrl.db.Create(&guestRole).Error; err != nil {
				return err
			}
			var userRole model.Role
			ctrl.db.Preload("Policies").Where("name = 'user' OR id = 2").First(&userRole)
			var targetPolicyID uint = 0
			if len(userRole.Policies) > 0 {
				targetPolicyID = userRole.Policies[0].ID
			} else {
				var defaultPolicy model.StoragePolicy
				if err := ctrl.db.Where("is_default = ?", true).First(&defaultPolicy).Error; err == nil {
					targetPolicyID = defaultPolicy.ID
				} else {
					ctrl.db.Order("id ASC").First(&defaultPolicy)
					targetPolicyID = defaultPolicy.ID
				}
			}
			if targetPolicyID > 0 {
				ctrl.db.Where("role_id = ?", guestRole.ID).Delete(&model.RolePolicy{})
				ctrl.db.Create(&model.RolePolicy{RoleID: guestRole.ID, PolicyID: targetPolicyID})
			}
		} else {
			updates := map[string]any{}
			if guestRole.AllowedFormats == "" {
				updates["allowed_formats"] = "jpg,jpeg,png,gif,heif,webp"
			}
			if guestRole.MaxSingleFileSizeMB == 0 {
				updates["max_single_file_size_mb"] = 5
			}
			if len(updates) > 0 {
				ctrl.db.Model(&guestRole).Updates(updates)
			}
		}

		var guestUser model.User
		err = ctrl.db.Where("username = ?", "guest").First(&guestUser).Error
		if err != nil {
			guestUser = model.User{
				Username:     "guest",
				Nickname:     "游客",
				RoleID:       guestRole.ID,
				PasswordHash: "$2a$10$LockedGuestAccountNoPasswordAllowed0000000000000000",
				Status:       "active",
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			if err := ctrl.db.Create(&guestUser).Error; err != nil {
				return err
			}
		}
	} else if (enableVal == "false" || enableVal == "0") && cleanGuestData {
		var guestUser model.User
		if err := ctrl.db.Where("username = ?", "guest").First(&guestUser).Error; err == nil {
			var userFiles []model.FileRecord
			ctrl.db.Where("user_id = ?", guestUser.ID).Find(&userFiles)
			for _, f := range userFiles {
				if ctrl.uploadService != nil {
					_ = ctrl.uploadService.DeleteFile(context.Background(), f.ID, guestUser.ID, true)
				} else {
					ctrl.db.Unscoped().Delete(&f)
				}
			}
			ctrl.db.Where("user_id = ?", guestUser.ID).Delete(&model.Session{})
			ctrl.db.Where("user_id = ?", guestUser.ID).Delete(&model.ApiToken{})
			ctrl.db.Unscoped().Where("id = ?", guestUser.ID).Delete(&model.User{})
		}

		var guestRole model.Role
		if err := ctrl.db.Where("name = ?", "guest").First(&guestRole).Error; err == nil {
			ctrl.db.Where("role_id = ?", guestRole.ID).Delete(&model.RolePolicy{})
			ctrl.db.Unscoped().Where("id = ?", guestRole.ID).Delete(&model.Role{})
		}
	}
	return nil
}
