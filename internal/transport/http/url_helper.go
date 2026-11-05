package http

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func isRemoteTrustedProxy(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	remote := c.Request.RemoteAddr
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// ExtractRequestBaseURL detects the scheme://host from the incoming HTTP request.
func ExtractRequestBaseURL(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	host := c.Request.Host
	if isRemoteTrustedProxy(c) {
		if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = strings.TrimSpace(strings.Split(xfh, ",")[0])
		}
	}
	if host == "" {
		return ""
	}
	proto := "http"
	if c.Request.TLS != nil || (isRemoteTrustedProxy(c) && strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https")) {
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
//   - If incoming request matches any preset domain, prioritize that preset domain (substituting wildcards if needed).
//   - If restrict_domains is disabled, allow dynamic domain/LAN IP matching for image links.
//   - If restrict_domains is enabled, enforce trusted preset domains.
//
// 2. If no preset domains are configured:
//   - Use the current request domain.
func ResolveBaseURL(c *gin.Context, db *gorm.DB, fallback string) string {
	var presetURLs []string
	restrictDomains := false

	// 1. Fetch DB configured base_url
	if db != nil {
		var s model.Setting
		if err := db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			presetURLs = splitDomainList(s.Value)
		}
		var rSetting model.Setting
		if err := db.Where("key = 'restrict_domains'").First(&rSetting).Error; err == nil && rSetting.Value == "true" {
			restrictDomains = true
		}
	}
	if len(presetURLs) == 0 && fallback != "" {
		presetURLs = splitDomainList(fallback)
	}

	// 2. If request exists
	if c != nil && c.Request != nil {
		reqBaseURL := ExtractRequestBaseURL(c)
		reqHost := ExtractHostOnly(c.Request.Host)
		if isRemoteTrustedProxy(c) {
			if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
				reqHost = ExtractHostOnly(strings.Split(xfh, ",")[0])
			}
		}

		if len(presetURLs) > 0 {
			// Check if incoming request matches any preset domain
			for _, p := range presetURLs {
				presetHost := ExtractHostOnly(p)
				if matchHost(presetHost, reqHost) {
					if strings.Contains(p, "*") {
						return strings.Replace(p, "*."+presetHost[2:], reqHost, 1)
					}
					return p
				}
			}
			// When domain restriction is disabled, allow dynamic LAN IP / multi-domain link generation
			if !restrictDomains && reqBaseURL != "" {
				return reqBaseURL
			}
			// Fallback to the primary non-wildcard preset domain
			for _, p := range presetURLs {
				if !strings.Contains(p, "*") {
					return p
				}
			}
			return presetURLs[0]
		} else if reqBaseURL != "" {
			return reqBaseURL
		}
	}

	// 3. Fallback when c is nil or request has no host
	if len(presetURLs) > 0 {
		for _, p := range presetURLs {
			if !strings.Contains(p, "*") {
				return p
			}
		}
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

// ResolveTrustedMailBaseURL resolves base URL strictly for security emails (password reset, 2FA reset, activation)
// It NEVER falls back to unvalidated client request headers.
func ResolveTrustedMailBaseURL(c *gin.Context, db *gorm.DB, fallback string) string {
	var presetURLs []string
	if db != nil {
		var s model.Setting
		if err := db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			presetURLs = splitDomainList(s.Value)
		}
	}
	if len(presetURLs) == 0 && fallback != "" {
		presetURLs = splitDomainList(fallback)
	}

	if c != nil && c.Request != nil && len(presetURLs) > 0 {
		reqHost := ExtractHostOnly(c.Request.Host)
		if isRemoteTrustedProxy(c) {
			if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
				reqHost = ExtractHostOnly(strings.Split(xfh, ",")[0])
			}
		}
		for _, p := range presetURLs {
			presetHost := ExtractHostOnly(p)
			if matchHost(presetHost, reqHost) {
				if strings.Contains(p, "*") {
					return strings.Replace(p, "*."+presetHost[2:], reqHost, 1)
				}
				return p
			}
		}
	}

	for _, p := range presetURLs {
		if !strings.Contains(p, "*") {
			return p
		}
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
	// If current request exists, format wildcards and include current domain if allowed
	if c != nil && c.Request != nil {
		reqURL := ExtractRequestBaseURL(c)
		reqHost := ExtractHostOnly(c.Request.Host)
		if reqURL != "" {
			found := false
			for i, u := range urls {
				uHost := ExtractHostOnly(u)
				if matchHost(uHost, reqHost) {
					found = true
					if strings.Contains(u, "*") {
						urls[i] = strings.Replace(u, "*."+uHost[2:], reqHost, 1)
					}
					break
				}
				if strings.EqualFold(u, reqURL) || uHost == reqHost {
					found = true
					break
				}
			}
			if !found && IsDomainAllowed(c, db, fallback) {
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
	if isRemoteTrustedProxy(c) {
		if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
			reqHost = ExtractHostOnly(strings.Split(xfh, ",")[0])
		}
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
