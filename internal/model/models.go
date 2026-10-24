package model

import (
	"time"

	"gorm.io/gorm"
)

// User represents an account in UtopiaImages
type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Username     string         `gorm:"uniqueIndex;size:64;not null" json:"username"` // immutable after creation
	Nickname     string         `gorm:"size:64" json:"nickname"`
	Email        string         `gorm:"uniqueIndex;size:255" json:"email"`
	PasswordHash string         `gorm:"size:255;not null" json:"-"`
	Avatar       string         `gorm:"size:512" json:"avatar"`
	Role         string         `gorm:"size:32;default:user" json:"role"` // admin / user
	GroupID      uint           `gorm:"index;default:1" json:"group_id"`
	Group        *Group         `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	Status       string         `gorm:"size:32;default:active" json:"status"` // active / disabled
	StorageLimit *int64         `json:"storage_limit"`                        // bytes; nil = inherit from group
	UsedBytes    int64          `gorm:"default:0" json:"used_bytes"`
	FileCount    int64          `gorm:"default:0" json:"file_count"`
	TokenVersion int64          `gorm:"default:1" json:"token_version"`
	Locale       string         `gorm:"size:16;default:zh-CN" json:"locale"`
	Require2FA   bool           `gorm:"default:false" json:"require_2fa"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Group represents a user group with permissions and storage quota
type Group struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Name                string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	IsBuiltin           bool      `gorm:"default:false" json:"is_builtin"`
	Permissions         string    `gorm:"type:text" json:"permissions"` // JSON array of string permissions
	DefaultStorageLimit int64     `gorm:"default:10737418240" json:"default_storage_limit"` // default 10GB
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Session represents an active login session
type Session struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"` // sha256 hash of raw token
	UserID        uint      `gorm:"index;not null" json:"user_id"`
	User          *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	IP            string    `gorm:"size:64" json:"ip"`
	UserAgent     string    `gorm:"size:512" json:"user_agent"`
	ExpiresAt     time.Time `gorm:"index;not null" json:"expires_at"`
	MfaVerifiedAt *time.Time `json:"mfa_verified_at"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
}

// ApiToken represents a personal access token (PAT)
type ApiToken struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"index;not null" json:"user_id"`
	Name       string     `gorm:"size:64;not null" json:"name"`
	Prefix     string     `gorm:"size:16;index" json:"prefix"` // first 8 chars for identification
	TokenHash  string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	Scopes     string     `gorm:"type:text" json:"scopes"` // comma separated
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// TOTPSecret stores the encrypted secret for 2FA
type TOTPSecret struct {
	UserID          uint      `gorm:"primaryKey" json:"user_id"`
	SecretEncrypted string    `gorm:"type:text;not null" json:"-"`
	ConfirmedAt     *time.Time `json:"confirmed_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// WebAuthnCredential stores registered Passkeys
type WebAuthnCredential struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserID          uint      `gorm:"index;not null" json:"user_id"`
	Name            string    `gorm:"size:64;not null" json:"name"`
	CredentialID    string    `gorm:"uniqueIndex;size:512;not null" json:"credential_id"`
	PublicKey       string    `gorm:"type:text;not null" json:"-"`
	AttestationType string    `gorm:"size:64" json:"attestation_type"`
	AAGUID          string    `gorm:"size:64" json:"aaguid"`
	SignCount       uint32    `gorm:"default:0" json:"sign_count"`
	CreatedAt       time.Time `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at"`
}

// RecoveryCode stores hashed one-time backup codes
type RecoveryCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index;not null" json:"user_id"`
	CodeHash  string     `gorm:"size:64;not null" json:"-"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// StorageBackend represents a physical storage instance
type StorageBackend struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	Driver    string    `gorm:"size:32;default:local" json:"driver"` // local / s3
	Config    string    `gorm:"type:text" json:"config"`             // JSON backend configuration
	IsDefault bool      `gorm:"default:false" json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StoragePolicy defines upload naming and URL routing rules per user
type StoragePolicy struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"index;not null" json:"user_id"`
	BackendID      uint      `gorm:"index;not null" json:"backend_id"`
	Name           string    `gorm:"size:64;not null" json:"name"`
	UseRoot        bool      `gorm:"default:false" json:"use_root"` // admin-only: map to domain root instead of /{username}/
	PathPrefix     string    `gorm:"size:128" json:"path_prefix"`   // e.g. "upload" -> /username/upload/... or /upload/...
	NamingRule     string    `gorm:"size:255;default:{Y}/{m}/{d}/{random:16}.{ext}" json:"naming_rule"`
	AccessHeaders  string    `gorm:"type:text" json:"access_headers"` // JSON headers (CORS, cache-control)
	IsDefault      bool      `gorm:"default:false" json:"is_default"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FileRecord represents an uploaded file
type FileRecord struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	UserID              uint           `gorm:"index;not null" json:"user_id"`
	PolicyID            uint           `gorm:"index;not null" json:"policy_id"`
	BackendID           uint           `gorm:"index;not null" json:"backend_id"`
	PublicPath          string         `gorm:"uniqueIndex;size:512;not null" json:"public_path"` // e.g. "alice/upload/2026/05/23/a.jpg" or "upload/26/a.jpg"
	StorageKey          string         `gorm:"size:512;not null" json:"storage_key"`             // key inside storage driver
	DisplayName         string         `gorm:"size:255;not null" json:"display_name"`
	MimeType            string         `gorm:"size:128;not null" json:"mime_type"`
	Size                int64          `gorm:"not null" json:"size"`
	Width               int            `json:"width"`
	Height              int            `json:"height"`
	Sha256              string         `gorm:"size:64;index" json:"sha256"`
	IsImage             bool           `gorm:"index" json:"is_image"`
	SourceKey           string         `gorm:"size:512" json:"source_key,omitempty"` // if converted, key of original unconverted file
	SourceSize          int64          `json:"source_size,omitempty"`
	WatermarkMode       string         `gorm:"size:16;default:inherit" json:"watermark_mode"` // inherit / on / off
	WatermarkTemplateID *uint          `json:"watermark_template_id,omitempty"`
	SignVersion         int64          `gorm:"default:1" json:"sign_version"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

// WatermarkTemplate stores watermark configuration
type WatermarkTemplate struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OwnerID   *uint     `gorm:"index" json:"owner_id"` // nil = site-wide global template
	Name      string    `gorm:"size:64;not null" json:"name"`
	Layers    string    `gorm:"type:text;not null" json:"layers"` // JSON definition of layers
	Version   int64     `gorm:"default:1" json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ImagePreset defines resize and format options for on-the-fly requests
type ImagePreset struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:32;not null" json:"name"` // e.g. "thumb", "medium"
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Fit       string    `gorm:"size:16;default:contain" json:"fit"` // contain / cover / fill
	Format    string    `gorm:"size:16;default:webp" json:"format"`
	Quality   int       `gorm:"default:85" json:"quality"`
	Version   int64     `gorm:"default:1" json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ImportJob tracks asynchronous import tasks
type ImportJob struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	PolicyID  uint      `json:"policy_id"`
	Options   string    `gorm:"type:text" json:"options"` // JSON options: preservePath, includeTopDir, conflict
	Status    string    `gorm:"size:32;default:pending" json:"status"` // pending / running / completed / failed
	Total     int       `gorm:"default:0" json:"total"`
	Done      int       `gorm:"default:0" json:"done"`
	Failed    int       `gorm:"default:0" json:"failed"`
	Report    string    `gorm:"type:text" json:"report"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Setting key-value store for site configuration
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
