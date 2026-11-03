package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDispatchServiceGetFileStream(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_ = db.AutoMigrate(&model.FileRecord{}, &model.FilePolicyReplica{}, &model.StoragePolicy{})

	tmpDir, err := os.MkdirTemp("", "uimg-dispatch-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFilePath := filepath.Join(tmpDir, "sample.jpg")
	if err := os.WriteFile(testFilePath, []byte("fake-image-data"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	localStorage, err := storage.NewLocalStorageDriver(tmpDir)
	if err != nil {
		t.Fatalf("failed to init local storage: %v", err)
	}

	policy := model.StoragePolicy{
		Name:   "Local Policy",
		Driver: "local",
		Config: `{"base_path":"` + tmpDir + `"}`,
	}
	db.Create(&policy)

	file := model.FileRecord{
		UserID:      1,
		PolicyID:    policy.ID,
		PublicPath:  "sample.jpg",
		StorageKey:  "sample.jpg",
		DisplayName: "sample.jpg",
		MimeType:    "image/jpeg",
		Size:        15,
		IsImage:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.Create(&file)

	replica := model.FilePolicyReplica{
		FileID:     file.ID,
		PolicyID:   policy.ID,
		PublicPath: "sample.jpg",
		StorageKey: "sample.jpg",
		Size:       15,
		Status:     "ready",
	}
	db.Create(&replica)

	driverMgr := storage.NewPolicyDriverManager(db)
	svc := NewDispatchService(db, localStorage, nil, nil)
	svc.SetDriverManager(driverMgr)

	// Test getFileStream with preferredReplica
	src, rc, err := svc.getFileStream(context.Background(), &file, &replica)
	if err != nil {
		t.Fatalf("getFileStream failed: %v", err)
	}
	if rc == nil {
		t.Fatalf("expected non-nil rc from getFileStream")
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("failed to read rc: %v", err)
	}
	if string(data) != "fake-image-data" {
		t.Fatalf("unexpected content: %s", string(data))
	}
	if !src.isLocal {
		t.Fatalf("expected src.isLocal to be true")
	}
}
