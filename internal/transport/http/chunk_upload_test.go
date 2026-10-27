package http

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func TestChunkUploadAPI_ArchiveFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cacheDir := t.TempDir()
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
			Dir: cacheDir,
		},
		Security: config.SecurityConfig{
			SecretKey: "12345678901234567890123456789012",
		},
	}

	server, err := NewServer(cfg, "")
	assert.NoError(t, err)

	u := &model.User{
		ID:           1,
		Username:     "admin",
		Nickname:     "Admin",
		PasswordHash: "dummy",
		RoleID:       1,
		Status:       "active",
	}
	server.db.Create(u)

	// Create session token
	sm := auth.NewSessionManager(server.db)
	token, _, err := sm.CreateSession(1, "127.0.0.1", "test-agent", true)
	assert.NoError(t, err)

	// Build a small zip archive
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	zf, err := zw.Create("hello.txt")
	assert.NoError(t, err)
	_, _ = zf.Write([]byte("hello chunk world"))
	assert.NoError(t, zw.Close())

	zipBytes := zipBuf.Bytes()
	hasher := sha256.New()
	hasher.Write(zipBytes)
	fileSHA := hex.EncodeToString(hasher.Sum(nil))

	// Split into 2 chunks
	mid := len(zipBytes) / 2
	part0 := zipBytes[:mid]
	part1 := zipBytes[mid:]

	// 1. Init Chunk Upload
	initReqBody := map[string]any{
		"filename":        "backup.zip",
		"total_size":      len(zipBytes),
		"chunk_size":      len(part0),
		"total_chunks":    2,
		"file_sha256":     fileSHA,
		"upload_type":     "archive",
		"conflict_action": "skip",
	}
	initJSON, _ := json.Marshal(initReqBody)

	req := httptest.NewRequest("POST", "/api/v1/upload/chunk/init", bytes.NewReader(initJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var initResp struct {
		UploadID    string `json:"upload_id"`
		TotalChunks int    `json:"total_chunks"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &initResp))
	assert.NotEmpty(t, initResp.UploadID)
	assert.Equal(t, 2, initResp.TotalChunks)

	uploadID := initResp.UploadID

	// 2. Upload chunk 0
	sendChunk := func(idx int, data []byte) int {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("upload_id", uploadID)
		_ = mw.WriteField("chunk_index", fmt.Sprintf("%d", idx))
		part, _ := mw.CreateFormFile("file", fmt.Sprintf("part-%d", idx))
		_, _ = part.Write(data)
		_ = mw.Close()

		cReq := httptest.NewRequest("POST", "/api/v1/upload/chunk/part", &body)
		cReq.Header.Set("Content-Type", mw.FormDataContentType())
		cReq.Header.Set("Authorization", "Bearer "+token)
		cRec := httptest.NewRecorder()
		server.engine.ServeHTTP(cRec, cReq)
		return cRec.Code
	}

	assert.Equal(t, http.StatusOK, sendChunk(0, part0))

	// 3. Status check
	sReq := httptest.NewRequest("GET", "/api/v1/upload/chunk/status?upload_id="+uploadID, nil)
	sReq.Header.Set("Authorization", "Bearer "+token)
	sRec := httptest.NewRecorder()
	server.engine.ServeHTTP(sRec, sReq)
	assert.Equal(t, http.StatusOK, sRec.Code)
	var statusResp struct {
		UploadedChunks []int `json:"uploaded_chunks"`
	}
	assert.NoError(t, json.Unmarshal(sRec.Body.Bytes(), &statusResp))
	assert.Equal(t, []int{0}, statusResp.UploadedChunks)

	// 4. Upload chunk 1
	assert.Equal(t, http.StatusOK, sendChunk(1, part1))

	// 5. Complete
	compReqBody := map[string]any{"upload_id": uploadID}
	compJSON, _ := json.Marshal(compReqBody)
	mReq := httptest.NewRequest("POST", "/api/v1/upload/chunk/complete", bytes.NewReader(compJSON))
	mReq.Header.Set("Content-Type", "application/json")
	mReq.Header.Set("Authorization", "Bearer "+token)
	mRec := httptest.NewRecorder()
	server.engine.ServeHTTP(mRec, mReq)

	assert.Equal(t, http.StatusOK, mRec.Code)
	var compResp struct {
		Total    int `json:"total"`
		Imported int `json:"imported"`
	}
	assert.NoError(t, json.Unmarshal(mRec.Body.Bytes(), &compResp))
	assert.Equal(t, 1, compResp.Total)
	assert.Equal(t, 1, compResp.Imported)

	// 6. Test Abort endpoint
	aInitReqBody := map[string]any{
		"filename":     "abort.zip",
		"total_size":   100,
		"chunk_size":   50,
		"total_chunks": 2,
		"upload_type":  "archive",
	}
	aInitJSON, _ := json.Marshal(aInitReqBody)
	aiReq := httptest.NewRequest("POST", "/api/v1/upload/chunk/init", bytes.NewReader(aInitJSON))
	aiReq.Header.Set("Content-Type", "application/json")
	aiReq.Header.Set("Authorization", "Bearer "+token)
	aiRec := httptest.NewRecorder()
	server.engine.ServeHTTP(aiRec, aiReq)
	assert.Equal(t, http.StatusOK, aiRec.Code)
	var aInitResp struct {
		UploadID string `json:"upload_id"`
	}
	assert.NoError(t, json.Unmarshal(aiRec.Body.Bytes(), &aInitResp))

	abortReqBody := map[string]any{"upload_id": aInitResp.UploadID}
	abortJSON, _ := json.Marshal(abortReqBody)
	abReq := httptest.NewRequest("POST", "/api/v1/upload/chunk/abort", bytes.NewReader(abortJSON))
	abReq.Header.Set("Content-Type", "application/json")
	abReq.Header.Set("Authorization", "Bearer "+token)
	abRec := httptest.NewRecorder()
	server.engine.ServeHTTP(abRec, abReq)
	assert.Equal(t, http.StatusOK, abRec.Code)
}
