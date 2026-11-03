package storage

import (
	"context"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStorageRegistryAndDrivers(t *testing.T) {
	drivers := SupportedDrivers()
	expectedDrivers := []string{
		"local",
		"database",
		"webdav",
		"s3_compatible",
		"s3",
		"minio",
		"oss",
		"cos",
		"obs",
		"gcs",
		"azure_blob",
		"github",
		"onedrive",
	}

	driverMap := make(map[string]bool)
	for _, d := range drivers {
		driverMap[d] = true
	}

	for _, expected := range expectedDrivers {
		if !driverMap[expected] {
			t.Errorf("expected driver %q to be registered, but not found", expected)
		}
	}
}

func TestPolicyDriverManager(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in memory: %v", err)
	}
	if err := db.AutoMigrate(&model.StoragePolicy{}, &model.FileBlob{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	// Insert test policies
	pLocal := model.StoragePolicy{
		Name:       "Local Policy",
		Driver:     "local",
		Config:     `{"base_path":"data/test_storage"}`,
		PathPrefix: "local",
		IsDefault:  true,
	}
	pDB := model.StoragePolicy{
		Name:       "DB Policy",
		Driver:     "database",
		Config:     `{}`,
		PathPrefix: "db",
	}
	db.Create(&pLocal)
	db.Create(&pDB)

	dm := NewPolicyDriverManager(db)
	ctx := context.Background()

	// 1. Get local driver
	d1, pol1, err := dm.GetDriver(ctx, pLocal.ID)
	if err != nil {
		t.Fatalf("failed to get local driver: %v", err)
	}
	if pol1.ID != pLocal.ID {
		t.Fatalf("expected policy ID %d, got %d", pLocal.ID, pol1.ID)
	}

	// Verify caching returns same instance
	d2, _, err := dm.GetDriver(ctx, pLocal.ID)
	if err != nil {
		t.Fatalf("failed to get cached driver: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("expected cached driver pointer to match")
	}

	// 2. Get database driver
	dDB, polDB, err := dm.GetDriver(ctx, pDB.ID)
	if err != nil {
		t.Fatalf("failed to get db driver: %v", err)
	}
	if polDB.ID != pDB.ID {
		t.Fatalf("expected policy ID %d, got %d", pDB.ID, polDB.ID)
	}
	if err := dDB.Test(ctx); err != nil {
		t.Fatalf("database driver test failed: %v", err)
	}

	// 3. Invalidate
	dm.Invalidate(pLocal.ID)
	d3, _, err := dm.GetDriver(ctx, pLocal.ID)
	if err != nil {
		t.Fatalf("failed to get recreated driver: %v", err)
	}
	if d3 == d1 {
		t.Fatalf("expected new driver instance after invalidation")
	}
}
