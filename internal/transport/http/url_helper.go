package http

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ResolveBaseURL dynamically detects the appropriate base URL for the client request.
// It prioritizes:
// 1. Client Origin / Referer (if valid public domain)
// 2. Client X-Forwarded-Host / Host (if valid public domain)
// 3. Database configured base_url (if not localhost)
// 4. Fallback configured URL (if not localhost)
// 5. Client Host / Origin (even if localhost in dev)
// 6. Root-relative "" (never outputs localhost to clients accessing via public domains)
func ResolveBaseURL(c *gin.Context, db *gorm.DB, fallback string) string {
	// 1. First check if client is accessing via a public domain via Origin or Referer
	if c != nil {
		if orig := strings.TrimRight(c.Request.Header.Get("Origin"), "/"); isPublicHTTPURL(orig) {
			return orig
		}
		if ref := c.Request.Header.Get("Referer"); ref != "" {
			if u, err := url.Parse(ref); err == nil && u.Host != "" && isPublicHost(u.Host) {
				return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
			}
		}
		// Check X-Forwarded-Host or Host
		host := c.Request.Host
		if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = xfh
		}
		if isPublicHost(host) {
			proto := "http"
			if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
				proto = "https"
			}
			return fmt.Sprintf("%s://%s", proto, host)
		}
	}

	// 2. Next, check DB settings
	if db != nil {
		var s model.Setting
		if err := db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			urls := splitDomainList(s.Value)
			// Prefer non-localhost
			for _, u := range urls {
				if isPublicHTTPURL(u) {
					return u
				}
			}
			if len(urls) > 0 {
				return urls[0]
			}
		}
	}

	// 3. Next, check fallback
	if fallback != "" {
		urls := splitDomainList(fallback)
		for _, u := range urls {
			if isPublicHTTPURL(u) {
				return u
			}
		}
		if len(urls) > 0 && !strings.Contains(urls[0], "localhost") && !strings.Contains(urls[0], "127.0.0.1") {
			return urls[0]
		}
	}

	// 4. If request exists, use its host/proto even if local/dev
	if c != nil {
		host := c.Request.Host
		if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = xfh
		}
		if host != "" {
			proto := "http"
			if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
				proto = "https"
			}
			return fmt.Sprintf("%s://%s", proto, host)
		}
	}

	return ""
}

// GetConfiguredBaseURLs returns all configured base URLs
func GetConfiguredBaseURLs(c *gin.Context, db *gorm.DB, fallback string) []string {
	var raw string
	if db != nil {
		var s model.Setting
		if err := db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			raw = s.Value
		}
	}
	if raw == "" {
		raw = fallback
	}
	urls := splitDomainList(raw)
	// If current request has a public domain, ensure it's in the list
	if c != nil {
		reqURL := ResolveBaseURL(c, db, fallback)
		if reqURL != "" {
			found := false
			for _, u := range urls {
				if strings.EqualFold(u, reqURL) {
					found = true
					break
				}
			}
			if !found {
				urls = append([]string{reqURL}, urls...)
			}
		}
	}
	return urls
}

func splitDomainList(raw string) []string {
	var result []string
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	for _, f := range fields {
		f = strings.TrimRight(strings.TrimSpace(f), "/")
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

func isPublicHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	if colon := strings.Index(h, ":"); colon != -1 {
		h = h[:colon]
	}
	return h != "localhost" && h != "127.0.0.1" && h != "0.0.0.0" && h != "::1"
}

func isPublicHTTPURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	return isPublicHost(u.Host)
}
