package service

import (
	"testing"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/stretchr/testify/assert"
)

func TestMFATokenLifecycle(t *testing.T) {
	db := setupTestDB(t)
	sessionMgr := auth.NewSessionManager(db)
	totpMgr := auth.NewTOTPManager("test-encryption-key-32-bytes---")

	authSvc := NewAuthService(db, sessionMgr, totpMgr, nil, nil)

	// 1. Missing MFA token rejected
	_, err := authSvc.VerifyMFAAndLogin("", "123456", "127.0.0.1", "test-agent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing MFA token")

	// 2. Generate valid MFA token
	userID := uint(42)
	mfaToken := authSvc.GenerateMFAToken(userID)
	assert.NotEmpty(t, mfaToken)

	// 3. Invalid code length rejected without burning attempts
	_, err = authSvc.VerifyMFAAndLogin(mfaToken, "123", "127.0.0.1", "test-agent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid 2FA code format")

	// 4. Repeated failed attempts increment attempt count and lock after 5 attempts
	for i := 0; i < 5; i++ {
		_, err = authSvc.VerifyMFAAndLogin(mfaToken, "123456", "127.0.0.1", "test-agent", false)
		assert.Error(t, err)
	}

	// 5. 6th attempt with same token must be rejected because token reached 5 attempts
	_, err = authSvc.VerifyMFAAndLogin(mfaToken, "123456", "127.0.0.1", "test-agent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many failed MFA attempts")

	// 6. 7th attempt must be rejected because token was deleted
	_, err = authSvc.VerifyMFAAndLogin(mfaToken, "123456", "127.0.0.1", "test-agent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MFA session expired or invalid")

	// 6. Expired MFA token is rejected
	expiredToken := authSvc.GenerateMFAToken(userID)
	authSvc.mfaLock.Lock()
	authSvc.mfaTokens[expiredToken].ExpiresAt = time.Now().Add(-1 * time.Minute)
	authSvc.mfaLock.Unlock()

	_, err = authSvc.VerifyMFAAndLogin(expiredToken, "123456", "127.0.0.1", "test-agent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MFA session expired or invalid")
}
