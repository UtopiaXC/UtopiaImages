package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

func GenerateRandomToken(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

type SessionManager struct {
	db *gorm.DB
}

func NewSessionManager(db *gorm.DB) *SessionManager {
	return &SessionManager{db: db}
}

// CreateSession generates a new session token, hashes it, and stores the session in DB
func (sm *SessionManager) CreateSession(userID uint, ip, userAgent string, rememberMe bool) (string, *model.Session, error) {
	rawToken, err := GenerateRandomToken(32)
	if err != nil {
		return "", nil, err
	}

	tokenHash := HashToken(rawToken)

	duration := 7 * 24 * time.Hour
	if rememberMe {
		duration = 30 * 24 * time.Hour
	}

	session := &model.Session{
		ID:         tokenHash,
		UserID:     userID,
		IP:         ip,
		UserAgent:  userAgent,
		ExpiresAt:  time.Now().Add(duration),
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}

	if err := sm.db.Create(session).Error; err != nil {
		return "", nil, err
	}

	return rawToken, session, nil
}

// ValidateSession verifies if raw token exists and is not expired
func (sm *SessionManager) ValidateSession(rawToken string) (*model.User, *model.Session, error) {
	tokenHash := HashToken(rawToken)

	var session model.Session
	err := sm.db.Preload("User").Preload("User.Group").Where("id = ? AND expires_at > ?", tokenHash, time.Now()).First(&session).Error
	if err != nil {
		return nil, nil, errors.New("invalid or expired session")
	}

	if session.User == nil || session.User.Status != "active" {
		return nil, nil, errors.New("user account disabled or not found")
	}

	// Update last seen periodically
	if time.Since(session.LastSeenAt) > 10*time.Minute {
		sm.db.Model(&session).Update("last_seen_at", time.Now())
	}

	return session.User, &session, nil
}

// RevokeSession deletes the session
func (sm *SessionManager) RevokeSession(rawToken string) error {
	tokenHash := HashToken(rawToken)
	return sm.db.Where("id = ?", tokenHash).Delete(&model.Session{}).Error
}

// RevokeAllUserSessions revokes all sessions of a user
func (sm *SessionManager) RevokeAllUserSessions(userID uint) error {
	return sm.db.Where("user_id = ?", userID).Delete(&model.Session{}).Error
}

// ValidateApiToken validates personal access tokens
func (sm *SessionManager) ValidateApiToken(rawToken string) (*model.User, *model.ApiToken, error) {
	tokenHash := HashToken(rawToken)

	var token model.ApiToken
	err := sm.db.Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		return nil, nil, errors.New("invalid api token")
	}

	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, nil, errors.New("api token expired")
	}

	var user model.User
	if err := sm.db.Preload("Group").Where("id = ? AND status = 'active'", token.UserID).First(&user).Error; err != nil {
		return nil, nil, errors.New("user not found or inactive")
	}

	// Update last used time
	now := time.Now()
	sm.db.Model(&token).Update("last_used_at", &now)

	return &user, &token, nil
}
