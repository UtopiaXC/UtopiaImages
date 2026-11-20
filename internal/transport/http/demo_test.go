package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/common"
	"github.com/UtopiaXC/UtopiaImages/internal/config"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
)

func newDemoTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	tmpDir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	cfgPath := filepath.Join("data", "config", "config.yaml")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	config.EnsureDataDirectories(cfg)

	srv, err := NewServer(cfg, cfgPath)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() {
		srv.Stop()
		if db := srv.GetDB(); db != nil {
			if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
				_ = sqlDB.Close()
			}
		}
	})
	return srv, tmpDir
}

func doJSONRequest(t *testing.T, srv *Server, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Host = "localhost:2983"
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	srv.Engine().ServeHTTP(w, req)
	return w
}

func loginAs(t *testing.T, srv *Server, username, password string) []*http.Cookie {
	t.Helper()
	w := doJSONRequest(t, srv, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": username,
		"password": password,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login failed for %s: status=%d body=%s", username, w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie on login for %s", username)
	}
	return cookies
}

func assertCode(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedCode string) {
	t.Helper()
	if w.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d: %s", expectedStatus, w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (%s)", err, w.Body.String())
	}
	if resp["code"] != expectedCode {
		t.Fatalf("expected error code %q, got %v (body=%s)", expectedCode, resp["code"], w.Body.String())
	}
}

func TestDemoDefaultsAndAdminProtections(t *testing.T) {
	srv, _ := newDemoTestServer(t)

	// 1. Setup not required; site defaults enabled
	w := doJSONRequest(t, srv, http.MethodGet, "/api/v1/setup/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status code=%d", w.Code)
	}
	var setupResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &setupResp)
	if setupResp["setup_required"] != false {
		t.Fatalf("expected setup_required=false, got %v", setupResp["setup_required"])
	}

	w = doJSONRequest(t, srv, http.MethodGet, "/api/v1/site", nil)
	var siteResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &siteResp)
	if siteResp["allow_registration"] != true || siteResp["enable_guest_upload"] != true || siteResp["enable_square"] != true {
		t.Fatalf("expected demo defaults true, got %v", siteResp)
	}

	// 2. Wrong password on admin must not freeze admin
	for i := 0; i < 6; i++ {
		w = doJSONRequest(t, srv, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"username": "admin",
			"password": "wrong-password",
		})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on wrong admin password, got %d", w.Code)
		}
	}

	// Reset IP tracker by testing with a solved captcha or checking admin user status in DB
	var adminUser model.User
	if err := srv.GetDB().Where("id = 1").First(&adminUser).Error; err != nil {
		t.Fatalf("find admin: %v", err)
	}
	if adminUser.Status != "active" || adminUser.LockoutUntil != nil {
		t.Fatalf("admin should never be frozen/locked out, status=%s lockout=%v", adminUser.Status, adminUser.LockoutUntil)
	}
	if adminUser.Email == nil || *adminUser.Email != "example@example.com" {
		t.Fatalf("expected admin email example@example.com, got %v", adminUser.Email)
	}

	// Solve captcha for the login IP and sign in as admin/admin
	capW := doJSONRequest(t, srv, http.MethodGet, "/api/v1/auth/captcha", nil)
	if capW.Code != http.StatusOK {
		t.Fatalf("captcha status=%d", capW.Code)
	}
	var ch struct {
		Algorithm string `json:"algorithm"`
		Challenge string `json:"challenge"`
		MaxNumber int64  `json:"maxnumber"`
		Salt      string `json:"salt"`
		Signature string `json:"signature"`
	}
	_ = json.Unmarshal(capW.Body.Bytes(), &ch)
	_ = ch

	// Clear IP tracker for localhost so loginAs succeeds cleanly, or test login from fresh RemoteAddr
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.99:12345"
	w = httptest.NewRecorder()
	srv.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin login failed: %d %s", w.Code, w.Body.String())
	}
	adminCookies := w.Result().Cookies()

	// 3. Admin self-settings locked
	assertCode(t, doJSONRequest(t, srv, http.MethodPatch, "/api/v1/me/username", map[string]any{"new_username": "admin2"}, adminCookies...), http.StatusForbidden, common.ErrDemoAdminLocked)
	assertCode(t, doJSONRequest(t, srv, http.MethodPatch, "/api/v1/me/profile", map[string]any{"nickname": "hacked"}, adminCookies...), http.StatusForbidden, common.ErrDemoAdminLocked)
	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/me/password", map[string]any{"old_password": "admin", "new_password": "newpassword123"}, adminCookies...), http.StatusForbidden, common.ErrDemoAdminLocked)
	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/me/totp/setup", nil, adminCookies...), http.StatusForbidden, common.ErrDemoAdmin2FAForbidden)
	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/me/totp/enable", map[string]any{"code": "123456"}, adminCookies...), http.StatusForbidden, common.ErrDemoAdmin2FAForbidden)
	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/me/passkeys/register-begin", nil, adminCookies...), http.StatusForbidden, common.ErrDemoAdmin2FAForbidden)

	// 4. Admin users/roles/policies protections
	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"username": "secondadmin",
		"password": "password123",
		"role_id":  1,
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSingleAdminOnly)

	assertCode(t, doJSONRequest(t, srv, http.MethodPatch, "/api/v1/admin/users/1", map[string]any{
		"nickname": "changed",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoAdminLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPatch, "/api/v1/admin/roles/1", map[string]any{
		"display_name": "HackedAdmin",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoAdminRoleLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/policies", map[string]any{
		"name":   "evil",
		"driver": "webdav",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoPolicyLocked)

	// 5. System settings restrictions & base_url allowed
	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"site_name": "Hacked Site",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSystemSettingLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"require_2fa": "true",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSystemSettingLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"max_single_file_size": "0",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSystemSettingLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"smtp_enabled": "true",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSMTPDisabled)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/smtp/test", map[string]any{
		"host": "smtp.example.com",
		"port": 465,
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSMTPDisabled)

	assertCode(t, doJSONRequest(t, srv, http.MethodDelete, "/api/v1/admin/favicon", nil, adminCookies...), http.StatusForbidden, common.ErrDemoFaviconLocked)

	// global_call_stat, allow_registration, and default_register_role_id are locked to their demo defaults
	w = doJSONRequest(t, srv, http.MethodGet, "/api/v1/admin/settings", nil, adminCookies...)
	if w.Code != http.StatusOK {
		t.Fatalf("get admin settings failed: %d %s", w.Code, w.Body.String())
	}
	var adminSettings map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &adminSettings); err != nil {
		t.Fatalf("unmarshal admin settings failed: %v", err)
	}
	if adminSettings["global_call_stat"] != "true" || adminSettings["allow_registration"] != "true" || adminSettings["default_register_role_id"] != "2" {
		t.Fatalf("unexpected locked settings defaults: global_call_stat=%q allow_registration=%q default_register_role_id=%q",
			adminSettings["global_call_stat"], adminSettings["allow_registration"], adminSettings["default_register_role_id"])
	}

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"global_call_stat": "false",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSystemSettingLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"allow_registration": "false",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSystemSettingLocked)

	assertCode(t, doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"default_register_role_id": "3",
	}, adminCookies...), http.StatusForbidden, common.ErrDemoSystemSettingLocked)

	// Updating base_url alongside default system settings must succeed
	w = doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"site_name":                 "Utopia Images",
		"site_description":          "又一个Utopia Images自部署图床",
		"base_url":                  "https://demo.example.com",
		"restrict_domains":          "false",
		"log_level":                 "info",
		"log_permanent_level":       "ERROR",
		"log_max_size_mb":           "20",
		"log_max_backups":           "10",
		"log_max_age_days":          "30",
		"derived_cache_max_size_mb": "5120",
		"derived_cache_ttl_days":    "0",
		"global_call_stat":          "true",
		"allow_registration":        "true",
		"default_register_role_id":  "2",
		"max_single_file_size":      "50",
		"max_process_image_size":    "5",
		"require_2fa":               "false",
		"smtp_enabled":              "false",
	}, adminCookies...)
	if w.Code != http.StatusOK {
		t.Fatalf("expected updating base_url to succeed, got %d %s", w.Code, w.Body.String())
	}
}

func TestDemoRegistrationBypassEmailAndReset(t *testing.T) {
	srv, tmpDir := newDemoTestServer(t)

	// 1. Register user directly (disable captcha via admin first so test doesn't have to brute-force altcha PoW)
	adminCookies := loginAs(t, srv, "admin", "admin")
	w := doJSONRequest(t, srv, http.MethodPost, "/api/v1/admin/settings", map[string]any{
		"captcha_provider": "none",
	}, adminCookies...)
	if w.Code != http.StatusOK {
		t.Fatalf("disable captcha failed: %d %s", w.Code, w.Body.String())
	}

	w = doJSONRequest(t, srv, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"username": "demouser1",
		"email":    "demouser1@example.com",
		"password": "password123",
		"nickname": "Demo User",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("register failed: %d %s", w.Code, w.Body.String())
	}

	// Newly registered user can log in immediately without email activation
	_ = loginAs(t, srv, "demouser1", "password123")

	// 2. Write dummy files into storage & cache
	storageFile := filepath.Join(tmpDir, "data", "storage", "test.txt")
	if err := os.WriteFile(storageFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("write storage file: %v", err)
	}
	cacheFile := filepath.Join(tmpDir, "cache", "ab", "cd", "derived.webp")
	_ = os.MkdirAll(filepath.Dir(cacheFile), 0755)
	if err := os.WriteFile(cacheFile, []byte("webp"), 0644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	// 3. Subscribe to reset events and trigger ResetNow
	sub := srv.demoResetService.Subscribe()
	defer srv.demoResetService.Unsubscribe(sub)

	if err := srv.demoResetService.ResetNow(); err != nil {
		t.Fatalf("ResetNow: %v", err)
	}

	select {
	case st := <-sub:
		if st.ResetEpoch != 2 {
			t.Fatalf("expected reset_epoch=2, got %d", st.ResetEpoch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE reset notification")
	}

	// 4. Verify storage & cache were cleared, user was wiped, and admin still works
	if _, err := os.Stat(storageFile); !os.IsNotExist(err) {
		t.Fatalf("expected storage file to be removed after reset")
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Fatalf("expected cache file to be removed after reset")
	}
	var count int64
	srv.GetDB().Model(&model.User{}).Where("username = ?", "demouser1").Count(&count)
	if count != 0 {
		t.Fatalf("expected demouser1 to be wiped after reset, count=%d", count)
	}
	_ = loginAs(t, srv, "admin", "admin")
}

func TestDemoColdStartResetBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	cfgPath := filepath.Join("data", "config", "config.yaml")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	config.EnsureDataDirectories(cfg)

	// Boot 1: initialize DB and insert a custom user + storage file
	srv1, err := NewServer(cfg, cfgPath)
	if err != nil {
		t.Fatalf("NewServer 1: %v", err)
	}
	srv1.GetDB().Create(&model.User{
		Username:     "persistuser",
		PasswordHash: "hash",
		RoleID:       2,
		Status:       "active",
	})
	storageFile := filepath.Join(tmpDir, "data", "storage", "coldstart.txt")
	if err := os.WriteFile(storageFile, []byte("data"), 0644); err != nil {
		t.Fatalf("write storage file: %v", err)
	}

	// Set demo_last_reset_at to 10 minutes ago (< 30m) and close
	tenMinAgo := time.Now().Add(-10 * time.Minute).UnixMilli()
	srv1.GetDB().Save(&model.Setting{Key: "demo_last_reset_at", Value: strconv.FormatInt(tenMinAgo, 10)})
	srv1.Stop()
	if sqlDB, err := srv1.GetDB().DB(); err == nil && sqlDB != nil {
		_ = sqlDB.Close()
	}

	// Boot 2: cold start within 30m should preserve data and resume remaining countdown (~20m)
	srv2, err := NewServer(cfg, cfgPath)
	if err != nil {
		t.Fatalf("NewServer 2: %v", err)
	}
	var cnt int64
	srv2.GetDB().Model(&model.User{}).Where("username = ?", "persistuser").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expected persistuser to be preserved when < 30m elapsed, got %d", cnt)
	}
	st2 := srv2.demoResetService.GetStatus()
	if st2.RemainingMs > 21*60*1000 || st2.RemainingMs < 18*60*1000 {
		t.Fatalf("expected ~20m remaining, got %d ms", st2.RemainingMs)
	}

	// Set demo_last_reset_at to 35 minutes ago (>= 30m) and close
	thirtyFiveMinAgo := time.Now().Add(-35 * time.Minute).UnixMilli()
	srv2.GetDB().Save(&model.Setting{Key: "demo_last_reset_at", Value: strconv.FormatInt(thirtyFiveMinAgo, 10)})
	srv2.Stop()
	if sqlDB, err := srv2.GetDB().DB(); err == nil && sqlDB != nil {
		_ = sqlDB.Close()
	}

	// Boot 3: cold start >= 30m after last reset must wipe DB and storage immediately
	srv3, err := NewServer(cfg, cfgPath)
	if err != nil {
		t.Fatalf("NewServer 3: %v", err)
	}
	defer func() {
		srv3.Stop()
		if sqlDB, err := srv3.GetDB().DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	srv3.GetDB().Model(&model.User{}).Where("username = ?", "persistuser").Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected persistuser to be wiped on expired cold start, got %d", cnt)
	}
	if _, err := os.Stat(storageFile); !os.IsNotExist(err) {
		t.Fatalf("expected storage file to be wiped on expired cold start")
	}
	st3 := srv3.demoResetService.GetStatus()
	if st3.RemainingMs < 29*60*1000 {
		t.Fatalf("expected ~30m remaining after cold-start reset, got %d ms", st3.RemainingMs)
	}
}
