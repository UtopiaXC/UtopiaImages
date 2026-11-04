package pathclean

import (
	"path"
	"strings"
)

// CleanPath cleans an incoming URL or storage path to prevent directory traversal and injection
func CleanPath(p string) string {
	// Normalize backslashes to slashes
	p = strings.ReplaceAll(p, "\\", "/")
	// Remove null bytes
	p = strings.ReplaceAll(p, "\x00", "")
	// Filter characters that can break query strings, fragments, HTML/XML injection
	for _, bad := range []string{"?", "#", "\"", "'", "<", ">", ")", "("} {
		p = strings.ReplaceAll(p, bad, "")
	}
	// Clean using standard path.Clean
	cleaned := path.Clean("/" + p)
	// Trim leading slash
	return strings.TrimPrefix(cleaned, "/")
}

// IsReservedWord checks if first segment is a system reserved path.
// All reserved top-level prefixes/paths are strictly <= 3 characters,
// and any segment <= 3 characters is automatically treated as reserved.
func IsReservedWord(segment string) bool {
	s := strings.ToLower(strings.TrimSpace(segment))
	if len(s) <= 3 {
		return true
	}

	reserved := map[string]bool{
		"u":             true,
		"usr":           true,
		"dsh":           true,
		"api":           true,
		"stp":           true,
		"ast":           true,
		"hlt":           true,
		"_app":          true,
		"console":       true,
		"login":         true,
		"logout":        true,
		"setup":         true,
		"dashboard":     true,
		"admin":         true,
		"administrator": true,
		"guest":         true,
		"system":        true,
		"root":          true,
		"favicon.ico":   true,
		"favicon.svg":   true,
		"robots.txt":    true,
		".well-known":   true,
		"static":        true,
		"assets":        true,
		"auth":          true,
		"user":          true,
		"settings":      true,
		"docs":          true,
		"help":          true,
		"www":           true,
		"healthz":       true,
		"upload":        true,
		"square":        true,
		"gallery":       true,
		"files":         true,
		"public":        true,
		"config":        true,
	}
	return reserved[s]
}

// IsValidUsername validates that a username contains only 4-32 characters of [a-zA-Z0-9_-],
// must strictly have length > 3 (cannot be <= 3), and cannot be a system reserved word.
func IsValidUsername(username string) bool {
	username = strings.ToLower(strings.TrimSpace(username))
	// Username length MUST be strictly greater than 3 (length >= 4) and at most 32
	if len(username) <= 3 || len(username) > 32 {
		return false
	}
	if IsReservedWord(username) {
		return false
	}
	for i := 0; i < len(username); i++ {
		ch := username[i]
		isLower := ch >= 'a' && ch <= 'z'
		isDigit := ch >= '0' && ch <= '9'
		isSymbol := ch == '_' || ch == '-'
		if !isLower && !isDigit && !isSymbol {
			return false
		}
	}
	return true
}
