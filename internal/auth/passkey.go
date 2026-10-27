package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
)

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "http://localhost:2983"
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func parseBaseURLs(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	var result []string
	seen := make(map[string]bool)
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			norm := normalizeBaseURL(f)
			if !seen[norm] {
				seen[norm] = true
				result = append(result, norm)
			}
		}
	}
	if len(result) == 0 {
		result = []string{normalizeBaseURL(raw)}
	}
	return result
}

func createWebAuthn(rawBaseURLs string) (*webauthn.WebAuthn, error) {
	urls := parseBaseURLs(rawBaseURLs)
	primaryURL := urls[0]
	u, err := url.Parse(primaryURL)
	primaryRPID := "localhost"
	if err == nil && u.Hostname() != "" {
		primaryRPID = u.Hostname()
	}

	origins := []string{
		"http://localhost:5173",
		"http://localhost:2983",
	}

	for _, rawURL := range urls {
		cleanURL := normalizeBaseURL(rawURL)
		pu, err := url.Parse(cleanURL)
		if err == nil && pu.Hostname() != "" {
			origins = append(origins, cleanURL)
			if pu.Hostname() != "localhost" {
				origins = append(origins, "https://"+pu.Host, "http://"+pu.Host)
			}
		}
	}

	originSet := make(map[string]bool)
	var uniqueOrigins []string
	for _, o := range origins {
		trimmed := strings.TrimRight(o, "/")
		if trimmed != "" && !originSet[trimmed] {
			originSet[trimmed] = true
			uniqueOrigins = append(uniqueOrigins, trimmed)
		}
	}

	return webauthn.New(&webauthn.Config{
		RPDisplayName: "Utopia Images",
		RPID:          primaryRPID,
		RPOrigins:     uniqueOrigins,
	})
}

type PasskeyManager struct {
	db          *gorm.DB
	mu          sync.RWMutex
	webAuthn    *webauthn.WebAuthn
	baseURL     string
	sessions    map[string]*sessionEntry
	sessionLock sync.Mutex
}

type sessionEntry struct {
	sessionData *webauthn.SessionData
	userID      uint
	expiresAt   time.Time
}

func NewPasskeyManager(db *gorm.DB, baseURL string) (*PasskeyManager, error) {
	w, err := createWebAuthn(baseURL)
	if err != nil {
		return nil, err
	}

	pm := &PasskeyManager{
		db:       db,
		webAuthn: w,
		baseURL:  normalizeBaseURL(baseURL),
		sessions: make(map[string]*sessionEntry),
	}

	// Periodically cleanup expired WebAuthn sessions
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			pm.cleanupExpired()
		}
	}()

	return pm, nil
}

func (pm *PasskeyManager) getWebAuthn() *webauthn.WebAuthn {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.webAuthn
}

func (pm *PasskeyManager) UpdateBaseURL(baseURL string) error {
	w, err := createWebAuthn(baseURL)
	if err != nil {
		return err
	}
	pm.mu.Lock()
	pm.webAuthn = w
	pm.baseURL = normalizeBaseURL(baseURL)
	pm.mu.Unlock()
	return nil
}

func (pm *PasskeyManager) RPID() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.webAuthn != nil && pm.webAuthn.Config != nil {
		return pm.webAuthn.Config.RPID
	}
	return "localhost"
}

func (pm *PasskeyManager) BaseURL() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.baseURL
}

func (pm *PasskeyManager) BaseURLs() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return parseBaseURLs(pm.baseURL)
}

func (pm *PasskeyManager) SetDB(db *gorm.DB) {
	pm.db = db
}

func (pm *PasskeyManager) cleanupExpired() {
	pm.sessionLock.Lock()
	defer pm.sessionLock.Unlock()
	now := time.Now()
	for k, v := range pm.sessions {
		if now.After(v.expiresAt) {
			delete(pm.sessions, k)
		}
	}
}

// WebAuthnUser adapter implementing webauthn.User interface
type WebAuthnUser struct {
	User        *model.User
	Credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte {
	return []byte(strconv.FormatUint(uint64(u.User.ID), 10))
}

func (u *WebAuthnUser) WebAuthnName() string {
	return u.User.Username
}

func (u *WebAuthnUser) WebAuthnDisplayName() string {
	if u.User.Nickname != "" {
		return u.User.Nickname
	}
	return u.User.Username
}

func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

func (pm *PasskeyManager) loadUserCredentials(userID uint) []webauthn.Credential {
	var records []model.WebAuthnCredential
	pm.db.Where("user_id = ?", userID).Find(&records)

	var creds []webauthn.Credential
	for _, r := range records {
		if r.CredentialData != "" {
			var cred webauthn.Credential
			if err := json.Unmarshal([]byte(r.CredentialData), &cred); err == nil {
				cred.Authenticator.SignCount = r.SignCount
				creds = append(creds, cred)
				continue
			}
		}

		rawID, err := base64.RawURLEncoding.DecodeString(r.CredentialID)
		if err != nil {
			rawID, _ = base64.StdEncoding.DecodeString(r.CredentialID)
		}
		rawPubKey, _ := base64.StdEncoding.DecodeString(r.PublicKey)
		rawAAGUID, _ := base64.StdEncoding.DecodeString(r.AAGUID)

		creds = append(creds, webauthn.Credential{
			ID:              rawID,
			PublicKey:       rawPubKey,
			AttestationType: r.AttestationType,
			Flags: webauthn.CredentialFlags{
				BackupEligible: r.BackupEligible,
				BackupState:    r.BackupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    rawAAGUID,
				SignCount: r.SignCount,
			},
		})
	}
	return creds
}

func (pm *PasskeyManager) resolveCeremonyRPIDAndOrigin(requestHost string) (string, string) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	defaultRPID := "localhost"
	if pm.webAuthn != nil && pm.webAuthn.Config != nil {
		defaultRPID = pm.webAuthn.Config.RPID
	}

	if requestHost == "" {
		return defaultRPID, ""
	}

	// Clean host (strip port if present)
	host := strings.ToLower(requestHost)
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	if pm.webAuthn != nil && pm.webAuthn.Config != nil {
		for _, o := range pm.webAuthn.Config.RPOrigins {
			if ou, err := url.Parse(o); err == nil && strings.ToLower(ou.Hostname()) == host {
				scheme := "https"
				if ou.Scheme != "" {
					scheme = ou.Scheme
				}
				return host, fmt.Sprintf("%s://%s", scheme, host)
			}
		}
	}

	return defaultRPID, ""
}

// BeginRegistration starts Passkey registration flow with optional requestHost
func (pm *PasskeyManager) BeginRegistration(user *model.User, requestHost ...string) (*protocol.CredentialCreation, string, error) {
	creds := pm.loadUserCredentials(user.ID)
	waUser := &WebAuthnUser{User: user, Credentials: creds}

	var regOpts []webauthn.RegistrationOption
	if len(requestHost) > 0 && requestHost[0] != "" {
		rpID, origin := pm.resolveCeremonyRPIDAndOrigin(requestHost[0])
		if rpID != "" {
			regOpts = append(regOpts, webauthn.WithRegistrationRelyingPartyID(rpID))
		}
		if origin != "" {
			regOpts = append(regOpts, webauthn.WithRegistrationOrigin(origin))
		}
	}

	options, sessionData, err := pm.getWebAuthn().BeginRegistration(waUser, regOpts...)
	if err != nil {
		return nil, "", err
	}

	sessionKey, _ := GenerateRandomToken(16)
	pm.sessionLock.Lock()
	pm.sessions[sessionKey] = &sessionEntry{
		sessionData: sessionData,
		userID:      user.ID,
		expiresAt:   time.Now().Add(5 * time.Minute),
	}
	pm.sessionLock.Unlock()

	return options, sessionKey, nil
}

// FinishRegistration completes Passkey registration
func (pm *PasskeyManager) FinishRegistration(sessionKey, passkeyName string, r *protocol.ParsedCredentialCreationData) (*model.WebAuthnCredential, error) {
	pm.sessionLock.Lock()
	entry, exists := pm.sessions[sessionKey]
	if exists {
		delete(pm.sessions, sessionKey)
	}
	pm.sessionLock.Unlock()

	if !exists || time.Now().After(entry.expiresAt) {
		return nil, errors.New("registration session expired, please try again")
	}

	var user model.User
	if err := pm.db.Where("id = ?", entry.userID).First(&user).Error; err != nil {
		return nil, errors.New("user not found")
	}

	creds := pm.loadUserCredentials(user.ID)
	waUser := &WebAuthnUser{User: &user, Credentials: creds}

	credential, err := pm.getWebAuthn().CreateCredential(waUser, *entry.sessionData, r)
	if err != nil {
		return nil, err
	}

	if passkeyName == "" {
		passkeyName = "Passkey " + time.Now().Format("2006-01-02 15:04")
	}

	credBytes, _ := json.Marshal(credential)

	credRecord := &model.WebAuthnCredential{
		UserID:          user.ID,
		Name:            passkeyName,
		CredentialID:    base64.RawURLEncoding.EncodeToString(credential.ID),
		PublicKey:       base64.StdEncoding.EncodeToString(credential.PublicKey),
		AttestationType: credential.AttestationType,
		AAGUID:          base64.StdEncoding.EncodeToString(credential.Authenticator.AAGUID),
		SignCount:       credential.Authenticator.SignCount,
		CredentialData:  string(credBytes),
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		CreatedAt:       time.Now(),
	}

	if err := pm.db.Create(credRecord).Error; err != nil {
		return nil, err
	}

	return credRecord, nil
}

// BeginLogin starts Passkey assertion login flow with optional requestHost
func (pm *PasskeyManager) BeginLogin(username string, requestHost ...string) (*protocol.CredentialAssertion, string, error) {
	var options *protocol.CredentialAssertion
	var sessionData *webauthn.SessionData
	var err error

	var loginOpts []webauthn.LoginOption
	if len(requestHost) > 0 && requestHost[0] != "" {
		rpID, origin := pm.resolveCeremonyRPIDAndOrigin(requestHost[0])
		if rpID != "" {
			loginOpts = append(loginOpts, webauthn.WithLoginRelyingPartyID(rpID))
		}
		if origin != "" {
			loginOpts = append(loginOpts, webauthn.WithLoginOrigin(origin))
		}
	}

	if username != "" {
		var user model.User
		if err := pm.db.Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
			return nil, "", errors.New("user not found")
		}
		creds := pm.loadUserCredentials(user.ID)
		if len(creds) == 0 {
			return nil, "", errors.New("no Passkey registered for this user")
		}
		waUser := &WebAuthnUser{User: &user, Credentials: creds}
		options, sessionData, err = pm.getWebAuthn().BeginLogin(waUser, loginOpts...)
	} else {
		// Discoverable credentials (resident key)
		options, sessionData, err = pm.getWebAuthn().BeginDiscoverableLogin(loginOpts...)
	}

	if err != nil {
		return nil, "", err
	}

	sessionKey, _ := GenerateRandomToken(16)
	pm.sessionLock.Lock()
	pm.sessions[sessionKey] = &sessionEntry{
		sessionData: sessionData,
		expiresAt:   time.Now().Add(5 * time.Minute),
	}
	pm.sessionLock.Unlock()

	return options, sessionKey, nil
}

// FinishLogin validates assertion and returns authenticated user
func (pm *PasskeyManager) FinishLogin(sessionKey string, r *protocol.ParsedCredentialAssertionData) (*model.User, error) {
	pm.sessionLock.Lock()
	entry, exists := pm.sessions[sessionKey]
	if exists {
		delete(pm.sessions, sessionKey)
	}
	pm.sessionLock.Unlock()

	if !exists || time.Now().After(entry.expiresAt) {
		return nil, errors.New("login session expired, please try again")
	}

	// Lookup credential in database
	credIDStr := base64.RawURLEncoding.EncodeToString(r.RawID)
	var credRecord model.WebAuthnCredential
	if err := pm.db.Where("credential_id = ?", credIDStr).First(&credRecord).Error; err != nil {
		// Try fallback padding
		credIDStd := base64.StdEncoding.EncodeToString(r.RawID)
		if err := pm.db.Where("credential_id = ?", credIDStd).First(&credRecord).Error; err != nil {
			return nil, errors.New("unrecognized passkey credential")
		}
	}

	var user model.User
	if err := pm.db.Where("id = ?", credRecord.UserID).First(&user).Error; err != nil {
		return nil, errors.New("user not found")
	}
	if user.Status != "active" {
		return nil, errors.New("user account is not active or has been disabled")
	}
	if user.LockoutUntil != nil && user.LockoutUntil.After(time.Now()) {
		return nil, errors.New("user account is temporarily locked")
	}

	creds := pm.loadUserCredentials(user.ID)

	// Fix for Backup Eligible flag inconsistency:
	// If credential was saved without BackupEligible flag, or if the authenticator data flags
	// have updated backup eligibility, sync the loaded credential's BackupEligible and BackupState
	// flags with the incoming assertion authenticator data flags to prevent false validation rejection.
	for i := range creds {
		if bytes.Equal(creds[i].ID, r.RawID) {
			assertionHasBE := r.Response.AuthenticatorData.Flags.HasBackupEligible()
			assertionHasBS := r.Response.AuthenticatorData.Flags.HasBackupState()
			if credRecord.CredentialData == "" || creds[i].Flags.BackupEligible != assertionHasBE {
				creds[i].Flags.BackupEligible = assertionHasBE
				creds[i].Flags.BackupState = assertionHasBS
			}
		}
	}

	waUser := &WebAuthnUser{User: &user, Credentials: creds}

	// For discoverable logins (resident credentials), sessionData.UserID was initially empty.
	// Populate it with the verified user's WebAuthnID so that ValidateLogin's User and Session ID check succeeds.
	if len(entry.sessionData.UserID) == 0 {
		entry.sessionData.UserID = waUser.WebAuthnID()
	} else if !bytes.Equal(entry.sessionData.UserID, waUser.WebAuthnID()) {
		return nil, errors.New("passkey user does not match login user")
	}

	credential, err := pm.getWebAuthn().ValidateLogin(waUser, *entry.sessionData, r)
	if err != nil {
		return nil, err
	}

	// Update sign count, last used, flags, and serialized credential data
	now := time.Now()
	credBytes, _ := json.Marshal(credential)
	updates := map[string]any{
		"sign_count":      credential.Authenticator.SignCount,
		"last_used_at":    &now,
		"backup_eligible": credential.Flags.BackupEligible,
		"backup_state":    credential.Flags.BackupState,
	}
	if len(credBytes) > 0 {
		updates["credential_data"] = string(credBytes)
	}
	pm.db.Model(&credRecord).Updates(updates)

	return &user, nil
}

func (pm *PasskeyManager) ListUserPasskeys(userID uint) []model.WebAuthnCredential {
	var records []model.WebAuthnCredential
	pm.db.Where("user_id = ?", userID).Order("id DESC").Find(&records)
	return records
}

func (pm *PasskeyManager) DeletePasskey(userID, passkeyID uint) error {
	res := pm.db.Where("id = ? AND user_id = ?", passkeyID, userID).Delete(&model.WebAuthnCredential{})
	if res.RowsAffected == 0 {
		return errors.New("passkey not found")
	}
	return nil
}
