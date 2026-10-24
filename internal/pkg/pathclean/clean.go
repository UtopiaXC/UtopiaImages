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
		"api":          true,
		"_app":         true,
		"console":      true,
		"login":        true,
		"logout":       true,
		"setup":        true,
		"dashboard":    true,
		"admin":        true,
		"favicon.ico":  true,
		"robots.txt":   true,
		".well-known":  true,
		"static":       true,
		"assets":       true,
		"auth":         true,
		"user":         true,
		"settings":     true,
		"docs":         true,
		"help":         true,
		"www":          true,
		"root":         true,
		"system":       true,
	}
	return reserved[strings.ToLower(segment)]
}
