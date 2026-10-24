package http

import (
	"net/http"
	"strconv"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingsController struct {
	db           *gorm.DB
	derivedCache *cache.DerivedCache
}

func NewSettingsController(db *gorm.DB, dc *cache.DerivedCache) *SettingsController {
	return &SettingsController{
		db:           db,
		derivedCache: dc,
	}
}

func (ctrl *SettingsController) RegisterRoutes(rg *gin.RouterGroup) {
	// 1. User preferences & profile settings
	me := rg.Group("/me")
	me.Use(auth.RequireAuth())
	{
		me.GET("/settings", ctrl.GetUserSettings)
		me.POST("/settings", ctrl.SaveUserSettings)
		me.PATCH("/profile", ctrl.UpdateProfile)
		me.POST("/password", ctrl.ChangePassword)
	}

	// 2. Admin system settings
	admin := rg.Group("/admin")
	admin.Use(auth.RequireAuth(), auth.RequireAdmin())
	{
		admin.GET("/settings", ctrl.GetSystemSettings)
		admin.POST("/settings", ctrl.SaveSystemSettings)
		admin.POST("/cache/clear", ctrl.ClearCache)
	}
}

func (ctrl *SettingsController) GetUserSettings(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)

	// Settings stored in model.Setting with prefix "user:{id}:"
	prefix := "user:" + string(rune(userID)) + ":"
	_ = prefix

	var settings []model.Setting
	ctrl.db.Where("key LIKE ?", "user:"+c.GetString(auth.CtxUserIDKey)+":%").Find(&settings)

	// Fetch user storage policies for selection
	var policies []model.StoragePolicy
	ctrl.db.Where("user_id = ?", userID).Find(&policies)

	// Fetch user watermarks
	var watermarks []model.WatermarkTemplate
	ctrl.db.Where("owner_id = ? OR owner_id IS NULL", userID).Find(&watermarks)

	resp := gin.H{
		"watermark_mode":        ctrl.getSettingVal("user_wm_mode_"+strconvUint(userID), "inherit"),
		"watermark_template_id": ctrl.getSettingVal("user_wm_tmpl_"+strconvUint(userID), ""),
		"default_link_format":   ctrl.getSettingVal("user_link_fmt_"+strconvUint(userID), "markdown"),
		"default_policy_id":     ctrl.getSettingVal("user_default_policy_"+strconvUint(userID), "1"),
		"available_policies":    policies,
		"available_watermarks":  watermarks,
	}

	c.JSON(http.StatusOK, resp)
}

func (ctrl *SettingsController) SaveUserSettings(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)

	var req struct {
		WatermarkMode       string `json:"watermark_mode"`
		WatermarkTemplateID string `json:"watermark_template_id"`
		DefaultLinkFormat   string `json:"default_link_format"`
		DefaultPolicyID     string `json:"default_policy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.WatermarkMode != "" {
		ctrl.setSettingVal("user_wm_mode_"+strconvUint(userID), req.WatermarkMode)
	}
	if req.WatermarkTemplateID != "" {
		ctrl.setSettingVal("user_wm_tmpl_"+strconvUint(userID), req.WatermarkTemplateID)
	}
	if req.DefaultLinkFormat != "" {
		ctrl.setSettingVal("user_link_fmt_"+strconvUint(userID), req.DefaultLinkFormat)
	}
	if req.DefaultPolicyID != "" {
		ctrl.setSettingVal("user_default_policy_"+strconvUint(userID), req.DefaultPolicyID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "User settings updated"})
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
		updates["email"] = req.Email
	}

	if len(updates) > 0 {
		ctrl.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates)
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

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

func (ctrl *SettingsController) GetSystemSettings(c *gin.Context) {
	var settings []model.Setting
	ctrl.db.Find(&settings)

	res := map[string]string{
		"site_name":              "Utopia Images",
		"site_description":       "Next-generation self-hosted image & static asset host",
		"base_url":               "http://localhost:8080",
		"max_single_file_size":   "50", // MB
		"max_process_image_size": "5",  // MB
		"captcha_provider":       "altcha",
		"session_timeout_days":   "7",
		"require_2fa":            "false",
	}

	for _, s := range settings {
		res[s.Key] = s.Value
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *SettingsController) SaveSystemSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for k, v := range req {
		ctrl.setSettingVal(k, v)
	}

	c.JSON(http.StatusOK, gin.H{"message": "System settings saved successfully"})
}

func (ctrl *SettingsController) ClearCache(c *gin.Context) {
	count := ctrl.derivedCache.Clear()
	c.JSON(http.StatusOK, gin.H{
		"message": "Derived image cache cleared",
		"cleared": count,
	})
}

func (ctrl *SettingsController) getSettingVal(key, def string) string {
	var s model.Setting
	if err := ctrl.db.Where("`key` = ?", key).First(&s).Error; err != nil {
		return def
	}
	return s.Value
}

func (ctrl *SettingsController) setSettingVal(key, val string) {
	s := model.Setting{
		Key:   key,
		Value: val,
	}
	ctrl.db.Save(&s)
}

func strconvUint(u uint) string {
	return strconv.FormatUint(uint64(u), 10)
}
