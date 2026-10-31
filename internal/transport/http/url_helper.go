package http

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ExtractRequestBaseURL detects the scheme://host from the incoming HTTP request.
func ExtractRequestBaseURL(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	host := c.Request.Host
	if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	if host == "" {
		return ""
	}
	proto := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https") {
		proto = "https"
	}
	return fmt.Sprintf("%s://%s", proto, host)
}

// ExtractHostOnly removes scheme, path, and port from a URL or domain string.
func ExtractHostOnly(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			raw = u.Host
		}
	}
	if colon := strings.Index(raw, ":"); colon != -1 {
		raw = raw[:colon]
	}
	return strings.ToLower(strings.Trim(raw, "/"))
}

// ResolveBaseURL dynamically detects the appropriate base URL for the client request.
// Rules:
// 1. If system preset domains are configured:
//   - If the incoming request domain matches any preset domain, prioritize that preset domain.
//   - If the incoming request domain is NOT in system presets, use the current request domain.
//
// 2. If no preset domains are configured:
//   - Use the current request domain.
//
// 3. If c == nil (background task):
//   - Return the first preset domain, or fallback URL.
func ResolveBaseURL(c *gin.Context, db *gorm.DB, fallback string) string {
	var presetURLs []string

	// 1. Fetch DB configured base_url
	if db != nil {
		var s model.Setting
		if err := db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			presetURLs = splitDomainList(s.Value)
		}
	}
	if len(presetURLs) == 0 && fallback != "" {
		presetURLs = splitDomainList(fallback)
	}

	// 2. If request exists
	if c != nil && c.Request != nil {
		reqBaseURL := ExtractRequestBaseURL(c)
		reqHost := ExtractHostOnly(c.Request.Host)
		if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
			reqHost = ExtractHostOnly(xfh)
		}

		if len(presetURLs) > 0 {
			// Check if incoming request matches any preset domain
			for _, p := range presetURLs {
				presetHost := ExtractHostOnly(p)
				if matchHost(presetHost, reqHost) {
					return p
				}
			}
			// Request domain is NOT in system presets -> use current domain
			if reqBaseURL != "" {
				return reqBaseURL
			}
		} else if reqBaseURL != "" {
			return reqBaseURL
		}
	}

	// 3. Fallback when c is nil or request has no host
	if len(presetURLs) > 0 {
		return presetURLs[0]
	}
	if fallback != "" {
		urls := splitDomainList(fallback)
		if len(urls) > 0 {
			return urls[0]
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
	// If current request exists, ensure current domain is included if not present
	if c != nil && c.Request != nil {
		reqURL := ExtractRequestBaseURL(c)
		if reqURL != "" {
			found := false
			for _, u := range urls {
				if strings.EqualFold(u, reqURL) || ExtractHostOnly(u) == ExtractHostOnly(reqURL) {
					found = true
					break
				}
			}
			if !found {
				urls = append(urls, reqURL)
			}
		}
	}
	return urls
}

// IsDomainAllowed checks whether access through the request's domain is allowed.
// When 'restrict_domains' is enabled, requests must match configured base_url domains.
func IsDomainAllowed(c *gin.Context, db *gorm.DB, fallback string) bool {
	if db == nil || c == nil || c.Request == nil {
		return true
	}

	// Check if restrict_domains setting is enabled
	var restrictSetting model.Setting
	if err := db.Where("key = 'restrict_domains'").First(&restrictSetting).Error; err != nil || restrictSetting.Value != "true" {
		// Default is disabled
		return true
	}

	// Retrieve preset domains
	var baseSetting model.Setting
	var raw string
	if err := db.Where("key = 'base_url'").First(&baseSetting).Error; err == nil && baseSetting.Value != "" {
		raw = baseSetting.Value
	} else {
		raw = fallback
	}

	presetURLs := splitDomainList(raw)
	if len(presetURLs) == 0 {
		// No domains configured, do not block
		return true
	}

	reqHost := ExtractHostOnly(c.Request.Host)
	if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
		reqHost = ExtractHostOnly(xfh)
	}

	for _, p := range presetURLs {
		presetHost := ExtractHostOnly(p)
		if matchHost(presetHost, reqHost) {
			return true
		}
	}

	return false
}

func matchHost(pattern, host string) bool {
	if pattern == "" || host == "" {
		return false
	}
	if strings.EqualFold(pattern, host) {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // e.g. ".example.com"
		if strings.HasSuffix(strings.ToLower(host), strings.ToLower(suffix)) {
			return true
		}
	}
	return false
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
