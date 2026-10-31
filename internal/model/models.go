package model

import (
	"time"

	"gorm.io/gorm"
)

// Role defines user permissions, storage quota limit per user, and allowed storage policies
type Role struct {
	ID          uint             `gorm:"primaryKey" json:"id"`
	Name        string           `gorm:"uniqueIndex;size:64;not null" json:"name"` // e.g. "admin", "user"
	DisplayName string           `gorm:"size:64;not null" json:"display_name"`
	QuotaBytes  int64            `gorm:"default:0" json:"quota_bytes"`   // 0 = unlimited
	IsSystem    bool             `gorm:"default:false" json:"is_system"` // admin role cannot be deleted
	Policies    []*StoragePolicy `gorm:"many2many:role_policies;joinForeignKey:role_id;joinReferences:policy_id;" json:"policies,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// StoragePolicy defines physical storage location and naming rules
type StoragePolicy struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	Driver           string    `gorm:"size:32;default:local" json:"driver"` // currently "local", extensible to "oss", "s3"
	Config           string    `gorm:"type:text" json:"config"`             // JSON backend driver configuration
	MaxCapacityBytes int64     `gorm:"default:0" json:"max_capacity_bytes"` // 0 = unlimited
	UsedBytes        int64     `gorm:"default:0" json:"used_bytes"`
	IsDefault        bool      `gorm:"default:false" json:"is_default"`
	PathPrefix       string    `gorm:"size:128" json:"path_prefix"`
	NamingRule       string    `gorm:"size:255;default:{Y}/{m}/{d}/{random:16}.{ext}" json:"naming_rule"`
	Roles            []*Role   `gorm:"many2many:role_policies;joinForeignKey:policy_id;joinReferences:role_id;" json:"roles,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RolePolicy is the junction table connecting Roles and StoragePolicies (3NF)
type RolePolicy struct {
	RoleID    uint `gorm:"primaryKey;index" json:"role_id"`
	PolicyID  uint `gorm:"primaryKey;index" json:"policy_id"`
	CreatedAt time.Time
}

// User represents an account in UtopiaImages
type User struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	Username              string         `gorm:"uniqueIndex;size:64;not null" json:"username"` // immutable after creation
	Nickname              string         `gorm:"size:64" json:"nickname"`
	Email                 *string        `gorm:"uniqueIndex;size:255" json:"email"`
	PasswordHash          string         `gorm:"size:255;not null" json:"-"`
	Avatar                string         `gorm:"size:512" json:"avatar"`
	RoleID                uint           `gorm:"index;not null" json:"role_id"`
	Role                  *Role          `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Status                string         `gorm:"size:32;default:active" json:"status"` // active / disabled
	UsedBytes             int64          `gorm:"default:0" json:"used_bytes"`
	FileCount             int64          `gorm:"default:0" json:"file_count"`
	TokenVersion          int64          `gorm:"default:1" json:"token_version"`
	Locale                string         `gorm:"size:16;default:zh-CN" json:"locale"`
	Require2FA            bool           `gorm:"column:require_2fa;default:false" json:"require_2fa"`
	FailedLoginAttempts   int            `gorm:"default:0" json:"failed_login_attempts"`
	LockoutUntil          *time.Time     `json:"lockout_until"`
	HotlinkEnabled        bool           `gorm:"default:false" json:"hotlink_enabled"`
	HotlinkAllowEmpty     bool           `gorm:"default:true" json:"hotlink_allow_empty"`
	HotlinkAllowedDomains string         `gorm:"type:text" json:"hotlink_allowed_domains"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// Session represents an active login session
type Session struct {
	ID            string     `gorm:"primaryKey;size:64" json:"id"` // sha256 hash of raw token
	UserID        uint       `gorm:"index;not null" json:"user_id"`
	User          *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TokenVersion  int64      `gorm:"default:1" json:"token_version"`
	IP            string     `gorm:"size:64" json:"ip"`
	UserAgent     string     `gorm:"size:512" json:"user_agent"`
	ExpiresAt     time.Time  `gorm:"index;not null" json:"expires_at"`
	MfaVerifiedAt *time.Time `json:"mfa_verified_at"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSeenAt    time.Time  `json:"last_seen_at"`
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
	UserID          uint       `gorm:"primaryKey" json:"user_id"`
	SecretEncrypted string     `gorm:"type:text;not null" json:"-"`
	ConfirmedAt     *time.Time `json:"confirmed_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// WebAuthnCredential stores registered Passkeys
type WebAuthnCredential struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	UserID          uint       `gorm:"index;not null" json:"user_id"`
	Name            string     `gorm:"size:64;not null" json:"name"`
	CredentialID    string     `gorm:"uniqueIndex;size:512;not null" json:"credential_id"`
	PublicKey       string     `gorm:"type:text;not null" json:"-"`
	AttestationType string     `gorm:"size:64" json:"attestation_type"`
	AAGUID          string     `gorm:"size:64" json:"aaguid"`
	SignCount       uint32     `gorm:"default:0" json:"sign_count"`
	CredentialData  string     `gorm:"type:text" json:"-"`
	BackupEligible  bool       `gorm:"default:false" json:"backup_eligible"`
	BackupState     bool       `gorm:"default:false" json:"backup_state"`
	CreatedAt       time.Time  `json:"created_at"`
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

// FileRecord represents an uploaded file
type FileRecord struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	UserID                uint           `gorm:"index;not null" json:"user_id"`
	PolicyID              uint           `gorm:"index;not null" json:"policy_id"`
	PublicPath            string         `gorm:"uniqueIndex;size:512;not null" json:"public_path"` // e.g. "uploads/2026/05/23/a.jpg"
	StorageKey            string         `gorm:"size:512;not null" json:"storage_key"`             // key inside storage driver
	DisplayName           string         `gorm:"size:255;not null" json:"display_name"`
	MimeType              string         `gorm:"size:128;not null" json:"mime_type"`
	Size                  int64          `gorm:"not null" json:"size"`
	Width                 int            `json:"width"`
	Height                int            `json:"height"`
	Sha256                string         `gorm:"size:64;index" json:"sha256"`
	IsImage               bool           `gorm:"index" json:"is_image"`
	SourceKey             string         `gorm:"size:512" json:"source_key,omitempty"` // if converted, key of original unconverted file
	SourceSize            int64          `json:"source_size,omitempty"`
	WatermarkMode         string         `gorm:"size:16;default:inherit" json:"watermark_mode"` // inherit / on / off
	WatermarkTemplateID   *uint          `json:"watermark_template_id,omitempty"`
	HotlinkMode           string         `gorm:"size:16;default:inherit" json:"hotlink_mode"` // inherit / on / off
	HotlinkAllowedDomains string         `gorm:"type:text" json:"hotlink_allowed_domains"`
	SignVersion           int64          `gorm:"default:1" json:"sign_version"`
	TotalCalls            int64          `gorm:"-" json:"total_calls"`
	TotalUV               int64          `gorm:"-" json:"total_uv"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// TemporaryLink records authorized temporary unwatermarked raw links
type TemporaryLink struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	FileID    uint       `gorm:"index;not null" json:"file_id"`
	UserID    uint       `gorm:"index;not null" json:"user_id"`
	Token     string     `gorm:"uniqueIndex;size:64;not null" json:"token"`
	ExpiresAt time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// WatermarkTemplate stores watermark configuration
type WatermarkTemplate struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OwnerID   *uint     `gorm:"index" json:"owner_id"` // nil = site-wide global template
	Name      string    `gorm:"index;size:64;not null" json:"name"`
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
	Options   string    `gorm:"type:text" json:"options"`              // JSON options: preservePath, includeTopDir, conflict
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
	Key         string    `gorm:"primaryKey;size:64" json:"key"`
	Value       string    `gorm:"type:text" json:"value"`
	BinaryValue []byte    `gorm:"type:blob" json:"-"`
	ContentType string    `gorm:"size:64" json:"content_type,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PasswordResetToken tracks self-service password reset and 2FA reset requests
type PasswordResetToken struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Token     string    `gorm:"uniqueIndex;size:128;not null" json:"token"`
	TokenType string    `gorm:"size:32;default:'password'" json:"token_type"` // password / 2fa
	ExpiresAt time.Time `gorm:"index;not null" json:"expires_at"`
	Used      bool      `gorm:"default:false" json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// ImageCallStat tracks image access frequency by format and size (PV and UV)
type ImageCallStat struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	FileID    uint      `gorm:"index:idx_file_fmt_size,unique;not null" json:"file_id"`
	Format    string    `gorm:"size:32;index:idx_file_fmt_size,unique;not null;default:'original'" json:"format"`
	Size      string    `gorm:"size:64;index:idx_file_fmt_size,unique;not null;default:'original'" json:"size"`
	Count     int64     `gorm:"default:0" json:"count"` // PV
	UV        int64     `gorm:"default:0" json:"uv"`    // UV
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ImageCallVisitor tracks unique visitors for images
type ImageCallVisitor struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	FileID    uint      `gorm:"index:idx_call_visitor,unique;not null" json:"file_id"`
	Format    string    `gorm:"size:32;index:idx_call_visitor,unique;not null;default:'original'" json:"format"`
	Size      string    `gorm:"size:64;index:idx_call_visitor,unique;not null;default:'original'" json:"size"`
	VisitorID string    `gorm:"size:64;index:idx_call_visitor,unique;not null" json:"visitor_id"`
	IP        string    `gorm:"size:64;not null" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}
