package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}

	err = db.AutoMigrate(
		&model.Role{},
		&model.User{},
		&model.StoragePolicy{},
		&model.RolePolicy{},
		&model.FileRecord{},
		&model.FilePolicyReplica{},
		&model.FileBlob{},
		&model.WatermarkTemplate{},
		&model.Setting{},
	)
	if err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}
	return db
}

func createTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestMultiPolicyUploadAndSync(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	driverMgr := storage.NewPolicyDriverManager(db)

	pol1 := model.StoragePolicy{
		Name:       "DB Policy 1",
		Driver:     "database",
		PathPrefix: "db1",
		Config:     "{}",
	}
	pol2 := model.StoragePolicy{
		Name:       "DB Policy 2",
		Driver:     "database",
		PathPrefix: "db2",
		Config:     "{}",
	}
	pol3 := model.StoragePolicy{
		Name:       "DB Policy 3",
		Driver:     "database",
		PathPrefix: "db3",
		Config:     "{}",
	}
	if err := db.Create(&pol1).Error; err != nil {
		t.Fatalf("create pol1 failed: %v", err)
	}
	if err := db.Create(&pol2).Error; err != nil {
		t.Fatalf("create pol2 failed: %v", err)
	}
	if err := db.Create(&pol3).Error; err != nil {
		t.Fatalf("create pol3 failed: %v", err)
	}

	role := model.Role{
		Name:        "test-role",
		DisplayName: "Test Role",
		QuotaBytes:  1024 * 1024 * 100,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role failed: %v", err)
	}

	db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: pol1.ID})
	db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: pol2.ID})
	db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: pol3.ID})

	user := model.User{
		Username:  "tester",
		RoleID:    role.ID,
		Role:      &role,
		UsedBytes: 0,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	uploadSvc := NewUploadService(db, nil, nil, nil)
	uploadSvc.SetDriverManager(driverMgr)

	pngData := createTestPNG()

	// 1. Upload to 2 policies
	opts := UploadOptions{
		PolicyIDs:     []uint{pol1.ID, pol2.ID},
		WatermarkMode: "off",
		PublicMode:    "private",
	}

	file, err := uploadSvc.Upload(ctx, &user, bytes.NewReader(pngData), "test.png", int64(len(pngData)), opts)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if file == nil {
		t.Fatalf("expected file to be created")
	}

	// Verify replicas
	var replicas []model.FilePolicyReplica
	if err := db.Where("file_id = ?", file.ID).Find(&replicas).Error; err != nil {
		t.Fatalf("query replicas failed: %v", err)
	}
	if len(replicas) != 2 {
		t.Fatalf("expected 2 replicas, got %d", len(replicas))
	}
	for _, r := range replicas {
		if r.Status != "ready" {
			t.Errorf("replica %d status is not ready: %s (%s)", r.ID, r.Status, r.ErrorMsg)
		}
	}

	// Verify user quota = file.Size * 2
	var updatedUser model.User
	if err := db.First(&updatedUser, user.ID).Error; err != nil {
		t.Fatalf("query user failed: %v", err)
	}
	expectedQuota := file.Size * 2
	if updatedUser.UsedBytes != expectedQuota {
		t.Errorf("expected user UsedBytes %d, got %d", expectedQuota, updatedUser.UsedBytes)
	}

	// 2. Sync to Policy 3
	syncedReplicas, err := uploadSvc.SyncFileToPolicies(ctx, &user, file.ID, []uint{pol3.ID}, false)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(syncedReplicas) != 1 {
		t.Fatalf("expected 1 newly added replica after sync, got %d", len(syncedReplicas))
	}

	// Check total replicas in DB
	if err := db.Where("file_id = ?", file.ID).Find(&replicas).Error; err != nil {
		t.Fatalf("query replicas failed: %v", err)
	}
	if len(replicas) != 3 {
		t.Fatalf("expected 3 total replicas in DB, got %d", len(replicas))
	}

	// Verify user quota = file.Size * 3
	if err := db.First(&updatedUser, user.ID).Error; err != nil {
		t.Fatalf("query user failed: %v", err)
	}
	expectedQuota = file.Size * 3
	if updatedUser.UsedBytes != expectedQuota {
		t.Errorf("expected user UsedBytes %d after sync, got %d", expectedQuota, updatedUser.UsedBytes)
	}

	// 3. Remove replica from Policy 1
	err = uploadSvc.RemoveFileFromPolicy(ctx, &user, file.ID, pol1.ID, false)
	if err != nil {
		t.Fatalf("remove replica failed: %v", err)
	}

	// Verify user quota = file.Size * 2
	if err := db.First(&updatedUser, user.ID).Error; err != nil {
		t.Fatalf("query user failed: %v", err)
	}
	expectedQuota = file.Size * 2
	if updatedUser.UsedBytes != expectedQuota {
		t.Errorf("expected user UsedBytes %d after removing 1 replica, got %d", expectedQuota, updatedUser.UsedBytes)
	}

	// 4. Remove second replica
	err = uploadSvc.RemoveFileFromPolicy(ctx, &user, file.ID, pol2.ID, false)
	if err != nil {
		t.Fatalf("remove replica 2 failed: %v", err)
	}

	// 5. Attempting to remove the last replica must be rejected (Requirement 5)
	err = uploadSvc.RemoveFileFromPolicy(ctx, &user, file.ID, pol3.ID, false)
	if err == nil {
		t.Fatalf("expected error when removing last remaining replica, got nil")
	}

	// Verify file record still exists
	var checkFile model.FileRecord
	if err := db.First(&checkFile, file.ID).Error; err != nil {
		t.Errorf("expected file record to still exist, got %v", err)
	}

	// 6. Explicit file deletion destroys the file and frees all quota
	err = uploadSvc.DeleteFile(ctx, file.ID, user.ID, false)
	if err != nil {
		t.Fatalf("delete file failed: %v", err)
	}

	// Verify file record is gone
	if err := db.First(&checkFile, file.ID).Error; err != gorm.ErrRecordNotFound {
		t.Errorf("expected ErrRecordNotFound after DeleteFile, got %v", err)
	}

	// Verify user quota is 0
	if err := db.First(&updatedUser, user.ID).Error; err != nil {
		t.Fatalf("query user failed: %v", err)
	}
	if updatedUser.UsedBytes != 0 {
		t.Errorf("expected user UsedBytes 0 after full deletion, got %d", updatedUser.UsedBytes)
	}
}

func TestStoragePolicySizeLimitAndPartialSuccess(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	driverMgr := storage.NewPolicyDriverManager(db)

	// Policy A: Max 1MB
	polA := model.StoragePolicy{
		Name:                "Small Policy",
		Driver:              "database",
		PathPrefix:          "small",
		MaxSingleFileSizeMB: 1, // 1MB limit
		Config:              "{}",
	}
	// Policy B: Unlimited
	polB := model.StoragePolicy{
		Name:                "Large Policy",
		Driver:              "database",
		PathPrefix:          "large",
		MaxSingleFileSizeMB: 0, // unlimited
		Config:              "{}",
	}
	if err := db.Create(&polA).Error; err != nil {
		t.Fatalf("create polA failed: %v", err)
	}
	if err := db.Create(&polB).Error; err != nil {
		t.Fatalf("create polB failed: %v", err)
	}

	role := model.Role{
		Name:        "user-role",
		DisplayName: "User Role",
		QuotaBytes:  1024 * 1024 * 100,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: polA.ID})
	db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: polB.ID})

	user := model.User{
		Username:  "tester2",
		RoleID:    role.ID,
		Role:      &role,
		UsedBytes: 0,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	uploadSvc := NewUploadService(db, nil, nil, nil)
	uploadSvc.SetDriverManager(driverMgr)

	// Create 1.5MB of data (> 1MB)
	largeData := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47}, 400*1024) // ~1.6MB

	// Upload specifying only Policy A -> should fail completely
	optsA := UploadOptions{
		PolicyIDs:     []uint{polA.ID},
		WatermarkMode: "off",
		PublicMode:    "private",
	}
	_, err := uploadSvc.Upload(ctx, &user, bytes.NewReader(largeData), "large.png", int64(len(largeData)), optsA)
	if err == nil {
		t.Fatalf("expected upload to fail for polA due to size limit")
	}

	// Upload specifying both Policy A and Policy B -> should succeed partially (only Policy B)
	optsBoth := UploadOptions{
		PolicyIDs:     []uint{polA.ID, polB.ID},
		WatermarkMode: "off",
		PublicMode:    "private",
	}
	file, err := uploadSvc.Upload(ctx, &user, bytes.NewReader(largeData), "large.png", int64(len(largeData)), optsBoth)
	if err != nil {
		t.Fatalf("upload to both should partially succeed, got error: %v", err)
	}
	if file == nil {
		t.Fatalf("expected file to be created")
	}

	var replicas []model.FilePolicyReplica
	db.Where("file_id = ?", file.ID).Find(&replicas)
	if len(replicas) != 1 {
		t.Fatalf("expected 1 replica in PolB, got %d", len(replicas))
	}
	if replicas[0].PolicyID != polB.ID {
		t.Errorf("expected replica to be in PolB (%d), got %d", polB.ID, replicas[0].PolicyID)
	}

	// Quota should only be 1 * file.Size
	var updatedUser model.User
	db.First(&updatedUser, user.ID)
	if updatedUser.UsedBytes != int64(len(largeData)) {
		t.Errorf("expected UsedBytes %d, got %d", len(largeData), updatedUser.UsedBytes)
	}
}
