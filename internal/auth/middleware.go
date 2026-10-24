package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	CtxUserKey   = "current_user"
	CtxUserIDKey = "current_user_id"
	CtxRoleKey   = "current_role"
	SessionCookieName = "uimg_session"
)

func AuthMiddleware(sm *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var user *model.User

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
					if u, _, err := sm.ValidateApiToken(rawToken); err == nil {
						user = u
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
			c.Set(CtxUserKey, user)
			c.Set(CtxUserIDKey, user.ID)
			c.Set(CtxRoleKey, user.Role)
		}

		c.Next()
	}
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
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		if user.Role == "admin" {
			c.Next()
			return
		}

		if user.Group != nil && user.Group.Permissions != "" {
			var perms []string
			if err := json.Unmarshal([]byte(user.Group.Permissions), &perms); err == nil {
				for _, p := range perms {
					if p == "*" || p == permission {
						c.Next()
						return
					}
				}
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    "FORBIDDEN",
			"message": "Insufficient permissions for " + permission,
		})
	}
}
