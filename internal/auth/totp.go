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

	"github.com/pquerna/otp/totp"
)

type TOTPManager struct {
	encryptionKey []byte
}

func NewTOTPManager(secretKey string) *TOTPManager {
	sum := sha256.Sum256([]byte(secretKey))
	return &TOTPManager{encryptionKey: sum[:]}
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
	return totp.Validate(passcode, secret)
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

func (tm *TOTPManager) DecryptSecret(encrypted string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(tm.encryptionKey)
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
