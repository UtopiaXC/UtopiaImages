package http

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestImportArchiveUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode:    "test",
			Port:    2983,
			BaseURL: "http://localhost:2983",
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			DSN:    ":memory:",
		},
		Storage: config.StorageConfig{
			Driver: "local",
			Local: config.LocalStorage{
				BasePath: t.TempDir(),
			},
		},
		Cache: config.CacheConfig{
			Dir: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecretKey: "12345678901234567890123456789012",
		},
	}

	server, err := NewServer(cfg, "")
	assert.NoError(t, err)

	// Create user
	u := &model.User{
		ID:           1,
		Username:     "admin",
		Nickname:     "Admin",
		PasswordHash: "dummy",
		RoleID:       1,
		Status:       "active",
	}
	server.db.Create(u)

	// Create a dummy session for admin
	sm := auth.NewSessionManager(server.db)
	token, _, err := sm.CreateSession(1, "127.0.0.1", "test-agent", true)
	assert.NoError(t, err)

	// Create a dummy zip file
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("test.txt")
	assert.NoError(t, err)
	_, _ = f.Write([]byte("hello world"))
	_ = zw.Close()

	// Prepare multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("archive_file", "test.zip")
	assert.NoError(t, err)
	_, err = io.Copy(part, &zipBuf)
	assert.NoError(t, err)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/import/archive", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "uimg_session", Value: token})

	w := httptest.NewRecorder()
	server.Engine().ServeHTTP(w, req)

	t.Logf("Response Code: %d", w.Code)
	t.Logf("Response Body: %s", w.Body.String())
}
