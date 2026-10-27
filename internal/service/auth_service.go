package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/common"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
	"gorm.io/gorm"
)

type setupTokenEntry struct {
	UserID    uint
	ExpiresAt time.Time
}

type mfaTokenEntry struct {
	UserID    uint
	ExpiresAt time.Time
	Attempts  int
}

type AuthService struct {
	db             *gorm.DB
	sessionManager *auth.SessionManager
	totpManager    *auth.TOTPManager
	captchaManager *auth.CaptchaManager
	passkeyManager *auth.PasskeyManager
	tracker        *auth.LoginAttemptsTracker
	setupTokens    map[string]setupTokenEntry
	setupLock      sync.RWMutex
	mfaTokens      map[string]*mfaTokenEntry
	mfaLock        sync.Mutex
}

func NewAuthService(
	db *gorm.DB,
	sm *auth.SessionManager,
	tm *auth.TOTPManager,
	cm *auth.CaptchaManager,
	pm *auth.PasskeyManager,
) *AuthService {
	return &AuthService{
		db:             db,
		sessionManager: sm,
		totpManager:    tm,
		captchaManager: cm,
		passkeyManager: pm,
		tracker:        auth.NewLoginAttemptsTracker(),
		setupTokens:    make(map[string]setupTokenEntry),
		mfaTokens:      make(map[string]*mfaTokenEntry),
	}
}

// IsSetupRequired checks if no administrator account exists yet
func (s *AuthService) IsSetupRequired() bool {
	var count int64
	s.db.Model(&model.User{}).Where("role_id = 1").Count(&count)
	return count == 0
}

// SetDB allows updating the database instance if database connection is changed
func (s *AuthService) SetDB(db *gorm.DB) {
	s.db = db
}

// Tracker returns the rate limiter tracker
func (s *AuthService) Tracker() *auth.LoginAttemptsTracker {
	return s.tracker
}

// SetupInitialAdmin creates the primary administrator during first launch
func (s *AuthService) SetupInitialAdmin(username, password, nickname, email string) (*model.User, error) {
	if !s.IsSetupRequired() {
		return nil, errors.New("系统已完成初始化，管理员账号已存在")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	var emailPtr *string
	email = strings.TrimSpace(email)
	if email != "" {
		emailPtr = &email
	}

	admin := &model.User{
		Username:     username,
		Nickname:     nickname,
		Email:        emailPtr,
		PasswordHash: hash,
		RoleID:       1, // System Admin Role
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.Create(admin).Error; err != nil {
		return nil, err
	}

	s.db.Preload("Role").First(admin, admin.ID)
	return admin, nil
}

type LoginResult struct {
	RequiresMFA      bool        `json:"requires_mfa"`
	HasTOTP          bool        `json:"has_totp"`
	HasPasskey       bool        `json:"has_passkey"`
	Requires2FASetup bool        `json:"requires_2fa_setup"`
	SetupToken       string      `json:"setup_token,omitempty"`
	MFAToken         string      `json:"mfa_token,omitempty"`
	RequiresCaptcha  bool        `json:"requires_captcha"`
	SessionToken     string      `json:"session_token,omitempty"`
	User             *model.User `json:"user,omitempty"`
}

func (s *AuthService) GenerateMFAToken(userID uint) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s.mfaLock.Lock()
	defer s.mfaLock.Unlock()
	s.mfaTokens[tok] = &mfaTokenEntry{
		UserID:    userID,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Attempts:  0,
	}
	return tok
}

func (s *AuthService) GenerateSetupToken(userID uint) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s.setupLock.Lock()
	s.setupTokens[tok] = setupTokenEntry{
		UserID:    userID,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	s.setupLock.Unlock()
	return tok
}

func (s *AuthService) VerifySetupToken(token string) (uint, error) {
	if token == "" {
		return 0, errors.New("missing setup token")
	}
	s.setupLock.RLock()
	entry, ok := s.setupTokens[token]
	s.setupLock.RUnlock()
	if !ok || time.Now().After(entry.ExpiresAt) {
		return 0, errors.New("setup token expired or invalid")
	}
	return entry.UserID, nil
}

func (s *AuthService) RevokeSetupToken(token string) {
	s.setupLock.Lock()
	delete(s.setupTokens, token)
	s.setupLock.Unlock()
}

// Login authenticates user with anti-brute-force protection, progressive freeze, and immediate captcha
func (s *AuthService) Login(account, password, clientIP, userAgent string, captchaSolution string, rememberMe bool) (*LoginResult, string) {
	trackerKey := fmt.Sprintf("%s:%s", clientIP, account)

	// 1. Progressive backoff delay to thwart automated attacks
	if delay := s.tracker.GetDelay(trackerKey); delay > 0 {
		time.Sleep(delay)
	}

	// 2. Find user by username or email
	var user model.User
	err := s.db.Preload("Role").Where("username = ? OR email = ?", account, account).First(&user).Error
	if err != nil {
		s.tracker.RecordFailure(trackerKey)
		// Dummy check to mitigate timing attacks
		_, _ = auth.VerifyPassword(password, "$2a$10$abcdefghijklmnopqrstuuNOPQRSTUVWXYZ0123456789012345678901")
		return nil, common.ErrAuthInvalidCredentials
	}

	// 3. Check if user is permanently banned by administrator (Requirement 10)
	if user.Status == "banned" {
		return nil, common.ErrAuthUserBanned
	}

	// 3.5 Check if user account is not yet activated (Requirement 5)
	if user.Status == "inactive" || user.Status == "pending" {
		return nil, common.ErrAuthUserInactive
	}

	// 4. Check if currently frozen/locked out (Requirement 10)
	if user.LockoutUntil != nil && user.LockoutUntil.After(time.Now()) {
		if user.FailedLoginAttempts >= 15 {
			return nil, common.ErrAuthUserFrozen30D
		} else if user.FailedLoginAttempts >= 10 {
			return nil, common.ErrAuthUserFrozen10M
		}
		return nil, common.ErrAuthUserFrozen3M
	}

	// 5. Check if CAPTCHA is required (Immediately after 1 failure, per requirement 10)
	if user.FailedLoginAttempts >= 1 || s.tracker.RequiresCaptcha(trackerKey) {
		if captchaSolution == "" || !s.captchaManager.VerifyAltcha(captchaSolution) {
			return &LoginResult{RequiresCaptcha: true}, common.ErrAuthCaptchaRequired
		}
	}

	// 6. Verify password
	ok, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		user.FailedLoginAttempts++
		s.tracker.RecordFailure(trackerKey)

		now := time.Now()
		var errCode string
		updates := map[string]any{"failed_login_attempts": user.FailedLoginAttempts}

		if user.FailedLoginAttempts >= 15 {
			until := now.Add(30 * 24 * time.Hour)
			updates["lockout_until"] = &until
			updates["status"] = "frozen"
			errCode = common.ErrAuthUserFrozen30D
		} else if user.FailedLoginAttempts >= 10 {
			until := now.Add(10 * time.Minute)
			updates["lockout_until"] = &until
			updates["status"] = "frozen"
			errCode = common.ErrAuthUserFrozen10M
		} else if user.FailedLoginAttempts >= 5 {
			until := now.Add(3 * time.Minute)
			updates["lockout_until"] = &until
			updates["status"] = "frozen"
			errCode = common.ErrAuthUserFrozen3M
		} else {
			errCode = common.ErrAuthInvalidCredentials
		}

		s.db.Model(&user).Updates(updates)
		slog.Warn("Failed login attempt", "account", account, "attempts", user.FailedLoginAttempts, "ip", clientIP)
		return &LoginResult{RequiresCaptcha: true}, errCode
	}

	// 7. Check 2FA
	var totpSec model.TOTPSecret
	hasConfirmedTOTP := s.db.Where("user_id = ? AND confirmed_at IS NOT NULL", user.ID).First(&totpSec).Error == nil

	var passkeyCount int64
	s.db.Model(&model.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&passkeyCount)
	hasPasskey := passkeyCount > 0

	var sysReq2FA model.Setting
	s.db.Where("key = ?", "require_2fa").First(&sysReq2FA)
	forced2FA := user.Require2FA || sysReq2FA.Value == "true"

	if hasConfirmedTOTP || hasPasskey {
		mfaToken := s.GenerateMFAToken(user.ID)
		return &LoginResult{
			RequiresMFA: true,
			HasTOTP:     hasConfirmedTOTP,
			HasPasskey:  hasPasskey,
			MFAToken:    mfaToken,
			User:        &user,
		}, ""
	}

	if forced2FA {
		setupToken := s.GenerateSetupToken(user.ID)
		return &LoginResult{
			Requires2FASetup: true,
			SetupToken:       setupToken,
			User:             &user,
		}, ""
	}

	// 8. Success: reset attempt counter & lockout, set active if was frozen
	s.tracker.RecordSuccess(trackerKey)
	status := user.Status
	if status == "frozen" {
		status = "active"
	}
	s.db.Model(&user).Updates(map[string]any{
		"failed_login_attempts": 0,
		"lockout_until":         nil,
		"status":                status,
	})

	rawToken, _, err := s.sessionManager.CreateSession(user.ID, clientIP, userAgent, rememberMe)
	if err != nil {
		return nil, common.ErrInternalServerError
	}

	slog.Info("User logged in successfully", "username", user.Username, "ip", clientIP)
	return &LoginResult{
		SessionToken: rawToken,
		User:         &user,
	}, ""
}

// Register creates a new user account if registration is allowed and returns activation token (Requirement 5)
func (s *AuthService) Register(username, email, password, nickname string) (*model.User, string, string) {
	// Check allow_registration
	var regSetting model.Setting
	s.db.Where("key = ?", "allow_registration").First(&regSetting)
	if regSetting.Value != "true" {
		return nil, "", common.ErrRegistrationDisabled
	}

	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if len(username) < 3 {
		return nil, "", common.ErrValidationFailed
	}
	if !strings.Contains(email, "@") {
		return nil, "", common.ErrValidationFailed
	}
	if len(password) < 8 {
		return nil, "", common.ErrAuthPasswordTooShort
	}

	// Check duplicates
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, "", common.ErrAuthUsernameTaken
	}
	s.db.Model(&model.User{}).Where("email = ?", email).Count(&count)
	if count > 0 {
		return nil, "", common.ErrAuthEmailTaken
	}

	// Default role
	var defaultRoleSetting model.Setting
	s.db.Where("key = ?", "default_register_role_id").First(&defaultRoleSetting)
	roleID := uint(2)
	if defaultRoleSetting.Value != "" {
		if r, err := strconv.ParseUint(defaultRoleSetting.Value, 10, 32); err == nil && r > 0 {
			roleID = uint(r)
		}
	}

	hash, _ := auth.HashPassword(password)
	user := &model.User{
		Username:     username,
		Nickname:     nickname,
		Email:        &email,
		PasswordHash: hash,
		RoleID:       roleID,
		Status:       "inactive", // Permanently frozen until email activated (Requirement 5)
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, "", common.ErrDatabaseError
	}

	// Generate activation token valid for 24 hours
	randBytes := make([]byte, 24)
	_, _ = rand.Read(randBytes)
	tokenStr := hex.EncodeToString(randBytes)

	tokenRecord := model.PasswordResetToken{
		UserID:    user.ID,
		Token:     tokenStr,
		TokenType: "activation",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Used:      false,
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&tokenRecord).Error; err != nil {
		return nil, "", common.ErrDatabaseError
	}

	slog.Info("New user registered pending email activation", "username", username, "email", email)
	s.db.Preload("Role").First(user, user.ID)
	return user, tokenStr, ""
}

// ActivateAccount marks an inactive user active after validating the activation token
func (s *AuthService) ActivateAccount(tokenStr string) string {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return common.ErrActivationTokenInvalid
	}

	var tokenRecord model.PasswordResetToken
	err := s.db.Where("token = ? AND token_type = 'activation' AND used = ? AND expires_at > ?", tokenStr, false, time.Now()).First(&tokenRecord).Error
	if err != nil {
		return common.ErrActivationTokenInvalid
	}

	// Update user status to active
	if err := s.db.Model(&model.User{}).Where("id = ?", tokenRecord.UserID).Update("status", "active").Error; err != nil {
		return common.ErrDatabaseError
	}

	// Mark token used
	s.db.Model(&tokenRecord).Update("used", true)
	slog.Info("User account successfully activated", "user_id", tokenRecord.UserID)
	return ""
}

// RequestPasswordReset creates a password reset token (supports email or username)
func (s *AuthService) RequestPasswordReset(accountOrEmail string) (*model.User, string, string) {
	accountOrEmail = strings.TrimSpace(accountOrEmail)
	if accountOrEmail == "" {
		return nil, "", common.ErrValidationFailed
	}

	var user model.User
	if err := s.db.Where("email = ? OR username = ?", accountOrEmail, accountOrEmail).First(&user).Error; err != nil {
		return nil, "", common.ErrAuthUserNotFound
	}

	if user.Email == nil || *user.Email == "" {
		return nil, "", common.ErrAuthUserNotFound
	}

	// Generate secure token
	randBytes := make([]byte, 24)
	_, _ = rand.Read(randBytes)
	tokenStr := hex.EncodeToString(randBytes)

	tokenRecord := model.PasswordResetToken{
		UserID:    user.ID,
		Token:     tokenStr,
		TokenType: "password",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Used:      false,
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&tokenRecord).Error; err != nil {
		return nil, "", common.ErrDatabaseError
	}

	return &user, tokenStr, ""
}

// Request2FAResetByID creates a 2FA reset token for a user by their user ID
func (s *AuthService) Request2FAResetByID(userID uint) (*model.User, string, string) {
	if userID == 0 {
		return nil, "", common.ErrValidationFailed
	}

	var user model.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, "", common.ErrAuthUserNotFound
	}

	if user.Email == nil || *user.Email == "" {
		return nil, "", common.ErrAuthUserNotFound
	}

	randBytes := make([]byte, 24)
	_, _ = rand.Read(randBytes)
	tokenStr := hex.EncodeToString(randBytes)

	tokenRecord := model.PasswordResetToken{
		UserID:    user.ID,
		Token:     tokenStr,
		TokenType: "2fa",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Used:      false,
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&tokenRecord).Error; err != nil {
		return nil, "", common.ErrDatabaseError
	}

	return &user, tokenStr, ""
}

// Request2FAReset creates a 2FA reset token and returns the user and token
func (s *AuthService) Request2FAReset(accountOrEmail string) (*model.User, string, string) {
	accountOrEmail = strings.TrimSpace(accountOrEmail)
	if accountOrEmail == "" {
		return nil, "", common.ErrValidationFailed
	}

	var user model.User
	if err := s.db.Where("email = ? OR username = ?", accountOrEmail, accountOrEmail).First(&user).Error; err != nil {
		return nil, "", common.ErrAuthUserNotFound
	}

	if user.Email == nil || *user.Email == "" {
		return nil, "", common.ErrAuthUserNotFound
	}

	randBytes := make([]byte, 24)
	_, _ = rand.Read(randBytes)
	tokenStr := hex.EncodeToString(randBytes)

	tokenRecord := model.PasswordResetToken{
		UserID:    user.ID,
		Token:     tokenStr,
		TokenType: "2fa",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Used:      false,
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&tokenRecord).Error; err != nil {
		return nil, "", common.ErrDatabaseError
	}

	return &user, tokenStr, ""
}

// Reset2FAWithToken validates 2FA reset token, clears TOTP secrets and disables 2FA for the account
func (s *AuthService) Reset2FAWithToken(tokenStr string) string {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return common.ErrResetTokenInvalid
	}

	var tokenRecord model.PasswordResetToken
	if err := s.db.Where("token = ? AND token_type = '2fa' AND used = ? AND expires_at > ?", tokenStr, false, time.Now()).First(&tokenRecord).Error; err != nil {
		return common.ErrResetTokenInvalid
	}

	var user model.User
	if err := s.db.Where("id = ?", tokenRecord.UserID).First(&user).Error; err != nil {
		return common.ErrAuthUserNotFound
	}

	// Delete TOTP Secret
	s.db.Where("user_id = ?", user.ID).Delete(&model.TOTPSecret{})

	// Delete WebAuthn Passkeys
	s.db.Where("user_id = ?", user.ID).Delete(&model.WebAuthnCredential{})

	// Delete Recovery Codes
	s.db.Where("user_id = ?", user.ID).Delete(&model.RecoveryCode{})

	// Disable Require2FA on user and bump token version
	s.db.Model(&user).Updates(map[string]any{
		"require_2fa":   false,
		"token_version": gorm.Expr("token_version + 1"),
	})

	// Revoke all existing sessions for this user
	_ = s.sessionManager.RevokeAllUserSessions(user.ID)

	// Mark token as used
	s.db.Model(&tokenRecord).Update("used", true)
	slog.Info("2FA successfully reset via email token", "username", user.Username)

	return ""
}

// ResetPasswordWithToken validates reset token and resets user password, clearing 30-day freeze (Requirement 10)
func (s *AuthService) ResetPasswordWithToken(tokenStr, newPassword string) string {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return common.ErrResetTokenInvalid
	}
	if len(newPassword) < 8 {
		return common.ErrAuthPasswordTooShort
	}

	var tokenRecord model.PasswordResetToken
	if err := s.db.Where("token = ? AND token_type != '2fa' AND used = ? AND expires_at > ?", tokenStr, false, time.Now()).First(&tokenRecord).Error; err != nil {
		return common.ErrResetTokenInvalid
	}

	var user model.User
	if err := s.db.Where("id = ?", tokenRecord.UserID).First(&user).Error; err != nil {
		return common.ErrAuthUserNotFound
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return common.ErrInternalServerError
	}

	// Update user password and clear any frozen / lockout state (Requirement 10)
	s.db.Model(&user).Updates(map[string]any{
		"password_hash":         hash,
		"token_version":         gorm.Expr("token_version + 1"),
		"failed_login_attempts": 0,
		"lockout_until":         nil,
		"status":                "active",
	})

	// Revoke all active sessions on password reset
	_ = s.sessionManager.RevokeAllUserSessions(user.ID)

	// Mark token as used
	s.db.Model(&tokenRecord).Update("used", true)
	slog.Info("Password reset completed via token", "username", user.Username)

	return ""
}

// VerifyMFAAndLogin verifies the MFA pending token and TOTP code, with rate limiting and brute force protection
func (s *AuthService) VerifyMFAAndLogin(mfaToken, code, clientIP, userAgent string, rememberMe bool) (string, error) {
	if mfaToken == "" {
		return "", errors.New("missing MFA token, please login first")
	}

	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return "", errors.New("invalid 2FA code format")
	}

	// 1. Validate MFA pending token
	s.mfaLock.Lock()
	entry, exists := s.mfaTokens[mfaToken]
	if !exists || time.Now().After(entry.ExpiresAt) {
		if exists {
			delete(s.mfaTokens, mfaToken)
		}
		s.mfaLock.Unlock()
		return "", errors.New("MFA session expired or invalid, please login again")
	}

	if entry.Attempts >= 5 {
		delete(s.mfaTokens, mfaToken)
		s.mfaLock.Unlock()
		return "", errors.New("too many failed MFA attempts, please login again")
	}
	entry.Attempts++
	userID := entry.UserID
	s.mfaLock.Unlock()

	// 2. IP-based rate limiting via tracker
	trackerKey := fmt.Sprintf("mfa:%s", clientIP)
	if s.tracker.IsLocked(trackerKey) {
		return "", errors.New("too many failed MFA attempts from this IP, please try again later")
	}

	var sec model.TOTPSecret
	if err := s.db.Where("user_id = ? AND confirmed_at IS NOT NULL", userID).First(&sec).Error; err != nil {
		return "", errors.New("2FA not configured")
	}

	plainSecret, err := s.totpManager.DecryptSecret(sec.SecretEncrypted)
	if err != nil {
		return "", errors.New("failed to decrypt 2FA secret")
	}

	if !s.totpManager.Validate(code, plainSecret) {
		s.tracker.RecordFailure(trackerKey)
		return "", errors.New("invalid 2FA code")
	}

	// 3. Success: revoke MFA token and reset attempts tracker
	s.mfaLock.Lock()
	delete(s.mfaTokens, mfaToken)
	s.mfaLock.Unlock()
	s.tracker.RecordSuccess(trackerKey)

	rawToken, _, err := s.sessionManager.CreateSession(userID, clientIP, userAgent, rememberMe)
	return rawToken, err
}

// CreateApiToken creates a Personal Access Token
func (s *AuthService) CreateApiToken(userID uint, name, scopes string, expiresDays int) (string, *model.ApiToken, error) {
	rawToken, err := auth.GenerateRandomToken(24)
	if err != nil {
		return "", nil, err
	}

	fullToken := "uimg_" + rawToken
	prefix := fullToken[:10]
	hash := auth.HashToken(fullToken)

	var expiresAt *time.Time
	if expiresDays > 0 {
		exp := time.Now().Add(time.Duration(expiresDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	token := &model.ApiToken{
		UserID:    userID,
		Name:      name,
		Prefix:    prefix,
		TokenHash: hash,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(token).Error; err != nil {
		return "", nil, err
	}

	return fullToken, token, nil
}

// GenerateCaptchaChallenge issues a new ALTCHA challenge
func (s *AuthService) GenerateCaptchaChallenge() (*auth.AltchaChallenge, error) {
	return s.captchaManager.GenerateAltchaChallenge(50000)
}

// ChangeUsername updates a user's username and migrates all existing public_paths
// like GitHub does when a user or organization renames
func (s *AuthService) ChangeUsername(userID uint, newUsername string) error {
	newUsername = strings.ToLower(strings.TrimSpace(newUsername))
	if len(newUsername) < 3 || len(newUsername) > 32 {
		return errors.New("username must be between 3 and 32 characters")
	}

	if pathclean.IsReservedWord(newUsername) {
		return fmt.Errorf("username '%s' is a reserved system path", newUsername)
	}

	var user model.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return errors.New("user not found")
	}

	if user.Username == newUsername {
		return nil
	}

	// Check if username is already taken
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", newUsername).Count(&count)
	if count > 0 {
		return errors.New("username is already taken")
	}

	oldPrefix := user.Username + "/"
	newPrefix := newUsername + "/"

	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Update user record
		if err := tx.Model(&user).Update("username", newUsername).Error; err != nil {
			return err
		}

		// 2. Migrate all file public_path entries for this user
		var files []model.FileRecord
		tx.Where("user_id = ?", userID).Find(&files)

		for _, f := range files {
			if strings.HasPrefix(f.PublicPath, oldPrefix) {
				newPublicPath := newPrefix + strings.TrimPrefix(f.PublicPath, oldPrefix)
				if err := tx.Model(&model.FileRecord{}).Where("id = ?", f.ID).Update("public_path", newPublicPath).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *AuthService) SessionManager() *auth.SessionManager {
	return s.sessionManager
}

func (s *AuthService) TOTPManager() *auth.TOTPManager {
	return s.totpManager
}

func (s *AuthService) PasskeyManager() *auth.PasskeyManager {
	return s.passkeyManager
}

func (s *AuthService) DB() *gorm.DB {
	return s.db
}

func HashSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
