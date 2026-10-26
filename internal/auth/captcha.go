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
	"time"
)

type CaptchaManager struct {
	secretKey          string
	turnstileSecretKey string
}

func NewCaptchaManager(secretKey, turnstileSecretKey string) *CaptchaManager {
	return &CaptchaManager{
		secretKey:          secretKey,
		turnstileSecretKey: turnstileSecretKey,
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

// GenerateAltchaChallenge generates a proof-of-work challenge
func (cm *CaptchaManager) GenerateAltchaChallenge(difficulty int64) (*AltchaChallenge, error) {
	if difficulty <= 0 {
		difficulty = 50000 // default difficulty (~50k hashes, solves in ~100-300ms in browser)
	}

	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return nil, err
	}
	salt := hex.EncodeToString(saltBytes)

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

	// Signature = HMAC-SHA256(challenge, secretKey)
	mac := hmac.New(sha256.New, []byte(cm.secretKey))
	mac.Write([]byte(challenge))
	signature := hex.EncodeToString(mac.Sum(nil))

	return &AltchaChallenge{
		Algorithm: "SHA-256",
		Challenge: challenge,
		Salt:      salt,
		Signature: signature,
		MaxNumber: difficulty,
	}, nil
}

// VerifyAltcha verifies client's POW answer
func (cm *CaptchaManager) VerifyAltcha(payloadJSON string) bool {
	var p AltchaPayload
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		return false
	}

	// 1. Verify HMAC signature
	mac := hmac.New(sha256.New, []byte(cm.secretKey))
	mac.Write([]byte(p.Challenge))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(p.Signature), []byte(expectedSignature)) {
		return false
	}

	// 2. Verify solution
	data := fmt.Sprintf("%s%d", p.Salt, p.Number)
	sum := sha256.Sum256([]byte(data))
	calculatedChallenge := hex.EncodeToString(sum[:])

	return calculatedChallenge == p.Challenge
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

// LoginAttemptsTracker tracks consecutive failed attempts per IP / Username in memory
type LoginAttemptsTracker struct {
	attempts map[string]int
	lockouts map[string]time.Time
}

func NewLoginAttemptsTracker() *LoginAttemptsTracker {
	return &LoginAttemptsTracker{
		attempts: make(map[string]int),
		lockouts: make(map[string]time.Time),
	}
}

func (t *LoginAttemptsTracker) RecordFailure(key string) (int, bool) {
	t.attempts[key]++
	count := t.attempts[key]
	locked := false
	if count >= 5 {
		t.lockouts[key] = time.Now().Add(15 * time.Minute)
		locked = true
	}
	return count, locked
}

func (t *LoginAttemptsTracker) IsLocked(key string) bool {
	until, exists := t.lockouts[key]
	if !exists {
		return false
	}
	if time.Now().After(until) {
		delete(t.lockouts, key)
		t.attempts[key] = 0
		return false
	}
	return true
}

func (t *LoginAttemptsTracker) RecordSuccess(key string) {
	delete(t.attempts, key)
	delete(t.lockouts, key)
}

func (t *LoginAttemptsTracker) RequiresCaptcha(key string) bool {
	return t.attempts[key] >= 1
}

func (t *LoginAttemptsTracker) GetDelay(key string) time.Duration {
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
