package pathclean

import (
	"path"
	"strings"
)

// CleanPath cleans an incoming URL or storage path to prevent directory traversal
func CleanPath(p string) string {
	// Normalize backslashes to slashes
	p = strings.ReplaceAll(p, "\\", "/")
	// Remove null bytes
	p = strings.ReplaceAll(p, "\x00", "")
	// Clean using standard path.Clean
	cleaned := path.Clean("/" + p)
	// Trim leading slash
	return strings.TrimPrefix(cleaned, "/")
}

// IsReservedWord checks if first segment is a system reserved path
func IsReservedWord(segment string) bool {
	reserved := map[string]bool{
		"api":         true,
		"_app":        true,
		"console":     true,
		"login":       true,
		"logout":      true,
		"setup":       true,
		"dashboard":   true,
		"admin":       true,
		"favicon.ico": true,
		"favicon.svg": true,
		"robots.txt":  true,
		".well-known": true,
		"static":      true,
		"assets":      true,
		"auth":        true,
		"user":        true,
		"settings":    true,
		"docs":        true,
		"help":        true,
		"www":         true,
		"root":        true,
		"system":      true,
		"healthz":     true,
		"upload":      true,
		"files":       true,
	}
	return reserved[strings.ToLower(strings.TrimSpace(segment))]
}

// IsValidUsername validates that a username contains only 3-32 characters of [a-zA-Z0-9_-] and is not reserved
func IsValidUsername(username string) bool {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for i := 0; i < len(username); i++ {
		ch := username[i]
		isLower := ch >= 'a' && ch <= 'z'
		isUpper := ch >= 'A' && ch <= 'Z'
		isDigit := ch >= '0' && ch <= '9'
		isSymbol := ch == '_' || ch == '-'
		if !isLower && !isUpper && !isDigit && !isSymbol {
			return false
		}
	}
	return !IsReservedWord(username)
}
