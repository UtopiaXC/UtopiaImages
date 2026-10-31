package service

import (
	"net/http/httptest"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.Setting{}, &model.FileRecord{}, &model.User{}, &model.TOTPSecret{})
	assert.NoError(t, err)
	return db
}

func TestCheckReferer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	s := &DispatchService{
		db: db,
	}

	fileWithProtection := &model.FileRecord{
		HotlinkMode:           "on",
		HotlinkAllowedDomains: "allowed.org, *.example.com",
	}

	// 1. Fail-closed on invalid referer URL with control chars
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "http://example.com/photo.jpg", nil)
	req.Header.Set("Referer", "http://[invalid-url-with-control-chars\x7f")
	c.Request = req

	allowed := s.checkReferer(c, fileWithProtection)
	assert.False(t, allowed, "Malformed referer must fail closed")

	// 2. Empty referer with allowEmpty = false (default user HotlinkAllowEmpty is false)
	req = httptest.NewRequest("GET", "http://example.com/photo.jpg", nil)
	c.Request = req
	allowed = s.checkReferer(c, fileWithProtection)
	assert.False(t, allowed, "Empty referer must be rejected when allowEmpty is false")

	// 3. Allowed domain match
	req = httptest.NewRequest("GET", "http://example.com/photo.jpg", nil)
	req.Header.Set("Referer", "https://allowed.org/blog/index.html")
	c.Request = req
	allowed = s.checkReferer(c, fileWithProtection)
	assert.True(t, allowed, "Allowed referer domain must pass")

	// 4. Allowed wildcard subdomain match
	req = httptest.NewRequest("GET", "http://example.com/photo.jpg", nil)
	req.Header.Set("Referer", "https://sub.example.com/gallery")
	c.Request = req
	allowed = s.checkReferer(c, fileWithProtection)
	assert.True(t, allowed, "Allowed wildcard subdomain must pass")

	// 5. Disallowed domain
	req = httptest.NewRequest("GET", "http://example.com/photo.jpg", nil)
	req.Header.Set("Referer", "https://evil.attacker.com/steal")
	c.Request = req
	allowed = s.checkReferer(c, fileWithProtection)
	assert.False(t, allowed, "Disallowed referer domain must be rejected")
}
