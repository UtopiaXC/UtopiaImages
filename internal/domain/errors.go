package domain

import "errors"

// Standard Domain Error Codes
var (
	ErrNotFound             = errors.New("resource not found")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrForbidden            = errors.New("forbidden")
	ErrStorageLimitExceeded = errors.New("user storage capacity limit exceeded")
	ErrFileTooLarge         = errors.New("file size exceeds single file limit")
	ErrInvalidFileType      = errors.New("file type not allowed")
	ErrInvalidNamespace     = errors.New("invalid namespace or reserved path")
	ErrNamespaceCollision   = errors.New("namespace collides with existing user or admin route")
	ErrCaptchaRequired      = errors.New("captcha verification required")
	ErrInvalidCaptcha       = errors.New("invalid captcha token")
	ErrMfaRequired          = errors.New("two-factor authentication required")
	ErrInvalidCredentials   = errors.New("invalid username or password")
)

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(code, message string, details any) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Details: details,
	}
}
