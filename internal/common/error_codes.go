package common

import (
	"github.com/gin-gonic/gin"
)

// Standardized Error Codes for Frontend i18n
const (
	// Authentication & Security
	ErrAuthInvalidCredentials   = "AUTH_INVALID_CREDENTIALS"
	ErrAuthUserNotFound         = "AUTH_USER_NOT_FOUND"
	ErrAuthUserBanned           = "AUTH_USER_BANNED"
	ErrAuthUserFrozen3M         = "AUTH_USER_FROZEN_3M"
	ErrAuthUserFrozen10M        = "AUTH_USER_FROZEN_10M"
	ErrAuthUserFrozen30D        = "AUTH_USER_FROZEN_30D"
	ErrAuthCaptchaRequired      = "AUTH_CAPTCHA_REQUIRED"
	ErrAuthCaptchaFailed        = "AUTH_CAPTCHA_FAILED"
	ErrAuth2FARequired          = "AUTH_2FA_REQUIRED"
	ErrAuth2FAInvalid           = "AUTH_2FA_INVALID"
	ErrAuthUnauthorized         = "AUTH_UNAUTHORIZED"
	ErrAuthForbidden            = "AUTH_FORBIDDEN"
	ErrAuthPasswordTooShort     = "AUTH_PASSWORD_TOO_SHORT"
	ErrAuthPasswordMismatch     = "AUTH_PASSWORD_MISMATCH"
	ErrAuthOldPasswordWrong     = "AUTH_OLD_PASSWORD_WRONG"
	ErrAuthUsernameTaken        = "AUTH_USERNAME_TAKEN"
	ErrAuthEmailTaken           = "AUTH_EMAIL_TAKEN"
	ErrAuthUserInactive         = "AUTH_USER_INACTIVE"
	ErrRegistrationDisabled     = "REGISTRATION_DISABLED"
	ErrRegistrationSMTPRequired = "REGISTRATION_SMTP_REQUIRED"
	ErrSMTPNotConfigured        = "SMTP_NOT_CONFIGURED"
	ErrResetTokenInvalid        = "RESET_TOKEN_INVALID"
	ErrActivationTokenInvalid   = "ACTIVATION_TOKEN_INVALID"

	// Files & Storage
	ErrFileTooLarge     = "FILE_TOO_LARGE"
	ErrFileNotFound     = "FILE_NOT_FOUND"
	ErrFileStorageFull  = "FILE_STORAGE_FULL"
	ErrFileUploadFailed = "FILE_UPLOAD_FAILED"
	ErrFileDeleteFailed = "FILE_DELETE_FAILED"
	ErrFileInvalidType  = "FILE_INVALID_TYPE"
	ErrHotlinkForbidden = "HOTLINK_FORBIDDEN"

	// Common & System
	ErrValidationFailed    = "VALIDATION_FAILED"
	ErrDatabaseError       = "DATABASE_ERROR"
	ErrInternalServerError = "INTERNAL_SERVER_ERROR"
	ErrOperationFailed     = "OPERATION_FAILED"
)

// RespondError sends a standardized error response containing machine-readable error codes
func RespondError(c *gin.Context, httpStatus int, errorCode string) {
	c.JSON(httpStatus, gin.H{
		"code":  errorCode,
		"error": errorCode,
	})
}
