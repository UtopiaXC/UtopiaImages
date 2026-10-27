package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
)

func TestChunkService_FlowAndSHA256(t *testing.T) {
	tempCache, err := os.MkdirTemp("", "chunk-test-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempCache)

	svc := NewChunkService(nil, tempCache, nil, nil)
	defer svc.Stop()

	user := &model.User{ID: 1, RoleID: 1}

	// 1. Prepare data
	chunk0 := []byte("chunk-zero-data-")
	chunk1 := []byte("chunk-one-data--")
	fullData := append(chunk0, chunk1...)
	hasher := sha256.New()
	hasher.Write(fullData)
	fullSHA := hex.EncodeToString(hasher.Sum(nil))

	// 2. Init
	meta, uploaded, err := svc.InitChunkUpload(user, ChunkUploadMeta{
		Filename:    "test.txt",
		TotalSize:   int64(len(fullData)),
		ChunkSize:   int64(len(chunk0)),
		TotalChunks: 2,
		FileSHA256:  fullSHA,
		UploadType:  "file",
	})
	if err != nil {
		t.Fatalf("InitChunkUpload failed: %v", err)
	}
	if len(uploaded) != 0 {
		t.Fatalf("Expected 0 uploaded chunks, got %d", len(uploaded))
	}

	// 3. Save chunks
	if err := svc.SaveChunkPart(user, meta.UploadID, 0, bytes.NewReader(chunk0)); err != nil {
		t.Fatalf("SaveChunkPart 0 failed: %v", err)
	}

	// Check status
	_, statusUploaded, err := svc.GetChunkStatus(user, meta.UploadID)
	if err != nil || len(statusUploaded) != 1 || statusUploaded[0] != 0 {
		t.Fatalf("Expected [0] uploaded, got %v (err: %v)", statusUploaded, err)
	}

	if err := svc.SaveChunkPart(user, meta.UploadID, 1, bytes.NewReader(chunk1)); err != nil {
		t.Fatalf("SaveChunkPart 1 failed: %v", err)
	}

	// 4. Test Abort cleans up cache
	abortMeta, _, err := svc.InitChunkUpload(user, ChunkUploadMeta{
		Filename:    "abort.txt",
		TotalSize:   10,
		TotalChunks: 1,
		UploadType:  "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	abortDir := svc.getUploadDir(abortMeta.UploadID)
	if _, err := os.Stat(abortDir); os.IsNotExist(err) {
		t.Fatalf("Expected dir %s to exist", abortDir)
	}
	if err := svc.AbortChunkUpload(user, abortMeta.UploadID); err != nil {
		t.Fatalf("AbortChunkUpload failed: %v", err)
	}
	if _, err := os.Stat(abortDir); !os.IsNotExist(err) {
		t.Fatalf("Expected dir %s to be deleted after abort", abortDir)
	}

	// 5. Test SHA256 mismatch
	badMeta, _, _ := svc.InitChunkUpload(user, ChunkUploadMeta{
		Filename:    "bad.txt",
		TotalSize:   int64(len(fullData)),
		ChunkSize:   int64(len(chunk0)),
		TotalChunks: 2,
		FileSHA256:  "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		UploadType:  "file",
	})
	_ = svc.SaveChunkPart(user, badMeta.UploadID, 0, bytes.NewReader(chunk0))
	_ = svc.SaveChunkPart(user, badMeta.UploadID, 1, bytes.NewReader(chunk1))
	_, _, err = svc.CompleteChunkUpload(context.Background(), user, badMeta.UploadID)
	if err == nil {
		t.Fatalf("Expected SHA256 mismatch error, got nil")
	}

	// Verify bad upload cache cleaned up on mismatch
	badDir := svc.getUploadDir(badMeta.UploadID)
	if _, err := os.Stat(badDir); !os.IsNotExist(err) {
		t.Fatalf("Expected badDir %s to be cleaned up after SHA mismatch", badDir)
	}
}
