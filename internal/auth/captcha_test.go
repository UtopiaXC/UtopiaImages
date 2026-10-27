package auth

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoginAttemptsTrackerConcurrency(t *testing.T) {
	tracker := NewLoginAttemptsTracker()
	var wg sync.WaitGroup

	// Concurrently invoke tracker methods from 100 goroutines
	numGoroutines := 100
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("user-%d", id%5) // Share keys among goroutines to cause collisions
			tracker.RecordFailure(key)
			_ = tracker.IsLocked(key)
			_ = tracker.RequiresCaptcha(key)
			_ = tracker.GetDelay(key)
			if id%3 == 0 {
				tracker.RecordSuccess(key)
			}
		}(i)
	}

	wg.Wait()
}

func TestCaptchaManagerReplayPrevention(t *testing.T) {
	secret := "test-secret-key-1234567890123456"
	cm := NewCaptchaManager(secret, "")

	challenge, err := cm.GenerateAltchaChallenge(50000)
	assert.NoError(t, err)
	assert.NotEmpty(t, challenge.Challenge)

	// An already used challenge should be rejected immediately
	cm.usedMu.Lock()
	cm.usedChallenges[challenge.Challenge] = time.Now().Add(5 * time.Minute)
	cm.usedMu.Unlock()

	dummyPayload := fmt.Sprintf(`{"algorithm":"SHA-256","challenge":"%s","number":1,"salt":"%s","signature":"%s"}`,
		challenge.Challenge, challenge.Salt, challenge.Signature)

	assert.False(t, cm.VerifyAltcha(dummyPayload), "Expected already-used challenge to be rejected")
}

func TestCaptchaManagerExpiredSalt(t *testing.T) {
	secret := "test-secret-key-1234567890123456"
	cm := NewCaptchaManager(secret, "")

	// Construct a salt with expired timestamp (10 minutes ago)
	expiredTimestamp := time.Now().Add(-10 * time.Minute).Unix()
	expiredSalt := fmt.Sprintf("%x?expires=%d", "randomsalt", expiredTimestamp)

	payload := fmt.Sprintf(`{"algorithm":"SHA-256","challenge":"dummy","number":1,"salt":"%s","signature":"dummy"}`,
		expiredSalt)

	assert.False(t, cm.VerifyAltcha(payload), "Expected expired salt to be rejected")
}
