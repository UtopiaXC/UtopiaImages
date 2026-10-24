package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
	"gorm.io/gorm"
)

type AuthService struct {
	db             *gorm.DB
	sessionManager *auth.SessionManager
	totpManager    *auth.TOTPManager
	captchaManager *auth.CaptchaManager
	tracker        *auth.LoginAttemptsTracker
}

func NewAuthService(
	db *gorm.DB,
	sm *auth.SessionManager,
	tm *auth.TOTPManager,
	cm *auth.CaptchaManager,
) *AuthService {
	return &AuthService{
		db:             db,
		sessionManager: sm,
		totpManager:    tm,
		captchaManager: cm,
		tracker:        auth.NewLoginAttemptsTracker(),
	}
}

// IsSetupRequired checks if no administrator account exists yet
func (s *AuthService) IsSetupRequired() bool {
	var count int64
	s.db.Model(&model.User{}).Where("role = 'admin'").Count(&count)
	return count == 0
}

// SetupInitialAdmin creates the primary administrator during first launch
func (s *AuthService) SetupInitialAdmin(username, password, nickname, email string) (*model.User, error) {
	if !s.IsSetupRequired() {
		return nil, errors.New("system is already initialized")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	admin := &model.User{
		Username:     username,
		Nickname:     nickname,
		Email:        email,
		PasswordHash: hash,
		Role:         "admin",
		GroupID:      1, // Admin group
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.Create(admin).Error; err != nil {
		return nil, err
	}

	// Create default admin storage policy with UseRoot = true
	adminPolicy := &model.StoragePolicy{
		UserID:     admin.ID,
		BackendID:  1, // Default local backend
		Name:       "Default Admin Policy",
		UseRoot:    true,
		PathPrefix: "upload",
		NamingRule: "{Y}/{m}/{d}/{random:16}.{ext}",
		IsDefault:  true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	s.db.Create(adminPolicy)

	return admin, nil
}

type LoginResult struct {
	RequiresMFA     bool   `json:"requires_mfa"`
	RequiresCaptcha bool   `json:"requires_captcha"`
	SessionToken    string `json:"session_token,omitempty"`
	User            *model.User `json:"user,omitempty"`
}

// Login authenticates user with anti-brute-force protection, captcha, and 2FA
func (s *AuthService) Login(username, password, clientIP, userAgent string, captchaSolution string, rememberMe bool) (*LoginResult, error) {
	trackerKey := fmt.Sprintf("%s:%s", clientIP, username)

	// 1. Check if locked out
	if s.tracker.IsLocked(trackerKey) {
		return nil, errors.New("too many failed login attempts, please try again in 15 minutes")
	}

	// 2. Progressive backoff delay to thwart high-speed automated attacks
	if delay := s.tracker.GetDelay(trackerKey); delay > 0 {
		time.Sleep(delay)
	}

	// 3. Check if CAPTCHA is required
	if s.tracker.RequiresCaptcha(trackerKey) {
		if captchaSolution == "" || !s.captchaManager.VerifyAltcha(captchaSolution) {
			return &LoginResult{RequiresCaptcha: true}, errors.New("captcha verification required")
		}
	}

	// 4. Verify user credentials
	var user model.User
	err := s.db.Preload("Group").Where("username = ? OR email = ?", username, username).First(&user).Error
	if err != nil {
		s.tracker.RecordFailure(trackerKey)
		return nil, errors.New("invalid username or password")
	}

	if user.Status != "active" {
		return nil, errors.New("account disabled")
	}

	ok, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		s.tracker.RecordFailure(trackerKey)
		return nil, errors.New("invalid username or password")
	}

	// 5. Check 2FA
	if user.Require2FA {
		var totpSec model.TOTPSecret
		if err := s.db.Where("user_id = ? AND confirmed_at IS NOT NULL", user.ID).First(&totpSec).Error; err == nil {
			return &LoginResult{
				RequiresMFA: true,
				User:        &user,
			}, nil
		}
	}

	// 6. Success: reset attempt counter & issue session
	s.tracker.RecordSuccess(trackerKey)

	rawToken, _, err := s.sessionManager.CreateSession(user.ID, clientIP, userAgent, rememberMe)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		SessionToken: rawToken,
		User:         &user,
	}, nil
}

// VerifyMFAAndLogin verifies the TOTP code and finishes login
func (s *AuthService) VerifyMFAAndLogin(userID uint, code, clientIP, userAgent string, rememberMe bool) (string, error) {
	var sec model.TOTPSecret
	if err := s.db.Where("user_id = ? AND confirmed_at IS NOT NULL", userID).First(&sec).Error; err != nil {
		return "", errors.New("2FA not configured")
	}

	plainSecret, err := s.totpManager.DecryptSecret(sec.SecretEncrypted)
	if err != nil {
		return "", errors.New("failed to decrypt 2FA secret")
	}

	if !s.totpManager.Validate(code, plainSecret) {
		return "", errors.New("invalid 2FA code")
	}

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

func (s *AuthService) DB() *gorm.DB {
	return s.db
}

func HashSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

