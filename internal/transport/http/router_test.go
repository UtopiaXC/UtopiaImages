package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/web"
)

func TestAssetsRoute(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode:    "test",
			Port:    8080,
			BaseURL: "http://localhost:8080",
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

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("failed to init server: %v", err)
	}

	html, err := web.ReadIndexHTML()
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	// Match src="/assets/index-*.js"
	content := string(html)
	idx := strings.Index(content, "/assets/index-")
	if idx == -1 {
		t.Fatalf("asset path not found in index.html: %s", content)
	}
	endIdx := strings.Index(content[idx:], `"`)
	assetPath := content[idx : idx+endIdx]

	req := httptest.NewRequest("GET", assetPath, nil)
	w := httptest.NewRecorder()
	server.Engine().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 for js (%s), got %d", assetPath, w.Code)
	}

	t.Logf("Asset %s served successfully with 200 OK (%d bytes)", assetPath, len(w.Body.Bytes()))
}
