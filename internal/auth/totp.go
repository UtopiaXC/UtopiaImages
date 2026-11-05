package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"image/png"
	"io"
	"sync"
	"time"

	"github.com/pquerna/otp/totp"
)

type TOTPManager struct {
	encryptionKey []byte
	fallbackKeys  [][]byte
	usedCodes     sync.Map // key: "user:code", val: expiresAt time.Time
}

func NewTOTPManager(secretKey string) *TOTPManager {
	sum := sha256.Sum256([]byte(secretKey))
	knownLegacy := []string{
		"CHANGE_ME_TO_A_SECURE_RANDOM_STRING",
		"default-insecure-secret-key-change-me",
	}
	fallbacks := make([][]byte, 0, len(knownLegacy))
	for _, l := range knownLegacy {
		if l != secretKey {
			s := sha256.Sum256([]byte(l))
			fallbacks = append(fallbacks, s[:])
		}
	}
	tm := &TOTPManager{
		encryptionKey: sum[:],
		fallbackKeys:  fallbacks,
	}
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			tm.cleanupReplay()
		}
	}()
	return tm
}

func (tm *TOTPManager) GenerateSecret(username, issuer string) (secret string, qrPngBase64 string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
	if err != nil {
		return "", "", err
	}

	img, err := key.Image(200, 200)
	if err != nil {
		return "", "", err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", "", err
	}

	qrBase64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	return key.Secret(), qrBase64, nil
}

func (tm *TOTPManager) Validate(passcode, secret string) bool {
	return tm.ValidateWithContext("", passcode, secret)
}

// ValidateWithContext validates passcode against secret and guards against replay within the validity window
func (tm *TOTPManager) ValidateWithContext(userKey, passcode, secret string) bool {
	if !totp.Validate(passcode, secret) {
		return false
	}
	if userKey != "" {
		replayKey := userKey + ":" + passcode
		now := time.Now()
		expiry := now.Add(90 * time.Second)
		val, loaded := tm.usedCodes.LoadOrStore(replayKey, expiry)
		if loaded {
			if t, ok := val.(time.Time); ok && now.Before(t) {
				return false // replay detected
			}
			tm.usedCodes.Store(replayKey, expiry)
		}
	}
	return true
}

func (tm *TOTPManager) cleanupReplay() {
	now := time.Now()
	tm.usedCodes.Range(func(key, value any) bool {
		if t, ok := value.(time.Time); ok && now.After(t) {
			tm.usedCodes.Delete(key)
		}
		return true
	})
}

func (tm *TOTPManager) EncryptSecret(secret string) (string, error) {
	block, err := aes.NewCipher(tm.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSecret decrypts the encrypted secret using primary key or fallback keys
func (tm *TOTPManager) DecryptSecret(encrypted string) (string, error) {
	secret, _, _, err := tm.DecryptSecretWithUpgrade(encrypted)
	return secret, err
}

// DecryptSecretWithUpgrade attempts decryption and signals if upgraded re-encryption is needed
func (tm *TOTPManager) DecryptSecretWithUpgrade(encrypted string) (secret string, reEncrypted string, upgraded bool, err error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", "", false, err
	}

	tryDecrypt := func(key []byte) (string, error) {
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}

		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", err
		}

		nonceSize := gcm.NonceSize()
		if len(data) < nonceSize {
			return "", errors.New("ciphertext too short")
		}

		nonce, ciphertext := data[:nonceSize], data[nonceSize:]
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return "", err
		}

		return string(plaintext), nil
	}

	// Try primary encryption key
	res, err := tryDecrypt(tm.encryptionKey)
	if err == nil {
		return res, "", false, nil
	}

	// Try fallback keys if primary failed
	for _, fb := range tm.fallbackKeys {
		if fbRes, fbErr := tryDecrypt(fb); fbErr == nil {
			if newEnc, reErr := tm.EncryptSecret(fbRes); reErr == nil {
				return fbRes, newEnc, true, nil
			}
			return fbRes, "", false, nil
		}
	}

	return "", "", false, err
}
