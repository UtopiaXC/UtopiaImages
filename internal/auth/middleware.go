package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	CtxUserKey        = "current_user"
	CtxUserIDKey      = "current_user_id"
	CtxRoleKey        = "current_role"
	CtxApiTokenKey    = "current_api_token"
	CtxIsApiTokenKey  = "is_api_token"
	SessionCookieName = "uimg_session"
)

func AuthMiddleware(sm *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var user *model.User
		var apiToken *model.ApiToken
		isApiToken := false

		// 1. Try Cookie session
		if cookie, err := c.Cookie(SessionCookieName); err == nil && cookie != "" {
			if u, _, err := sm.ValidateSession(cookie); err == nil {
				user = u
			}
		}

		// 2. Try Authorization: Bearer <token>
		if user == nil {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				rawToken := strings.TrimPrefix(authHeader, "Bearer ")
				if strings.HasPrefix(rawToken, "uimg_") {
					if u, tok, err := sm.ValidateApiToken(rawToken); err == nil {
						user = u
						apiToken = tok
						isApiToken = true
					}
				} else {
					// Fallback: check if bearer token is a raw session token
					if u, _, err := sm.ValidateSession(rawToken); err == nil {
						user = u
					}
				}
			}
		}

		if user != nil {
			roleName := "user"
			if user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin") {
				roleName = "admin"
			} else if user.Role != nil && user.Role.Name != "" {
				roleName = user.Role.Name
			}
			c.Set(CtxUserKey, user)
			c.Set(CtxUserIDKey, user.ID)
			c.Set(CtxRoleKey, roleName)
			c.Set(CtxIsApiTokenKey, isApiToken)
			if apiToken != nil {
				c.Set(CtxApiTokenKey, apiToken)
			}
		}

		c.Next()
	}
}

// ValidateSessionCSRF checks that state-changing requests using cookie sessions originate from the same site/origin
func ValidateSessionCSRF(c *gin.Context) bool {
	if isApi, ok := c.Get(CtxIsApiTokenKey); ok && isApi.(bool) {
		// Personal Access Tokens (PAT) e.g. PicGo / ShareX / CLI tools are immune to browser CSRF
		return true
	}

	method := c.Request.Method
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
		return true
	}

	// 1. Check Sec-Fetch-Site (modern browsers)
	secFetch := c.GetHeader("Sec-Fetch-Site")
	if secFetch == "cross-site" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":  "CSRF_BLOCKED",
			"error": "Cross-site request blocked",
		})
		return false
	}

	// 2. Check Origin / Referer against request Host
	targetHost := strings.ToLower(c.Request.Host)
	if origin := c.GetHeader("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil {
			if !strings.EqualFold(u.Host, targetHost) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code":  "CSRF_BLOCKED",
					"error": "Cross-origin request blocked",
				})
				return false
			}
		}
	} else if referer := c.Request.Referer(); referer != "" {
		if u, err := url.Parse(referer); err == nil {
			if !strings.EqualFold(u.Host, targetHost) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code":  "CSRF_BLOCKED",
					"error": "Cross-origin referer blocked",
				})
				return false
			}
		}
	}

	return true
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get(CtxUserKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Authentication required",
			})
			return
		}
		if !ValidateSessionCSRF(c) {
			return
		}
		c.Next()
	}
}

// RequireSessionAuth ensures the request comes from an authenticated interactive session, not an API Token
func RequireSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get(CtxUserKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Authentication required",
			})
			return
		}
		if isApi, ok := c.Get(CtxIsApiTokenKey); ok && isApi.(bool) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    "FORBIDDEN",
				"message": "Personal access tokens cannot manage security credentials or tokens",
			})
			return
		}
		if !ValidateSessionCSRF(c) {
			return
		}
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isApi, ok := c.Get(CtxIsApiTokenKey); ok && isApi.(bool) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    "FORBIDDEN",
				"message": "Personal access tokens cannot be used for administrative operations",
			})
			return
		}

		role, exists := c.Get(CtxRoleKey)
		if !exists || role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    "FORBIDDEN",
				"message": "Administrator privileges required",
			})
			return
		}
		c.Next()
	}
}

func HasTokenScope(scopesStr string, requiredPermission string) bool {
	if scopesStr == "" {
		return false
	}
	scopes := strings.Split(scopesStr, ",")
	for _, item := range scopes {
		trimmed := strings.TrimSpace(item)
		if trimmed == "*" || trimmed == "all" || trimmed == requiredPermission {
			return true
		}
	}
	return false
}

func hasTokenScope(scopesStr string, requiredPermission string) bool {
	return HasTokenScope(scopesStr, requiredPermission)
}

func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uVal, exists := c.Get(CtxUserKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Authentication required",
			})
			return
		}

		user := uVal.(*model.User)
		if user.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    "FORBIDDEN",
				"message": "User account is not active",
			})
			return
		}

		if isApi, ok := c.Get(CtxIsApiTokenKey); ok && isApi.(bool) {
			tokVal, tokExists := c.Get(CtxApiTokenKey)
			if !tokExists {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code":    "FORBIDDEN",
					"message": "Invalid API token context",
				})
				return
			}
			apiToken := tokVal.(*model.ApiToken)
			if !hasTokenScope(apiToken.Scopes, permission) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code":    "FORBIDDEN",
					"message": "API token lacks required scope: " + permission,
				})
				return
			}
		}

		c.Next()
	}
}
