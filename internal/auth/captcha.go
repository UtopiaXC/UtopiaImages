package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CaptchaManager struct {
	secretKey          string
	turnstileSecretKey string
	usedChallenges     map[string]time.Time
	usedMu             sync.Mutex
}

func NewCaptchaManager(secretKey, turnstileSecretKey string) *CaptchaManager {
	return &CaptchaManager{
		secretKey:          secretKey,
		turnstileSecretKey: turnstileSecretKey,
		usedChallenges:     make(map[string]time.Time),
	}
}

type AltchaChallenge struct {
	Algorithm string `json:"algorithm"` // SHA-256
	Challenge string `json:"challenge"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
	MaxNumber int64  `json:"maxnumber"`
}

type AltchaPayload struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int64  `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// GenerateAltchaChallenge generates a proof-of-work challenge with 5-minute expiration
func (cm *CaptchaManager) GenerateAltchaChallenge(difficulty int64) (*AltchaChallenge, error) {
	if difficulty <= 0 {
		difficulty = 50000 // default difficulty (~50k hashes, solves in ~100-300ms in browser)
	}

	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(5 * time.Minute).Unix()
	salt := fmt.Sprintf("%s?expires=%d", hex.EncodeToString(saltBytes), expiresAt)

	// Pick random number within [0, difficulty]
	nBig, err := rand.Int(rand.Reader, big.NewInt(difficulty))
	if err != nil {
		return nil, err
	}
	secretNumber := nBig.Int64()

	// Challenge = SHA-256(salt + number)
	data := fmt.Sprintf("%s%d", salt, secretNumber)
	sum := sha256.Sum256([]byte(data))
	challenge := hex.EncodeToString(sum[:])

	// Signature = HMAC-SHA256(challenge:salt, secretKey)
	mac := hmac.New(sha256.New, []byte(cm.secretKey))
	mac.Write([]byte(challenge + ":" + salt))
	signature := hex.EncodeToString(mac.Sum(nil))

	return &AltchaChallenge{
		Algorithm: "SHA-256",
		Challenge: challenge,
		Salt:      salt,
		Signature: signature,
		MaxNumber: difficulty,
	}, nil
}

// VerifyAltcha verifies client's POW answer with anti-replay and expiration checks
func (cm *CaptchaManager) VerifyAltcha(payloadJSON string) bool {
	var p AltchaPayload
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		return false
	}

	// 1. Verify HMAC signature (support bound challenge:salt as well as legacy challenge)
	mac1 := hmac.New(sha256.New, []byte(cm.secretKey))
	mac1.Write([]byte(p.Challenge + ":" + p.Salt))
	expectedSignature1 := hex.EncodeToString(mac1.Sum(nil))

	mac2 := hmac.New(sha256.New, []byte(cm.secretKey))
	mac2.Write([]byte(p.Challenge))
	expectedSignature2 := hex.EncodeToString(mac2.Sum(nil))

	if !hmac.Equal([]byte(p.Signature), []byte(expectedSignature1)) && !hmac.Equal([]byte(p.Signature), []byte(expectedSignature2)) {
		return false
	}

	// 2. Check expiration timestamp in salt if present
	if idx := strings.Index(p.Salt, "?expires="); idx != -1 {
		expStr := p.Salt[idx+len("?expires="):]
		if exp, err := strconv.ParseInt(expStr, 10, 64); err == nil {
			if time.Now().Unix() > exp {
				return false // Expired challenge
			}
		}
	}

	// 3. Verify POW solution
	data := fmt.Sprintf("%s%d", p.Salt, p.Number)
	sum := sha256.Sum256([]byte(data))
	calculatedChallenge := hex.EncodeToString(sum[:])
	if calculatedChallenge != p.Challenge {
		return false
	}

	// 4. Prevent replay attacks: ensure challenge can only be verified once
	cm.usedMu.Lock()
	defer cm.usedMu.Unlock()

	now := time.Now()
	// Prune old expired entries periodically
	if len(cm.usedChallenges) > 1000 {
		for k, expTime := range cm.usedChallenges {
			if now.After(expTime) {
				delete(cm.usedChallenges, k)
			}
		}
	}

	if _, used := cm.usedChallenges[p.Challenge]; used {
		return false // Challenge already solved and used
	}

	// Record as used with 10-minute validity window
	cm.usedChallenges[p.Challenge] = now.Add(10 * time.Minute)
	return true
}

// VerifyTurnstile verifies Cloudflare Turnstile token
func (cm *CaptchaManager) VerifyTurnstile(token, remoteIP string) bool {
	if cm.turnstileSecretKey == "" {
		return false
	}

	form := url.Values{}
	form.Set("secret", cm.turnstileSecretKey)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	return result.Success
}

// LoginAttemptsTracker tracks consecutive failed attempts per IP / Username in memory (thread-safe)
type LoginAttemptsTracker struct {
	mutex       sync.RWMutex
	attempts    map[string]int
	lockouts    map[string]time.Time
	lastAttempt map[string]time.Time
}

func NewLoginAttemptsTracker() *LoginAttemptsTracker {
	t := &LoginAttemptsTracker{
		attempts:    make(map[string]int),
		lockouts:    make(map[string]time.Time),
		lastAttempt: make(map[string]time.Time),
	}
	go t.startCleanup()
	return t
}

func (t *LoginAttemptsTracker) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		t.cleanup()
	}
}

func (t *LoginAttemptsTracker) cleanup() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	now := time.Now()
	for k, until := range t.lockouts {
		if now.After(until) {
			delete(t.lockouts, k)
			delete(t.attempts, k)
			delete(t.lastAttempt, k)
		}
	}
	for k, last := range t.lastAttempt {
		if now.Sub(last) > 1*time.Hour {
			delete(t.attempts, k)
			delete(t.lastAttempt, k)
			if until, ok := t.lockouts[k]; !ok || now.After(until) {
				delete(t.lockouts, k)
			}
		}
	}
}

func (t *LoginAttemptsTracker) RecordFailure(key string) (int, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()
	t.attempts[key]++
	t.lastAttempt[key] = now
	count := t.attempts[key]
	locked := false
	if count >= 5 {
		t.lockouts[key] = now.Add(15 * time.Minute)
		locked = true
	}
	return count, locked
}

func (t *LoginAttemptsTracker) IsLocked(key string) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	until, exists := t.lockouts[key]
	if !exists {
		return false
	}
	if time.Now().After(until) {
		delete(t.lockouts, key)
		t.attempts[key] = 0
		delete(t.lastAttempt, key)
		return false
	}
	return true
}

func (t *LoginAttemptsTracker) RecordSuccess(key string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	delete(t.attempts, key)
	delete(t.lockouts, key)
	delete(t.lastAttempt, key)
}

func (t *LoginAttemptsTracker) RequiresCaptcha(key string) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.attempts[key] >= 1
}

func (t *LoginAttemptsTracker) GetDelay(key string) time.Duration {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	count := t.attempts[key]
	if count <= 1 {
		return 0
	}
	// Progressive backoff: 200ms, 400ms, 800ms, 1600ms, max 3s
	delay := time.Duration(100*(1<<(count-1))) * time.Millisecond
	if delay > 3*time.Second {
		delay = 3 * time.Second
	}
	return delay
}

// Helper to convert string to int
func ParseIntSafe(s string, def int) int {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return val
}
