package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/common"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/mailer"
	"github.com/UtopiaXC/UtopiaImages/internal/repository"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"gorm.io/gorm"
)

type AuthController struct {
	authService *service.AuthService
	db          *gorm.DB
	cfg         *config.Config
	configPath  string
	onDBChange  func(*gorm.DB)
	setupMu     sync.Mutex
}

func NewAuthController(as *service.AuthService, db *gorm.DB, cfg *config.Config, configPath string) *AuthController {
	return &AuthController{
		authService: as,
		db:          db,
		cfg:         cfg,
		configPath:  configPath,
	}
}

func (ctrl *AuthController) SetOnDBChange(fn func(*gorm.DB)) {
	ctrl.onDBChange = fn
}

func (ctrl *AuthController) isCookieSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return true
	}
	if ctrl.cfg != nil && strings.HasPrefix(strings.ToLower(ctrl.cfg.Server.BaseURL), "https://") {
		return true
	}
	return false
}

func (ctrl *AuthController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/setup/status", ctrl.GetSetupStatus)
	rg.POST("/setup/test-db", ctrl.TestDatabaseConnection)
	rg.POST("/setup/test-smtp", ctrl.TestSMTPConnection)
	rg.POST("/setup", ctrl.SetupInitialAdmin)

	rg.GET("/auth/captcha", ctrl.GetCaptchaChallenge)
	rg.POST("/auth/login", ctrl.Login)
	rg.POST("/auth/register", ctrl.Register)
	rg.POST("/auth/activate", ctrl.ActivateAccount)
	rg.POST("/auth/forgot-password", ctrl.ForgotPassword)
	rg.POST("/auth/reset-password", ctrl.ResetPassword)
	rg.POST("/auth/forgot-2fa", ctrl.Forgot2FA)
	rg.POST("/auth/reset-2fa", ctrl.Reset2FA)
	rg.POST("/auth/mfa", ctrl.VerifyMFA)
	rg.POST("/auth/logout", ctrl.Logout)
	rg.POST("/auth/passkey/login-begin", ctrl.BeginPasskeyLogin)
	rg.POST("/auth/passkey/login-finish", ctrl.FinishPasskeyLogin)
	rg.POST("/auth/mfa/passkey-begin", ctrl.BeginMFAPasskey)
	rg.POST("/auth/mfa/passkey-finish", ctrl.FinishMFAPasskey)
	rg.POST("/auth/setup-2fa/totp-secret", ctrl.Setup2FATOTPSecret)
	rg.POST("/auth/setup-2fa/totp-verify", ctrl.Setup2FATOTPVerify)
	rg.POST("/auth/setup-2fa/passkey-begin", ctrl.Setup2FAPasskeyBegin)
	rg.POST("/auth/setup-2fa/passkey-finish", ctrl.Setup2FAPasskeyFinish)

	me := rg.Group("/me")
	me.Use(auth.RequireAuth())
	{
		me.GET("", ctrl.GetCurrentUser)
		me.PATCH("/username", auth.RequireSessionAuth(), ctrl.ChangeUsername)
		me.GET("/tokens", auth.RequireSessionAuth(), ctrl.ListApiTokens)
		me.POST("/tokens", auth.RequireSessionAuth(), ctrl.CreateApiToken)
		me.DELETE("/tokens/:id", auth.RequireSessionAuth(), ctrl.DeleteApiToken)
		me.POST("/totp/setup", auth.RequireSessionAuth(), ctrl.SetupTOTP)
		me.POST("/totp/enable", auth.RequireSessionAuth(), ctrl.EnableTOTP)
		me.POST("/totp/disable", auth.RequireSessionAuth(), ctrl.DisableTOTP)

		// Passkey endpoints
		me.GET("/passkeys", auth.RequireSessionAuth(), ctrl.ListPasskeys)
		me.POST("/passkeys/register-begin", auth.RequireSessionAuth(), ctrl.BeginPasskeyRegister)
		me.POST("/passkeys/register-finish", auth.RequireSessionAuth(), ctrl.FinishPasskeyRegister)
		me.DELETE("/passkeys/:id", auth.RequireSessionAuth(), ctrl.DeletePasskey)
	}
}

func (ctrl *AuthController) GetSetupStatus(c *gin.Context) {
	required := ctrl.authService.IsSetupRequired()
	detectedProto := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		detectedProto = "https"
	}
	detectedBaseURL := fmt.Sprintf("%s://%s", detectedProto, getRequestHost(c))

	c.JSON(http.StatusOK, gin.H{
		"setup_required":   required,
		"site_name":        ctrl.cfg.Site.Name,
		"site_description": ctrl.cfg.Site.Description,
		"base_url":         ctrl.cfg.Server.BaseURL,
		"detected_url":     detectedBaseURL,
		"current_locale":   ctrl.cfg.Site.Locale,
		"current_driver":   ctrl.cfg.Database.Driver,
	})
}

func (ctrl *AuthController) TestDatabaseConnection(c *gin.Context) {
	if !ctrl.authService.IsSetupRequired() {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "系统已完成初始化，禁止未授权访问"})
		return
	}

	var req struct {
		Driver   string `json:"driver" binding:"required"`
		DSN      string `json:"dsn"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		DBName   string `json:"db_name"`
		User     string `json:"user"`
		Password string `json:"password"`
		SSLMode  string `json:"sslmode"`
		Charset  string `json:"charset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": common.SanitizeError(err)})
		return
	}

	dsn := req.DSN
	driver := strings.ToLower(req.Driver)
	switch driver {
	case "sqlite", "sqlite3":
		if dsn == "" {
			dsn = "data/db/utopia_images.db"
		}
	case "postgres", "postgresql":
		if dsn == "" {
			if req.Port == 0 {
				req.Port = 5432
			}
			if req.SSLMode == "" {
				req.SSLMode = "disable"
			}
			dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
				req.Host, req.Port, req.User, req.Password, req.DBName, req.SSLMode)
		}
	case "mysql":
		if dsn == "" {
			if req.Port == 0 {
				req.Port = 3306
			}
			if req.Charset == "" {
				req.Charset = "utf8mb4"
			}
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
				req.User, req.Password, req.Host, req.Port, req.DBName, req.Charset)
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的数据库驱动: " + req.Driver})
		return
	}

	targetDB, err := repository.ConnectTargetDB(driver, dsn)
	if err != nil {
		slog.Warn("Test database connection failed", "driver", driver, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": common.SanitizeError(err)})
		return
	}

	sqlDB, err := targetDB.DB()
	if err != nil || sqlDB.Ping() != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无法连接到数据库服务器，请检查主机名和端口"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "数据库连接测试成功",
		"dsn":     dsn,
	})
}

func (ctrl *AuthController) TestSMTPConnection(c *gin.Context) {
	if !ctrl.authService.IsSetupRequired() {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "系统已完成初始化，禁止未授权访问"})
		return
	}
	var req struct {
		Host       string `json:"host" binding:"required"`
		Port       int    `json:"port" binding:"required"`
		From       string `json:"from"`
		User       string `json:"user"`
		Password   string `json:"password"`
		Encryption string `json:"encryption"`
		TestTo     string `json:"test_to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写完整的 SMTP 主机和端口"})
		return
	}

	testCfg := config.SMTPConfig{
		Enabled:    true,
		Host:       req.Host,
		Port:       req.Port,
		From:       req.From,
		User:       req.User,
		Password:   req.Password,
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

func (ctrl *AuthController) SetupInitialAdmin(c *gin.Context) {
	ctrl.setupMu.Lock()
	defer ctrl.setupMu.Unlock()

	if !ctrl.authService.IsSetupRequired() {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "系统已完成初始化，无法重复执行"})
		return
	}

	var req struct {
		Site struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			BaseURL     string `json:"base_url"`
			Locale      string `json:"locale"`
		} `json:"site"`
		Database struct {
			Driver string `json:"driver"`
			DSN    string `json:"dsn"`
		} `json:"database"`
		SMTP struct {
			Enabled    bool   `json:"enabled"`
			Host       string `json:"host"`
			Port       int    `json:"port"`
			From       string `json:"from"`
			FromName   string `json:"from_name"`
			User       string `json:"user"`
			Password   string `json:"password"`
			Encryption string `json:"encryption"`
		} `json:"smtp"`
		Admin struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Nickname string `json:"nickname"`
			Email    string `json:"email"`
		} `json:"admin"`
		// Fallbacks for flat payloads
		Username string `json:"username"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "输入数据格式不正确"})
		return
	}

	adminUsername := req.Admin.Username
	if adminUsername == "" {
		adminUsername = req.Username
	}
	adminPassword := req.Admin.Password
	if adminPassword == "" {
		adminPassword = req.Password
	}
	adminNickname := req.Admin.Nickname
	if adminNickname == "" {
		adminNickname = req.Nickname
	}
	adminEmail := req.Admin.Email
	if adminEmail == "" {
		adminEmail = req.Email
	}

	if adminUsername == "" || adminPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "管理员用户名和密码不能为空"})
		return
	}
	if len(adminPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "管理员密码长度至少需为 8 位"})
		return
	}
	if strings.TrimSpace(adminEmail) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "管理员邮箱为必填项，请输入有效邮箱地址"})
		return
	}

	// 1. If database driver or DSN changed during setup wizard, initialize target DB
	if req.Database.Driver != "" && req.Database.DSN != "" {
		driver := strings.ToLower(req.Database.Driver)
		if driver != strings.ToLower(ctrl.cfg.Database.Driver) || req.Database.DSN != ctrl.cfg.Database.DSN {
			targetDB, err := repository.ConnectTargetDB(driver, req.Database.DSN)
			if err != nil {
				slog.Error("Failed to connect target DB during setup", "error", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "连接目标数据库失败: " + common.SanitizeError(err)})
				return
			}

			// Migrate models and seed defaults on new DB
			if err := targetDB.AutoMigrate(
				&model.Role{},
				&model.StoragePolicy{},
				&model.RolePolicy{},
				&model.User{},
				&model.FileRecord{},
				&model.TemporaryLink{},
				&model.WatermarkTemplate{},
				&model.ImagePreset{},
				&model.Session{},
				&model.ApiToken{},
				&model.TOTPSecret{},
				&model.WebAuthnCredential{},
				&model.RecoveryCode{},
				&model.Setting{},
				&model.PasswordResetToken{},
			); err != nil {
				slog.Error("Failed to migrate target DB during setup", "error", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "数据库迁移初始化失败: " + common.SanitizeError(err)})
				return
			}

			if err := repository.SeedDefaults(targetDB); err != nil {
				slog.Error("Failed to seed defaults on target DB", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库默认数据初始化失败: " + common.SanitizeError(err)})
				return
			}

			ctrl.db = targetDB
			ctrl.authService.SetDB(targetDB)
			ctrl.cfg.Database.Driver = driver
			ctrl.cfg.Database.DSN = req.Database.DSN
			if ctrl.onDBChange != nil {
				ctrl.onDBChange(targetDB)
			}
		}
	}

	// 2. Create the initial administrator
	admin, err := ctrl.authService.SetupInitialAdmin(adminUsername, adminPassword, adminNickname, adminEmail)
	if err != nil {
		slog.Error("Setup initial admin failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": common.SanitizeError(err)})
		return
	}

	// 3. Save Site configuration
	if req.Site.Name != "" {
		ctrl.cfg.Site.Name = req.Site.Name
		ctrl.db.Save(&model.Setting{Key: "site_name", Value: req.Site.Name})
	}
	if req.Site.Description != "" {
		ctrl.cfg.Site.Description = req.Site.Description
		ctrl.db.Save(&model.Setting{Key: "site_description", Value: req.Site.Description})
	}
	if req.Site.BaseURL != "" {
		ctrl.cfg.Site.BaseURL = strings.TrimRight(req.Site.BaseURL, "/")
		ctrl.cfg.Server.BaseURL = ctrl.cfg.Site.BaseURL
		ctrl.db.Save(&model.Setting{Key: "base_url", Value: ctrl.cfg.Site.BaseURL})
		if ctrl.authService != nil && ctrl.authService.PasskeyManager() != nil {
			_ = ctrl.authService.PasskeyManager().UpdateBaseURL(ctrl.cfg.Site.BaseURL)
		}
	}
	if req.Site.Locale != "" {
		ctrl.cfg.Site.Locale = req.Site.Locale
		ctrl.db.Save(&model.Setting{Key: "site_locale", Value: req.Site.Locale})
	}

	// 4. Save SMTP configuration
	ctrl.cfg.SMTP.Enabled = req.SMTP.Enabled
	ctrl.cfg.SMTP.Host = req.SMTP.Host
	ctrl.cfg.SMTP.Port = req.SMTP.Port
	ctrl.cfg.SMTP.From = req.SMTP.From
	ctrl.cfg.SMTP.FromName = req.SMTP.FromName
	ctrl.cfg.SMTP.User = req.SMTP.User
	ctrl.cfg.SMTP.Password = req.SMTP.Password
	ctrl.cfg.SMTP.Encryption = req.SMTP.Encryption

	ctrl.db.Save(&model.Setting{Key: "smtp_enabled", Value: strconv.FormatBool(req.SMTP.Enabled)})
	ctrl.db.Save(&model.Setting{Key: "smtp_host", Value: req.SMTP.Host})
	ctrl.db.Save(&model.Setting{Key: "smtp_port", Value: strconv.Itoa(req.SMTP.Port)})
	ctrl.db.Save(&model.Setting{Key: "smtp_from", Value: req.SMTP.From})
	ctrl.db.Save(&model.Setting{Key: "smtp_from_name", Value: req.SMTP.FromName})
	ctrl.db.Save(&model.Setting{Key: "smtp_user", Value: req.SMTP.User})
	ctrl.db.Save(&model.Setting{Key: "smtp_encryption", Value: req.SMTP.Encryption})

	// 5. Persist to yaml config file
	if ctrl.configPath != "" {
		_ = config.SaveConfig(ctrl.configPath, ctrl.cfg)
	}

	slog.Info("System successfully initialized", "admin", admin.Username, "email", admin.Email)
	c.JSON(http.StatusOK, gin.H{
		"message": "系统初始化完成",
		"user":    admin,
	})
}

func (ctrl *AuthController) GetCaptchaChallenge(c *gin.Context) {
	provider := ctrl.authService.GetCaptchaProvider()
	if provider != "altcha" {
		c.JSON(http.StatusOK, gin.H{
			"provider": provider,
		})
		return
	}

	challenge, err := ctrl.authService.GenerateCaptchaChallenge()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate captcha"})
		return
	}
	c.JSON(http.StatusOK, challenge)
}

func (ctrl *AuthController) Login(c *gin.Context) {
	var req struct {
		Username   string `json:"username" binding:"required"`
		Password   string `json:"password" binding:"required"`
		Captcha    string `json:"captcha"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()

	result, errCode := ctrl.authService.Login(req.Username, req.Password, clientIP, userAgent, req.Captcha, req.RememberMe)
	if errCode != "" {
		reqCaptcha := false
		if result != nil && result.RequiresCaptcha {
			reqCaptcha = true
		} else if ctrl.authService.Tracker().RequiresCaptcha(fmt.Sprintf("%s:%s", clientIP, req.Username)) {
			reqCaptcha = true
		}
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":             errCode,
			"error":            errCode,
			"requires_captcha": reqCaptcha,
		})
		return
	}

	if result.RequiresMFA {
		c.JSON(http.StatusOK, gin.H{
			"requires_mfa": true,
			"has_totp":     result.HasTOTP,
			"has_passkey":  result.HasPasskey,
			"user_id":      result.User.ID,
			"username":     result.User.Username,
			"mfa_token":    result.MFAToken,
		})
		return
	}

	if result.Requires2FASetup {
		c.JSON(http.StatusOK, gin.H{
			"requires_2fa_setup": true,
			"setup_token":        result.SetupToken,
			"user_id":            result.User.ID,
			"username":           result.User.Username,
		})
		return
	}

	// Set session cookie
	maxAge := 7 * 86400
	if req.RememberMe {
		maxAge = 30 * 86400
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, result.SessionToken, maxAge, "/api", "", ctrl.isCookieSecure(c), true)

	c.JSON(http.StatusOK, gin.H{
		"token": result.SessionToken,
		"user":  result.User,
	})
}

func (ctrl *AuthController) Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
		Nickname string `json:"nickname"`
		Captcha  string `json:"captcha"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	if !ctrl.authService.VerifyCaptcha(req.Captcha, c.ClientIP()) {
		common.RespondError(c, http.StatusBadRequest, common.ErrAuthCaptchaRequired)
		return
	}

	// Requirement 5: System MUST have SMTP configured to enable registration
	if !ctrl.cfg.SMTP.Enabled || ctrl.cfg.SMTP.Host == "" {
		common.RespondError(c, http.StatusBadRequest, common.ErrRegistrationSMTPRequired)
		return
	}

	user, token, errCode := ctrl.authService.Register(req.Username, req.Email, req.Password, req.Nickname)
	if errCode != "" {
		common.RespondError(c, http.StatusBadRequest, errCode)
		return
	}

	// Send email with activation link
	activationLink := fmt.Sprintf("%s/login?action=activate&token=%s", ctrl.cfg.Server.BaseURL, token)
	if err := ctrl.sendActivationEmail(*user.Email, user.Username, activationLink); err != nil {
		slog.Error("Failed to send activation email", "error", err, "email", *user.Email)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "ACTIVATION_EMAIL_SENT",
		"message": "Activation email sent",
		"user":    user,
	})
}

func (ctrl *AuthController) ActivateAccount(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	errCode := ctrl.authService.ActivateAccount(req.Token)
	if errCode != "" {
		common.RespondError(c, http.StatusBadRequest, errCode)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "SUCCESS",
		"message": "Account activated successfully",
	})
}

func (ctrl *AuthController) ForgotPassword(c *gin.Context) {
	var req struct {
		Email   string `json:"email"`
		Account string `json:"account"`
		Captcha string `json:"captcha"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	if !ctrl.authService.VerifyCaptcha(req.Captcha, c.ClientIP()) {
		common.RespondError(c, http.StatusBadRequest, common.ErrAuthCaptchaRequired)
		return
	}

	ident := strings.TrimSpace(req.Email)
	if ident == "" {
		ident = strings.TrimSpace(req.Account)
	}
	if ident == "" {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	if !ctrl.cfg.SMTP.Enabled || ctrl.cfg.SMTP.Host == "" {
		common.RespondError(c, http.StatusBadRequest, common.ErrSMTPNotConfigured)
		return
	}

	user, token, errCode := ctrl.authService.RequestPasswordReset(ident)
	if errCode != "" {
		if errCode == common.ErrAuthUserNotFound {
			// Prevent username/email enumeration
			c.JSON(http.StatusOK, gin.H{
				"code":    "RESET_EMAIL_SENT",
				"message": "Password reset email sent",
			})
			return
		}
		common.RespondError(c, http.StatusBadRequest, errCode)
		return
	}

	// Send email with reset token link
	resetLink := fmt.Sprintf("%s/login?action=reset&token=%s", ctrl.cfg.Server.BaseURL, token)
	err := ctrl.sendPasswordResetEmail(*user.Email, user.Username, resetLink)
	if err != nil {
		slog.Error("Failed to send password reset email", "error", err, "email", *user.Email)
		common.RespondError(c, http.StatusInternalServerError, common.ErrInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "RESET_EMAIL_SENT",
		"message": "Password reset email sent",
	})
}

func (ctrl *AuthController) ResetPassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	errCode := ctrl.authService.ResetPasswordWithToken(req.Token, req.NewPassword)
	if errCode != "" {
		common.RespondError(c, http.StatusBadRequest, errCode)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "PASSWORD_RESET_SUCCESS",
		"message": "Password reset successfully",
	})
}

func (ctrl *AuthController) Forgot2FA(c *gin.Context) {
	var req struct {
		MFAToken string `json:"mfa_token"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Account  string `json:"account"`
		Captcha  string `json:"captcha"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	if !ctrl.cfg.SMTP.Enabled || ctrl.cfg.SMTP.Host == "" {
		common.RespondError(c, http.StatusBadRequest, common.ErrSMTPNotConfigured)
		return
	}

	var (
		user    *model.User
		token   string
		errCode string
	)

	if req.MFAToken != "" {
		// Validated via password authentication: resolve user securely from mfa token
		userID, ok := ctrl.authService.ValidateMFAToken(req.MFAToken)
		if !ok {
			common.RespondError(c, http.StatusUnauthorized, common.ErrAuthUnauthorized)
			return
		}
		user, token, errCode = ctrl.authService.Request2FAResetByID(userID)
	} else {
		// Unauthenticated reset: verify captcha and resolve by email or account
		if !ctrl.authService.VerifyCaptcha(req.Captcha, c.ClientIP()) {
			common.RespondError(c, http.StatusBadRequest, common.ErrAuthCaptchaRequired)
			return
		}

		ident := strings.TrimSpace(req.Email)
		if ident == "" {
			ident = strings.TrimSpace(req.Username)
		}
		if ident == "" {
			ident = strings.TrimSpace(req.Account)
		}
		if ident == "" {
			common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
			return
		}
		user, token, errCode = ctrl.authService.Request2FAReset(ident)
	}

	if errCode != "" {
		if errCode == common.ErrAuthUserNotFound {
			// Prevent user enumeration
			c.JSON(http.StatusOK, gin.H{
				"code":    "2FA_RESET_EMAIL_SENT",
				"message": "2FA reset email sent",
			})
			return
		}
		common.RespondError(c, http.StatusBadRequest, errCode)
		return
	}

	resetLink := fmt.Sprintf("%s/login?action=reset-2fa&token=%s", ctrl.cfg.Server.BaseURL, token)
	err := ctrl.send2FAResetEmail(*user.Email, user.Username, resetLink, token)
	if err != nil {
		slog.Error("Failed to send 2FA reset email", "error", err, "email", *user.Email)
		common.RespondError(c, http.StatusInternalServerError, common.ErrInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "2FA_RESET_EMAIL_SENT",
		"message": "2FA reset email sent",
	})
}

func (ctrl *AuthController) Reset2FA(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}

	errCode := ctrl.authService.Reset2FAWithToken(req.Token)
	if errCode != "" {
		common.RespondError(c, http.StatusBadRequest, errCode)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "2FA_RESET_SUCCESS",
		"message": "Two-factor authentication reset successfully",
	})
}

func (ctrl *AuthController) send2FAResetEmail(toEmail, username, resetLink, token string) error {
	subject := "[Utopia Images] 两步验证重置请求"
	body := fmt.Sprintf("您好 %s：\r\n\r\n我们收到了您的两步验证 (2FA) 重置请求。请在 15 分钟内点击以下链接重置 2FA，或在页面输入重置令牌：\r\n重置链接：%s\r\n重置令牌：%s\r\n\r\n重置后两步验证将被解除，您可以使用账号密码直接登录系统。\r\n若您未发起此重置请求，请忽略本邮件。\r\n", username, resetLink, token)
	return mailer.SendMail(ctrl.cfg.SMTP, toEmail, subject, body)
}

func (ctrl *AuthController) sendActivationEmail(toEmail, username, activationLink string) error {
	subject := "[Utopia Images] 账号激活确认"
	body := fmt.Sprintf("您好 %s：\r\n\r\n欢迎注册 Utopia Images！\r\n您的账号已成功创建，目前处于冻结状态。请在 24 小时内点击下方链接完成邮箱激活：\r\n%s\r\n\r\n激活完成后，您即可正常登录并使用 Utopia Images 平台。若您未注册过此账号，请忽略此邮件。\r\n", username, activationLink)
	return mailer.SendMail(ctrl.cfg.SMTP, toEmail, subject, body)
}

func (ctrl *AuthController) sendPasswordResetEmail(toEmail, username, resetLink string) error {
	subject := "[Utopia Images] 密码重置请求"
	body := fmt.Sprintf("您好 %s：\r\n\r\n我们收到了您的密码重置请求。请在 15 分钟内点击以下链接重置密码：\r\n%s\r\n\r\n若您未发起此重置请求，请忽略本邮件，原账号密码依然安全有效。\r\n", username, resetLink)
	return mailer.SendMail(ctrl.cfg.SMTP, toEmail, subject, body)
}

func (ctrl *AuthController) VerifyMFA(c *gin.Context) {
	var req struct {
		UserID     uint   `json:"user_id"`
		MFAToken   string `json:"mfa_token"`
		Code       string `json:"code" binding:"required"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := ctrl.authService.VerifyMFAAndLogin(req.MFAToken, req.Code, c.ClientIP(), c.Request.UserAgent(), req.RememberMe)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	maxAge := 7 * 86400
	if req.RememberMe {
		maxAge = 30 * 86400
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, token, maxAge, "/api", "", ctrl.isCookieSecure(c), true)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
	})
}

func (ctrl *AuthController) Logout(c *gin.Context) {
	if cookie, err := c.Cookie(auth.SessionCookieName); err == nil {
		ctrl.authService.SessionManager().RevokeSession(cookie)
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, "", -1, "/api", "", ctrl.isCookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (ctrl *AuthController) GetCurrentUser(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var totpCount int64
	ctrl.db.Model(&model.TOTPSecret{}).Where("user_id = ? AND confirmed_at IS NOT NULL", user.ID).Count(&totpCount)

	var passkeyCount int64
	ctrl.db.Model(&model.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&passkeyCount)

	var sysReq2FA model.Setting
	ctrl.db.Where("key = ?", "require_2fa").First(&sysReq2FA)
	isGlobal2FA := sysReq2FA.Value == "true"

	c.JSON(http.StatusOK, gin.H{
		"id":                      user.ID,
		"username":                user.Username,
		"nickname":                user.Nickname,
		"email":                   user.Email,
		"avatar":                  user.Avatar,
		"role_id":                 user.RoleID,
		"role":                    user.Role,
		"status":                  user.Status,
		"used_bytes":              user.UsedBytes,
		"file_count":              user.FileCount,
		"token_version":           user.TokenVersion,
		"locale":                  user.Locale,
		"require_2fa":             user.Require2FA,
		"global_require_2fa":      isGlobal2FA,
		"totp_enabled":            totpCount > 0,
		"has_totp":                totpCount > 0,
		"has_passkey":             passkeyCount > 0,
		"passkey_count":           passkeyCount,
		"hotlink_enabled":         user.HotlinkEnabled,
		"hotlink_allow_empty":     user.HotlinkAllowEmpty,
		"hotlink_allowed_domains": user.HotlinkAllowedDomains,
		"created_at":              user.CreatedAt,
		"updated_at":              user.UpdatedAt,
	})
}

func (ctrl *AuthController) ListApiTokens(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	var tokens []model.ApiToken
	ctrl.authService.DB().Where("user_id = ?", userID).Find(&tokens)
	c.JSON(http.StatusOK, tokens)
}

func (ctrl *AuthController) CreateApiToken(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	var req struct {
		Name        string `json:"name" binding:"required"`
		Scopes      string `json:"scopes"`
		ExpiresDays int    `json:"expires_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rawToken, tokenRecord, err := ctrl.authService.CreateApiToken(userID, req.Name, req.Scopes, req.ExpiresDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":  rawToken, // Only shown once upon creation!
		"record": tokenRecord,
	})
}

func (ctrl *AuthController) DeleteApiToken(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	tokenID := c.Param("id")
	ctrl.authService.DB().Where("id = ? AND user_id = ?", tokenID, userID).Delete(&model.ApiToken{})
	c.JSON(http.StatusOK, gin.H{"message": "Token deleted"})
}

func (ctrl *AuthController) SetupTOTP(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	secret, qrBase64, err := ctrl.authService.TOTPManager().GenerateSecret(user.Username, "Utopia Images")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate TOTP secret"})
		return
	}

	encSecret, _ := ctrl.authService.TOTPManager().EncryptSecret(secret)
	totpSec := model.TOTPSecret{
		UserID:          user.ID,
		SecretEncrypted: encSecret,
	}
	ctrl.authService.DB().Save(&totpSec)

	c.JSON(http.StatusOK, gin.H{
		"secret":    secret,
		"qr_base64": "data:image/png;base64," + qrBase64,
	})
}

func (ctrl *AuthController) EnableTOTP(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var sec model.TOTPSecret
	if err := ctrl.authService.DB().Where("user_id = ?", user.ID).First(&sec).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "TOTP setup not initialized"})
		return
	}

	plainSecret, err := ctrl.authService.TOTPManager().DecryptSecret(sec.SecretEncrypted)
	if err != nil || !ctrl.authService.TOTPManager().Validate(req.Code, plainSecret) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification code"})
		return
	}

	now := time.Now()
	sec.ConfirmedAt = &now
	ctrl.authService.DB().Save(&sec)
	ctrl.authService.DB().Model(&model.User{}).Where("id = ?", user.ID).Update("require_2fa", true)

	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication enabled successfully"})
}

func (ctrl *AuthController) DisableTOTP(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	ctrl.authService.DB().Where("user_id = ?", userID).Delete(&model.TOTPSecret{})

	var passkeyCount int64
	ctrl.authService.DB().Model(&model.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&passkeyCount)
	if passkeyCount == 0 {
		ctrl.authService.DB().Model(&model.User{}).Where("id = ?", userID).Update("require_2fa", false)
	}
	c.JSON(http.StatusOK, gin.H{"message": "TOTP authentication disabled"})
}

func (ctrl *AuthController) ChangeUsername(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	var req struct {
		NewUsername string `json:"new_username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.authService.ChangeUsername(userID, req.NewUsername); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Username updated successfully. Existing file links have been migrated to the new username.",
		"new_username": req.NewUsername,
	})
}

func getRequestHost(c *gin.Context) string {
	if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
		return xfh
	}
	return c.Request.Host
}

// Passkey Handlers

func (ctrl *AuthController) BeginPasskeyLogin(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported in current environment"})
		return
	}
	var req struct {
		Username string `json:"username"`
		UserID   uint   `json:"user_id"`
	}
	_ = c.ShouldBindJSON(&req)
	username := strings.TrimSpace(req.Username)
	if username == "" && req.UserID > 0 {
		var u model.User
		if err := ctrl.db.Where("id = ?", req.UserID).First(&u).Error; err == nil {
			username = u.Username
		}
	}
	options, sessionKey, err := ctrl.authService.PasskeyManager().BeginLogin(username, getRequestHost(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"options":     options,
		"session_key": sessionKey,
	})
}

func (ctrl *AuthController) FinishPasskeyLogin(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		sessionKey = c.GetHeader("X-Session-Key")
	}
	parsedData, err := protocol.ParseCredentialRequestResponseBody(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse passkey assertion: " + err.Error()})
		return
	}
	user, err := ctrl.authService.PasskeyManager().FinishLogin(sessionKey, parsedData)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Passkey verification failed: " + err.Error()})
		return
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is disabled or banned"})
		return
	}
	if user.LockoutUntil != nil && user.LockoutUntil.After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is temporarily locked"})
		return
	}
	rememberMe := c.Query("remember_me") == "true"
	rawToken, _, err := ctrl.authService.SessionManager().CreateSession(user.ID, c.ClientIP(), c.Request.UserAgent(), rememberMe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reset failed attempts and lockout on successful passwordless login
	ctrl.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"failed_login_attempts": 0,
		"lockout_until":         nil,
	})
	ctrl.authService.Tracker().RecordSuccess(fmt.Sprintf("%s:%s", c.ClientIP(), user.Username))

	maxAge := 7 * 86400
	if rememberMe {
		maxAge = 30 * 86400
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, rawToken, maxAge, "/api", "", ctrl.isCookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{
		"token": rawToken,
		"user":  user,
	})
}

// BeginMFAPasskey begins WebAuthn authentication for 2FA stage using an MFA token
func (ctrl *AuthController) BeginMFAPasskey(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported in current environment"})
		return
	}
	var req struct {
		MFAToken string `json:"mfa_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mfa_token is required"})
		return
	}
	userID, valid := ctrl.authService.ValidateMFAToken(req.MFAToken)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "MFA session expired or invalid, please login again"})
		return
	}
	var user model.User
	if err := ctrl.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is disabled or banned"})
		return
	}
	if user.LockoutUntil != nil && user.LockoutUntil.After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is temporarily locked"})
		return
	}
	options, sessionKey, err := ctrl.authService.PasskeyManager().BeginLogin(user.Username, getRequestHost(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"options":     options,
		"session_key": sessionKey,
	})
}

// FinishMFAPasskey finishes WebAuthn authentication for 2FA stage and consumes the MFA token
func (ctrl *AuthController) FinishMFAPasskey(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	mfaToken := c.Query("mfa_token")
	if mfaToken == "" {
		mfaToken = c.GetHeader("X-MFA-Token")
	}
	if mfaToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mfa_token is required"})
		return
	}
	expectedUserID, valid := ctrl.authService.ValidateMFAToken(mfaToken)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "MFA session expired or invalid, please login again"})
		return
	}

	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		sessionKey = c.GetHeader("X-Session-Key")
	}
	parsedData, err := protocol.ParseCredentialRequestResponseBody(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse passkey assertion: " + err.Error()})
		return
	}
	user, err := ctrl.authService.PasskeyManager().FinishLogin(sessionKey, parsedData)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Passkey verification failed: " + err.Error()})
		return
	}
	if user.ID != expectedUserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Passkey does not belong to the logging-in user"})
		return
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is disabled or banned"})
		return
	}
	if user.LockoutUntil != nil && user.LockoutUntil.After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is temporarily locked"})
		return
	}

	// Consume MFA token
	ctrl.authService.RevokeMFAToken(mfaToken)

	// Reset attempts and lockout
	ctrl.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"failed_login_attempts": 0,
		"lockout_until":         nil,
	})
	ctrl.authService.Tracker().RecordSuccess(fmt.Sprintf("%s:%s", c.ClientIP(), user.Username))

	rememberMe := c.Query("remember_me") == "true"
	rawToken, _, err := ctrl.authService.SessionManager().CreateSession(user.ID, c.ClientIP(), c.Request.UserAgent(), rememberMe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	maxAge := 7 * 86400
	if rememberMe {
		maxAge = 30 * 86400
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, rawToken, maxAge, "/api", "", ctrl.isCookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{
		"token": rawToken,
		"user":  user,
	})
}

func (ctrl *AuthController) ListPasskeys(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	userID := c.GetUint(auth.CtxUserIDKey)
	passkeys := ctrl.authService.PasskeyManager().ListUserPasskeys(userID)
	c.JSON(http.StatusOK, passkeys)
}

func (ctrl *AuthController) BeginPasskeyRegister(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	options, sessionKey, err := ctrl.authService.PasskeyManager().BeginRegistration(user, getRequestHost(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"options":     options,
		"session_key": sessionKey,
	})
}

func (ctrl *AuthController) FinishPasskeyRegister(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		sessionKey = c.GetHeader("X-Session-Key")
	}
	name := c.DefaultQuery("name", "Passkey")
	parsedData, err := protocol.ParseCredentialCreationResponseBody(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse passkey credential: " + err.Error()})
		return
	}
	cred, err := ctrl.authService.PasskeyManager().FinishRegistration(sessionKey, name, parsedData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passkey registration failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Passkey registered successfully",
		"passkey": cred,
	})
}

func (ctrl *AuthController) DeletePasskey(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	userID := c.GetUint(auth.CtxUserIDKey)
	id := ParseUintSafe(c.Param("id"), 0)
	if err := ctrl.authService.PasskeyManager().DeletePasskey(userID, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var passkeyCount int64
	ctrl.authService.DB().Model(&model.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&passkeyCount)
	var totpCount int64
	ctrl.authService.DB().Model(&model.TOTPSecret{}).Where("user_id = ? AND confirmed_at IS NOT NULL", userID).Count(&totpCount)
	if passkeyCount == 0 && totpCount == 0 {
		ctrl.authService.DB().Model(&model.User{}).Where("id = ?", userID).Update("require_2fa", false)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Passkey deleted"})
}

func (ctrl *AuthController) Setup2FATOTPSecret(c *gin.Context) {
	var req struct {
		SetupToken string `json:"setup_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing setup token"})
		return
	}
	userID, err := ctrl.authService.VerifySetupToken(req.SetupToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var user model.User
	if err := ctrl.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	secret, qrBase64, err := ctrl.authService.TOTPManager().GenerateSecret(user.Username, "Utopia Images")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate TOTP secret"})
		return
	}

	// Persist pending encrypted secret
	encSecret, err := ctrl.authService.TOTPManager().EncryptSecret(secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt TOTP secret"})
		return
	}
	totpRecord := model.TOTPSecret{
		UserID:          user.ID,
		SecretEncrypted: encSecret,
		CreatedAt:       time.Now(),
	}
	ctrl.db.Save(&totpRecord)

	c.JSON(http.StatusOK, gin.H{
		"secret":    secret,
		"qr_base64": "data:image/png;base64," + qrBase64,
	})
}

func (ctrl *AuthController) Setup2FATOTPVerify(c *gin.Context) {
	var req struct {
		SetupToken string `json:"setup_token" binding:"required"`
		Code       string `json:"code" binding:"required"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields"})
		return
	}
	userID, err := ctrl.authService.VerifySetupToken(req.SetupToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var user model.User
	if err := ctrl.db.Preload("Role").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var secret model.TOTPSecret
	if err := ctrl.db.Where("user_id = ?", user.ID).First(&secret).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No pending TOTP secret found"})
		return
	}
	plainSecret, err := ctrl.authService.TOTPManager().DecryptSecret(secret.SecretEncrypted)
	if err != nil || !ctrl.authService.TOTPManager().Validate(req.Code, plainSecret) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification code"})
		return
	}

	now := time.Now()
	ctrl.db.Model(&secret).Update("confirmed_at", &now)
	ctrl.db.Model(&user).Update("require_2fa", true)
	ctrl.authService.RevokeSetupToken(req.SetupToken)

	rawToken, _, err := ctrl.authService.SessionManager().CreateSession(user.ID, c.ClientIP(), c.Request.UserAgent(), req.RememberMe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	maxAge := 7 * 86400
	if req.RememberMe {
		maxAge = 30 * 86400
	}
	c.SetCookie(auth.SessionCookieName, rawToken, maxAge, "/api", "", ctrl.isCookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{
		"token": rawToken,
		"user":  user,
	})
}

func (ctrl *AuthController) Setup2FAPasskeyBegin(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	var req struct {
		SetupToken string `json:"setup_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing setup token"})
		return
	}
	userID, err := ctrl.authService.VerifySetupToken(req.SetupToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var user model.User
	if err := ctrl.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	options, sessionKey, err := ctrl.authService.PasskeyManager().BeginRegistration(&user, getRequestHost(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"options":     options,
		"session_key": sessionKey,
	})
}

func (ctrl *AuthController) Setup2FAPasskeyFinish(c *gin.Context) {
	if ctrl.authService.PasskeyManager() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Passkey is not supported"})
		return
	}
	setupToken := c.Query("setup_token")
	if setupToken == "" {
		setupToken = c.GetHeader("X-Setup-Token")
	}
	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		sessionKey = c.GetHeader("X-Session-Key")
	}
	userID, err := ctrl.authService.VerifySetupToken(setupToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	parsedData, err := protocol.ParseCredentialCreationResponseBody(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse credential: " + err.Error()})
		return
	}

	name := c.DefaultQuery("name", "Passkey")
	_, err = ctrl.authService.PasskeyManager().FinishRegistration(sessionKey, name, parsedData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passkey registration failed: " + err.Error()})
		return
	}

	ctrl.db.Model(&model.User{}).Where("id = ?", userID).Update("require_2fa", true)
	ctrl.authService.RevokeSetupToken(setupToken)

	var user model.User
	ctrl.db.Preload("Role").Where("id = ?", userID).First(&user)

	rememberMe := c.Query("remember_me") == "true"
	rawToken, _, err := ctrl.authService.SessionManager().CreateSession(user.ID, c.ClientIP(), c.Request.UserAgent(), rememberMe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	maxAge := 7 * 86400
	if rememberMe {
		maxAge = 30 * 86400
	}
	c.SetCookie(auth.SessionCookieName, rawToken, maxAge, "/api", "", ctrl.isCookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{
		"token": rawToken,
		"user":  user,
	})
}

func ParseUintSafe(s string, def uint) uint {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return def
	}
	return uint(v)
}
