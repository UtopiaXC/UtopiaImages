package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthController struct {
	authService *service.AuthService
}

func NewAuthController(as *service.AuthService) *AuthController {
	return &AuthController{authService: as}
}

func (ctrl *AuthController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/setup/status", ctrl.GetSetupStatus)
	rg.POST("/setup", ctrl.SetupInitialAdmin)

	rg.GET("/auth/captcha", ctrl.GetCaptchaChallenge)
	rg.POST("/auth/login", ctrl.Login)
	rg.POST("/auth/mfa", ctrl.VerifyMFA)
	rg.POST("/auth/logout", ctrl.Logout)

	me := rg.Group("/me")
	me.Use(auth.RequireAuth())
	{
		me.GET("", ctrl.GetCurrentUser)
		me.PATCH("/username", ctrl.ChangeUsername)
		me.GET("/tokens", ctrl.ListApiTokens)
		me.POST("/tokens", ctrl.CreateApiToken)
		me.DELETE("/tokens/:id", ctrl.DeleteApiToken)
		me.POST("/totp/setup", ctrl.SetupTOTP)
		me.POST("/totp/enable", ctrl.EnableTOTP)
		me.POST("/totp/disable", ctrl.DisableTOTP)
	}
}

func (ctrl *AuthController) GetSetupStatus(c *gin.Context) {
	required := ctrl.authService.IsSetupRequired()
	c.JSON(http.StatusOK, gin.H{
		"setup_required": required,
	})
}

func (ctrl *AuthController) SetupInitialAdmin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	admin, err := ctrl.authService.SetupInitialAdmin(req.Username, req.Password, req.Nickname, req.Email)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Administrator account created successfully",
		"user":    admin,
	})
}

func (ctrl *AuthController) GetCaptchaChallenge(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()

	result, err := ctrl.authService.Login(req.Username, req.Password, clientIP, userAgent, req.Captcha, req.RememberMe)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":            err.Error(),
			"requires_captcha": result != nil && result.RequiresCaptcha,
		})
		return
	}

	if result.RequiresMFA {
		c.JSON(http.StatusOK, gin.H{
			"requires_mfa": true,
			"user_id":      result.User.ID,
		})
		return
	}

	// Set session cookie
	maxAge := 7 * 86400
	if req.RememberMe {
		maxAge = 30 * 86400
	}
	c.SetCookie(auth.SessionCookieName, result.SessionToken, maxAge, "/api", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"token": result.SessionToken,
		"user":  result.User,
	})
}

func (ctrl *AuthController) VerifyMFA(c *gin.Context) {
	var req struct {
		UserID     uint   `json:"user_id" binding:"required"`
		Code       string `json:"code" binding:"required"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := ctrl.authService.VerifyMFAAndLogin(req.UserID, req.Code, c.ClientIP(), c.Request.UserAgent(), req.RememberMe)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	maxAge := 7 * 86400
	if req.RememberMe {
		maxAge = 30 * 86400
	}
	c.SetCookie(auth.SessionCookieName, token, maxAge, "/api", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
	})
}

func (ctrl *AuthController) Logout(c *gin.Context) {
	if cookie, err := c.Cookie(auth.SessionCookieName); err == nil {
		ctrl.authService.SessionManager().RevokeSession(cookie)
	}
	c.SetCookie(auth.SessionCookieName, "", -1, "/api", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (ctrl *AuthController) GetCurrentUser(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	c.JSON(http.StatusOK, user)
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
		"qr_base64": qrBase64,
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
	ctrl.authService.DB().Model(&model.User{}).Where("id = ?", userID).Update("require_2fa", false)
	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication disabled"})
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

func ParseUintSafe(s string, def uint) uint {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return def
	}
	return uint(v)
}

