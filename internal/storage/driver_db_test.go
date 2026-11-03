package storage

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in memory: %v", err)
	}
	if err := db.AutoMigrate(&model.FileBlob{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}
	return db
}

func TestDatabaseStorageDriver(t *testing.T) {
	db := setupTestDB(t)
	driver := NewDatabaseStorageDriver(db)
	ctx := context.Background()

	// 1. Test connection
	if err := driver.Test(ctx); err != nil {
		t.Fatalf("driver test failed: %v", err)
	}

	// 2. Put payload larger than chunk size (e.g. 5MB)
	dataSize := 5 * 1024 * 1024 // 5MB
	testPayload := make([]byte, dataSize)
	for i := range testPayload {
		testPayload[i] = byte(i % 256)
	}

	key := "test/sample_image.png"
	if err := driver.Put(ctx, key, bytes.NewReader(testPayload), int64(dataSize)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 3. Stat
	info, err := driver.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != int64(dataSize) {
		t.Fatalf("expected size %d, got %d", dataSize, info.Size)
	}

	// 4. Get and verify
	rc, err := driver.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer rc.Close()

	readData, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(readData) != dataSize {
		t.Fatalf("expected len %d, got %d", dataSize, len(readData))
	}
	if !bytes.Equal(readData, testPayload) {
		t.Fatalf("read data does not match original payload")
	}

	// 5. Delete
	if err := driver.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify not found after delete
	_, err = driver.Get(ctx, key)
	if err == nil {
		t.Fatalf("expected error after delete, got nil")
	}
}
