package http

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/common"
	"github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var serverStartTime = time.Now()

type AdminController struct {
	db            *gorm.DB
	uploadService *service.UploadService
	baseURL       string
	derivedCache  *cache.DerivedCache
}

func NewAdminController(db *gorm.DB, us *service.UploadService, baseURL string, dc *cache.DerivedCache) *AdminController {
	return &AdminController{
		db:            db,
		uploadService: us,
		baseURL:       baseURL,
		derivedCache:  dc,
	}
}

func (ctrl *AdminController) SetDB(db *gorm.DB) {
	ctrl.db = db
}

func (ctrl *AdminController) SetBaseURL(baseURL string) {
	ctrl.baseURL = baseURL
}

func (ctrl *AdminController) resolveBaseURL(c *gin.Context) string {
	return ResolveBaseURL(c, ctrl.db, ctrl.baseURL)
}

func (ctrl *AdminController) RegisterRoutes(rg *gin.RouterGroup) {
	admin := rg.Group("/admin")
	admin.Use(auth.RequireAuth(), auth.RequireAdmin())
	{
		admin.GET("/stats", ctrl.GetStats)

		// All-site Files management
		admin.GET("/files", ctrl.ListAllFiles)
		admin.GET("/files/:id/details", ctrl.GetAllFileDetails)
		admin.DELETE("/files/:id", ctrl.DeleteSiteFile)
		admin.POST("/files/batch-delete", ctrl.BatchDeleteSiteFiles)

		// Users management
		admin.GET("/users", ctrl.ListUsers)
		admin.POST("/users", ctrl.CreateUser)
		admin.PATCH("/users/:id", ctrl.UpdateUser)
		admin.DELETE("/users/:id", ctrl.DeleteUser)
		admin.POST("/users/:id/ban", ctrl.BanUser)
		admin.POST("/users/:id/unfreeze", ctrl.UnfreezeUser)

		// Roles management (Requirement 8)
		admin.GET("/roles", ctrl.ListRoles)
		admin.POST("/roles", ctrl.CreateRole)
		admin.PATCH("/roles/:id", ctrl.UpdateRole)
		admin.DELETE("/roles/:id", ctrl.DeleteRole)

		// Storage policies management (Requirement 8)
		admin.GET("/policies", ctrl.ListPolicies)
		admin.POST("/policies", ctrl.CreatePolicy)
		admin.PATCH("/policies/:id", ctrl.UpdatePolicy)
		admin.DELETE("/policies/:id", ctrl.DeletePolicy)

		// Image Presets
		admin.GET("/presets", ctrl.ListPresets)
		admin.POST("/presets", ctrl.CreatePreset)
		admin.DELETE("/presets/:id", ctrl.DeletePreset)

		// Database status
		admin.GET("/database/status", ctrl.GetDatabaseStatus)
	}
}

func (ctrl *AdminController) GetStats(c *gin.Context) {
	var userCount, fileCount, roleCount, policyCount int64
	var totalBytes int64

	ctrl.db.Model(&model.User{}).Count(&userCount)
	ctrl.db.Model(&model.FileRecord{}).Count(&fileCount)
	ctrl.db.Model(&model.Role{}).Count(&roleCount)
	ctrl.db.Model(&model.StoragePolicy{}).Count(&policyCount)

	type Result struct {
		Total int64
	}
	var res Result
	ctrl.db.Model(&model.FileRecord{}).Select("COALESCE(SUM(size), 0) as total").Scan(&res)
	totalBytes = res.Total

	// 14 days activity trend
	type DayStat struct {
		Date       string `json:"date"`
		FilesCount int64  `json:"files_count"`
		BytesCount int64  `json:"bytes_count"`
	}
	daysMap := make(map[string]*DayStat)
	var daysList []DayStat
	now := time.Now()
	for i := 13; i >= 0; i-- {
		dStr := now.AddDate(0, 0, -i).Format("2006-01-02")
		ds := &DayStat{Date: dStr, FilesCount: 0, BytesCount: 0}
		daysMap[dStr] = ds
	}

	type FileSimple struct {
		CreatedAt time.Time
		Size      int64
	}
	var recentRecords []FileSimple
	ctrl.db.Model(&model.FileRecord{}).
		Where("created_at >= ?", now.AddDate(0, 0, -14).Truncate(24*time.Hour)).
		Select("created_at, size").
		Find(&recentRecords)

	for _, rec := range recentRecords {
		dStr := rec.CreatedAt.Format("2006-01-02")
		if stat, ok := daysMap[dStr]; ok {
			stat.FilesCount++
			stat.BytesCount += rec.Size
		}
	}

	for i := 13; i >= 0; i-- {
		dStr := now.AddDate(0, 0, -i).Format("2006-01-02")
		daysList = append(daysList, *daysMap[dStr])
	}

	// Mime type distribution
	type MimeStat struct {
		Mime      string `json:"mime"`
		Count     int64  `json:"count"`
		TotalSize int64  `json:"total_size"`
	}
	var mimeStats []MimeStat
	ctrl.db.Model(&model.FileRecord{}).
		Select("mime_type as mime, count(*) as count, COALESCE(sum(size), 0) as total_size").
		Group("mime_type").
		Order("count DESC").
		Limit(10).
		Scan(&mimeStats)

	// Recent 10 uploaded files with user info
	var recentFiles []model.FileRecord
	ctrl.db.Order("id DESC").Limit(10).Find(&recentFiles)

	userIDs := make([]uint, 0)
	policyIDs := make([]uint, 0)
	for _, f := range recentFiles {
		userIDs = append(userIDs, f.UserID)
		policyIDs = append(policyIDs, f.PolicyID)
	}

	var users []model.User
	if len(userIDs) > 0 {
		ctrl.db.Where("id IN ?", userIDs).Find(&users)
	}
	userMap := make(map[uint]string)
	for _, u := range users {
		if u.Nickname != "" {
			userMap[u.ID] = u.Nickname
		} else {
			userMap[u.ID] = u.Username
		}
	}

	var policies []model.StoragePolicy
	if len(policyIDs) > 0 {
		ctrl.db.Where("id IN ?", policyIDs).Find(&policies)
	}
	policyMap := make(map[uint]string)
	for _, p := range policies {
		policyMap[p.ID] = p.Name
	}

	type RecentFileDTO struct {
		ID            uint      `json:"id"`
		DisplayName   string    `json:"display_name"`
		PublicPath    string    `json:"public_path"`
		Size          int64     `json:"size"`
		MimeType      string    `json:"mime_type"`
		StoragePolicy string    `json:"storage_policy"`
		Uploader      string    `json:"uploader"`
		CreatedAt     time.Time `json:"created_at"`
	}
	recentFilesDTO := make([]RecentFileDTO, 0, len(recentFiles))
	for _, f := range recentFiles {
		policyName := "默认策略"
		if name, ok := policyMap[f.PolicyID]; ok && name != "" {
			policyName = name
		}
		uploaderName := "系统"
		if name, ok := userMap[f.UserID]; ok && name != "" {
			uploaderName = name
		}
		recentFilesDTO = append(recentFilesDTO, RecentFileDTO{
			ID:            f.ID,
			DisplayName:   f.DisplayName,
			PublicPath:    f.PublicPath,
			Size:          f.Size,
			MimeType:      f.MimeType,
			StoragePolicy: policyName,
			Uploader:      uploaderName,
			CreatedAt:     f.CreatedAt,
		})
	}

	// Recent 10 users
	var recentUsers []model.User
	ctrl.db.Preload("Role").Order("id DESC").Limit(10).Find(&recentUsers)

	// System Runtime Info
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	sysInfo := gin.H{
		"go_version":            runtime.Version(),
		"os":                    runtime.GOOS,
		"arch":                  runtime.GOARCH,
		"goroutines":            runtime.NumGoroutine(),
		"alloc_bytes":           mem.Alloc,
		"total_alloc_bytes":     mem.TotalAlloc,
		"sys_bytes":             mem.Sys,
		"num_gc":                mem.NumGC,
		"server_uptime_seconds": int64(time.Since(serverStartTime).Seconds()),
		"log_level":             logger.GetLevel(),
		"log_permanent_level":   logger.GetPermanentLevel(),
	}

	c.JSON(http.StatusOK, gin.H{
		"users_count":       userCount,
		"files_count":       fileCount,
		"total_bytes":       totalBytes,
		"roles_count":       roleCount,
		"policies_count":    policyCount,
		"daily_trends":      daysList,
		"mime_distribution": mimeStats,
		"recent_files":      recentFilesDTO,
		"recent_users":      recentUsers,
		"system_info":       sysInfo,
		"recent_logs":       logger.GetRecentLogs(50),
		"recent_errors":     logger.GetRecentErrors(50),
	})
}

// -------------------------------------------------------------
// Users Management
// -------------------------------------------------------------

func (ctrl *AdminController) ListUsers(c *gin.Context) {
	var users []model.User
	ctrl.db.Preload("Role").Order("id ASC").Find(&users)
	c.JSON(http.StatusOK, users)
}

func (ctrl *AdminController) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=6"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		RoleID   uint   `json:"role_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if !pathclean.IsValidUsername(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名格式不合法或属于系统保留字，仅允许 3~32 位字母、数字、下划线及连字符"})
		return
	}

	var existingCount int64
	ctrl.db.Unscoped().Model(&model.User{}).Where("LOWER(username) = ?", req.Username).Count(&existingCount)
	if existingCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名已被使用"})
		return
	}

	if req.RoleID == 0 {
		req.RoleID = 2 // Default regular user role
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	var emailPtr *string
	if strings.TrimSpace(req.Email) != "" {
		em := strings.TrimSpace(req.Email)
		emailPtr = &em
	}

	user := model.User{
		Username:     req.Username,
		Nickname:     req.Nickname,
		Email:        emailPtr,
		PasswordHash: hash,
		RoleID:       req.RoleID,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := ctrl.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": common.SanitizeError(err)})
		return
	}

	ctrl.db.Preload("Role").First(&user, user.ID)
	c.JSON(http.StatusOK, user)
}

func (ctrl *AdminController) UpdateUser(c *gin.Context) {
	userID := c.Param("id")
	var user model.User
	if err := ctrl.db.Preload("Role").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req struct {
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		RoleID   uint   `json:"role_id"`
		Status   string `json:"status"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uVal, _ := c.Get(auth.CtxUserKey)
	currentAdmin := uVal.(*model.User)

	if user.ID == 1 || user.ID == currentAdmin.ID {
		if req.RoleID > 0 && req.RoleID != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot downgrade primary administrator or yourself"})
			return
		}
		if req.Status != "" && req.Status != "active" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot disable or ban primary administrator or yourself"})
			return
		}
	}

	isAdminTarget := user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
	if (req.RoleID > 0 && req.RoleID != 1 && isAdminTarget) || (req.Status != "" && req.Status != "active" && isAdminTarget) {
		var activeAdminCount int64
		ctrl.db.Model(&model.User{}).Where("role_id = 1 AND status = 'active' AND id != ?", user.ID).Count(&activeAdminCount)
		if activeAdminCount == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "System must retain at least one active administrator"})
			return
		}
	}

	updates := map[string]any{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Email != "" {
		em := strings.TrimSpace(req.Email)
		updates["email"] = &em
	}
	if req.RoleID > 0 {
		updates["role_id"] = req.RoleID
	}
	if req.Status != "" {
		updates["status"] = req.Status
		if req.Status == "disabled" || req.Status == "banned" {
			updates["token_version"] = gorm.Expr("token_version + 1")
			ctrl.db.Where("user_id = ?", user.ID).Delete(&model.Session{})
			ctrl.db.Where("user_id = ?", user.ID).Delete(&model.ApiToken{})
		}
	}
	if req.Password != "" {
		h, _ := auth.HashPassword(req.Password)
		updates["password_hash"] = h
		updates["token_version"] = gorm.Expr("token_version + 1")
		ctrl.db.Where("user_id = ?", user.ID).Delete(&model.Session{})
		ctrl.db.Where("user_id = ?", user.ID).Delete(&model.ApiToken{})
	}

	if err := ctrl.db.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": common.SanitizeError(err)})
		return
	}
	ctrl.db.Preload("Role").Where("id = ?", user.ID).First(&user)
	c.JSON(http.StatusOK, gin.H{"message": "User updated", "user": user})
}

func (ctrl *AdminController) DeleteUser(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	uVal, _ := c.Get(auth.CtxUserKey)
	currentAdmin := uVal.(*model.User)
	if id == currentAdmin.ID {
		common.RespondError(c, http.StatusBadRequest, "CANNOT_DELETE_SELF")
		return
	}
	if id == 1 {
		common.RespondError(c, http.StatusBadRequest, "CANNOT_DELETE_ADMIN")
		return
	}

	var targetUser model.User
	if err := ctrl.db.Preload("Role").First(&targetUser, id).Error; err == nil {
		if targetUser.RoleID == 1 || (targetUser.Role != nil && targetUser.Role.Name == "admin") {
			common.RespondError(c, http.StatusBadRequest, "CANNOT_DELETE_ADMIN")
			return
		}
	} else {
		common.RespondError(c, http.StatusNotFound, common.ErrAuthUserNotFound)
		return
	}

	// Clean up user's files from storage and return storage policy capacity
	var userFiles []model.FileRecord
	ctrl.db.Where("user_id = ?", id).Find(&userFiles)
	for _, f := range userFiles {
		_ = ctrl.uploadService.DeleteFile(c.Request.Context(), f.ID, id, true)
	}

	err := ctrl.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("user_id = ?", id).Delete(&model.Session{})
		tx.Where("user_id = ?", id).Delete(&model.ApiToken{})
		tx.Where("user_id = ?", id).Delete(&model.TOTPSecret{})
		tx.Where("user_id = ?", id).Delete(&model.WebAuthnCredential{})
		tx.Where("user_id = ?", id).Delete(&model.RecoveryCode{})
		tx.Where("user_id = ?", id).Delete(&model.PasswordResetToken{})
		res := tx.Unscoped().Where("id = ?", id).Delete(&model.User{})
		if res.RowsAffected == 0 {
			return errors.New("user not found")
		}
		return nil
	})
	if err != nil {
		common.RespondError(c, http.StatusNotFound, common.ErrAuthUserNotFound)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

func (ctrl *AdminController) BanUser(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}
	uVal, _ := c.Get(auth.CtxUserKey)
	currentAdmin := uVal.(*model.User)
	if id == currentAdmin.ID {
		common.RespondError(c, http.StatusBadRequest, "CANNOT_BAN_SELF")
		return
	}
	if id == 1 {
		common.RespondError(c, http.StatusBadRequest, "CANNOT_BAN_ADMIN")
		return
	}
	var user model.User
	if err := ctrl.db.Preload("Role").First(&user, id).Error; err != nil {
		common.RespondError(c, http.StatusNotFound, common.ErrAuthUserNotFound)
		return
	}
	if user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin") {
		common.RespondError(c, http.StatusBadRequest, "CANNOT_BAN_ADMIN")
		return
	}
	user.Status = "banned"
	user.TokenVersion++
	if err := ctrl.db.Save(&user).Error; err != nil {
		slog.Error("Failed to ban user", "user_id", user.ID, "error", err)
		common.RespondError(c, http.StatusInternalServerError, common.ErrInternalServerError)
		return
	}
	ctrl.db.Where("user_id = ?", user.ID).Delete(&model.Session{})
	ctrl.db.Where("user_id = ?", user.ID).Delete(&model.ApiToken{})
	slog.Warn("User banned by admin", "admin_id", currentAdmin.ID, "target_user_id", user.ID, "username", user.Username)
	c.JSON(http.StatusOK, gin.H{"message": "User banned successfully", "status": user.Status})
}

func (ctrl *AdminController) UnfreezeUser(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		common.RespondError(c, http.StatusBadRequest, common.ErrValidationFailed)
		return
	}
	var user model.User
	if err := ctrl.db.First(&user, id).Error; err != nil {
		common.RespondError(c, http.StatusNotFound, common.ErrAuthUserNotFound)
		return
	}
	user.Status = "active"
	user.FailedLoginAttempts = 0
	user.LockoutUntil = nil
	if err := ctrl.db.Save(&user).Error; err != nil {
		slog.Error("Failed to unfreeze user", "user_id", user.ID, "error", err)
		common.RespondError(c, http.StatusInternalServerError, common.ErrInternalServerError)
		return
	}
	slog.Info("User unfreezed and activated by admin", "target_user_id", user.ID, "username", user.Username)
	c.JSON(http.StatusOK, gin.H{"message": "User unlocked and activated successfully", "status": user.Status})
}

// -------------------------------------------------------------
// Roles Management (Requirement 8)
// -------------------------------------------------------------

type RoleDTO struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	QuotaBytes  int64     `json:"quota_bytes"`
	IsSystem    bool      `json:"is_system"`
	UserCount   int64     `json:"user_count"`
	PolicyIDs   []uint    `json:"policy_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ctrl *AdminController) ListRoles(c *gin.Context) {
	var roles []model.Role
	ctrl.db.Preload("Policies").Order("id ASC").Find(&roles)

	dtos := make([]RoleDTO, 0, len(roles))
	for _, r := range roles {
		var uCount int64
		ctrl.db.Model(&model.User{}).Where("role_id = ?", r.ID).Count(&uCount)

		policyIDs := make([]uint, 0, len(r.Policies))
		for _, p := range r.Policies {
			policyIDs = append(policyIDs, p.ID)
		}

		dtos = append(dtos, RoleDTO{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			QuotaBytes:  r.QuotaBytes,
			IsSystem:    r.IsSystem,
			UserCount:   uCount,
			PolicyIDs:   policyIDs,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, dtos)
}

func (ctrl *AdminController) CreateRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		DisplayName string `json:"display_name" binding:"required"`
		QuotaBytes  int64  `json:"quota_bytes"`
		PolicyIDs   []uint `json:"policy_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := model.Role{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		QuotaBytes:  req.QuotaBytes,
		IsSystem:    false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := ctrl.db.Create(&role).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to create role: " + err.Error()})
		return
	}

	for _, pid := range req.PolicyIDs {
		ctrl.db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: pid})
	}

	c.JSON(http.StatusOK, role)
}

func (ctrl *AdminController) UpdateRole(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	var role model.Role
	if err := ctrl.db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
		QuotaBytes  *int64 `json:"quota_bytes"`
		PolicyIDs   []uint `json:"policy_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{"updated_at": time.Now()}
	if req.DisplayName != "" {
		updates["display_name"] = req.DisplayName
	}
	if req.QuotaBytes != nil {
		updates["quota_bytes"] = *req.QuotaBytes
	}

	ctrl.db.Model(&role).Updates(updates)

	// Update role policy associations
	if req.PolicyIDs != nil {
		ctrl.db.Where("role_id = ?", role.ID).Delete(&model.RolePolicy{})
		for _, pid := range req.PolicyIDs {
			ctrl.db.Create(&model.RolePolicy{RoleID: role.ID, PolicyID: pid})
		}
	}

	ctrl.db.Preload("Policies").First(&role, role.ID)
	c.JSON(http.StatusOK, role)
}

func (ctrl *AdminController) DeleteRole(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	var role model.Role
	if err := ctrl.db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	if role.IsSystem || role.Name == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "System administrator role cannot be deleted"})
		return
	}

	var userCount int64
	ctrl.db.Model(&model.User{}).Where("role_id = ?", role.ID).Count(&userCount)
	if userCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete role while users are still assigned to it"})
		return
	}

	ctrl.db.Where("role_id = ?", role.ID).Delete(&model.RolePolicy{})
	ctrl.db.Delete(&role)
	c.JSON(http.StatusOK, gin.H{"message": "Role deleted successfully"})
}

// -------------------------------------------------------------
// Storage Policies Management (Requirement 8)
// -------------------------------------------------------------

type PolicyDTO struct {
	ID               uint      `json:"id"`
	Name             string    `json:"name"`
	Driver           string    `json:"driver"`
	Config           string    `json:"config"`
	MaxCapacityBytes int64     `json:"max_capacity_bytes"`
	UsedBytes        int64     `json:"used_bytes"`
	IsDefault        bool      `json:"is_default"`
	PathPrefix       string    `json:"path_prefix"`
	NamingRule       string    `json:"naming_rule"`
	RoleIDs          []uint    `json:"role_ids"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (ctrl *AdminController) ListPolicies(c *gin.Context) {
	var policies []model.StoragePolicy
	ctrl.db.Preload("Roles").Order("id ASC").Find(&policies)

	dtos := make([]PolicyDTO, 0, len(policies))
	for _, p := range policies {
		roleIDs := make([]uint, 0, len(p.Roles))
		for _, r := range p.Roles {
			roleIDs = append(roleIDs, r.ID)
		}
		dtos = append(dtos, PolicyDTO{
			ID:               p.ID,
			Name:             p.Name,
			Driver:           p.Driver,
			Config:           p.Config,
			MaxCapacityBytes: p.MaxCapacityBytes,
			UsedBytes:        p.UsedBytes,
			IsDefault:        p.IsDefault,
			PathPrefix:       p.PathPrefix,
			NamingRule:       p.NamingRule,
			RoleIDs:          roleIDs,
			CreatedAt:        p.CreatedAt,
			UpdatedAt:        p.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, dtos)
}

func (ctrl *AdminController) CreatePolicy(c *gin.Context) {
	var req struct {
		Name             string  `json:"name" binding:"required"`
		Driver           string  `json:"driver"`
		MaxCapacityBytes int64   `json:"max_capacity_bytes"`
		IsDefault        bool    `json:"is_default"`
		PathPrefix       *string `json:"path_prefix"`
		NamingRule       string  `json:"naming_rule"`
		RoleIDs          []uint  `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Currently only "local" driver is supported
	driver := "local"
	if req.NamingRule == "" {
		req.NamingRule = "{Y}/{m}/{d}/{random:16}.{ext}"
	}
	pathPrefix := ""
	if req.PathPrefix != nil {
		pathPrefix = strings.Trim(strings.TrimSpace(*req.PathPrefix), "/")
	}

	if req.IsDefault {
		ctrl.db.Model(&model.StoragePolicy{}).Where("1 = 1").Update("is_default", false)
	}

	policy := model.StoragePolicy{
		Name:             req.Name,
		Driver:           driver,
		Config:           `{"base_path":"data/storage"}`,
		MaxCapacityBytes: req.MaxCapacityBytes,
		IsDefault:        req.IsDefault,
		PathPrefix:       pathPrefix,
		NamingRule:       req.NamingRule,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := ctrl.db.Create(&policy).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, rid := range req.RoleIDs {
		ctrl.db.Create(&model.RolePolicy{RoleID: rid, PolicyID: policy.ID})
	}

	c.JSON(http.StatusOK, policy)
}

func (ctrl *AdminController) UpdatePolicy(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	var policy model.StoragePolicy
	if err := ctrl.db.Where("id = ?", id).First(&policy).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Policy not found"})
		return
	}

	var req struct {
		Name             string  `json:"name"`
		MaxCapacityBytes *int64  `json:"max_capacity_bytes"`
		IsDefault        bool    `json:"is_default"`
		PathPrefix       *string `json:"path_prefix"`
		NamingRule       string  `json:"naming_rule"`
		RoleIDs          []uint  `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.IsDefault {
		ctrl.db.Model(&model.StoragePolicy{}).Where("id != ?", policy.ID).Update("is_default", false)
	}

	updates := map[string]any{
		"updated_at": time.Now(),
		"is_default": req.IsDefault,
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.MaxCapacityBytes != nil {
		updates["max_capacity_bytes"] = *req.MaxCapacityBytes
	}
	if req.PathPrefix != nil {
		updates["path_prefix"] = strings.Trim(strings.TrimSpace(*req.PathPrefix), "/")
	}
	if req.NamingRule != "" {
		updates["naming_rule"] = req.NamingRule
	}

	ctrl.db.Model(&policy).Updates(updates)

	// Update role policy associations
	if req.RoleIDs != nil {
		ctrl.db.Where("policy_id = ?", policy.ID).Delete(&model.RolePolicy{})
		for _, rid := range req.RoleIDs {
			ctrl.db.Create(&model.RolePolicy{RoleID: rid, PolicyID: policy.ID})
		}
	}

	ctrl.db.Preload("Roles").First(&policy, policy.ID)
	c.JSON(http.StatusOK, policy)
}

func (ctrl *AdminController) DeletePolicy(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	var policy model.StoragePolicy
	if err := ctrl.db.Where("id = ?", id).First(&policy).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Policy not found"})
		return
	}

	if policy.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Default storage policy cannot be deleted"})
		return
	}

	var fileCount int64
	ctrl.db.Model(&model.FileRecord{}).Where("policy_id = ?", policy.ID).Count(&fileCount)
	if fileCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete storage policy that currently stores files"})
		return
	}

	ctrl.db.Where("policy_id = ?", policy.ID).Delete(&model.RolePolicy{})
	ctrl.db.Delete(&policy)
	c.JSON(http.StatusOK, gin.H{"message": "Policy deleted successfully"})
}

// -------------------------------------------------------------
// Image Presets
// -------------------------------------------------------------

func (ctrl *AdminController) ListPresets(c *gin.Context) {
	var presets []model.ImagePreset
	ctrl.db.Find(&presets)
	c.JSON(http.StatusOK, presets)
}

func (ctrl *AdminController) CreatePreset(c *gin.Context) {
	var preset model.ImagePreset
	if err := c.ShouldBindJSON(&preset); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.db.Create(&preset).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, preset)
}

func (ctrl *AdminController) DeletePreset(c *gin.Context) {
	id := c.Param("id")
	ctrl.db.Where("id = ?", id).Delete(&model.ImagePreset{})
	c.JSON(http.StatusOK, gin.H{"message": "Preset deleted"})
}

// -------------------------------------------------------------
// Database Status & Migration
// -------------------------------------------------------------

func (ctrl *AdminController) GetDatabaseStatus(c *gin.Context) {
	var userCount, fileCount, policyCount, roleCount int64
	ctrl.db.Model(&model.User{}).Count(&userCount)
	ctrl.db.Model(&model.FileRecord{}).Count(&fileCount)
	ctrl.db.Model(&model.StoragePolicy{}).Count(&policyCount)
	ctrl.db.Model(&model.Role{}).Count(&roleCount)

	currentDriver := ctrl.db.Dialector.Name()

	c.JSON(http.StatusOK, gin.H{
		"driver": currentDriver,
		"counts": gin.H{
			"users":            userCount,
			"files":            fileCount,
			"roles":            roleCount,
			"storage_policies": policyCount,
		},
	})
}

// -------------------------------------------------------------
// Site-wide Files Management (Requirement 1)
// -------------------------------------------------------------

type AdminFileItem struct {
	ID            uint      `json:"id"`
	PublicPath    string    `json:"public_path"`
	StorageKey    string    `json:"storage_key"`
	DisplayName   string    `json:"display_name"`
	MimeType      string    `json:"mime_type"`
	Size          int64     `json:"size"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	Sha256        string    `json:"sha256"`
	IsImage       bool      `json:"is_image"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	URL           string    `json:"url"`
	ThumbnailURL  string    `json:"thumbnail_url"`
	TempLinkCount int64     `json:"temp_link_count"`
	TotalCalls    int64     `json:"total_calls"`
	User          struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	} `json:"user"`
	Policy struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		Driver string `json:"driver"`
	} `json:"policy"`
}

func (ctrl *AdminController) ListAllFiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := ctrl.db.Model(&model.FileRecord{})

	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%" + search + "%"
		var matchingUserIDs []uint
		ctrl.db.Model(&model.User{}).Where("username LIKE ? OR nickname LIKE ? OR email LIKE ?", like, like, like).Pluck("id", &matchingUserIDs)
		if len(matchingUserIDs) > 0 {
			query = query.Where("display_name LIKE ? OR public_path LIKE ? OR user_id IN (?)", like, like, matchingUserIDs)
		} else {
			query = query.Where("display_name LIKE ? OR public_path LIKE ?", like, like)
		}
	}

	if policyID := c.Query("policy_id"); policyID != "" && policyID != "all" {
		query = query.Where("policy_id = ?", policyID)
	}

	if uid := c.Query("user_id"); uid != "" && uid != "all" {
		query = query.Where("user_id = ?", uid)
	}

	if format := strings.ToLower(c.Query("format")); format != "" && format != "all" {
		switch format {
		case "jpeg", "jpg":
			query = query.Where("mime_type IN ('image/jpeg', 'image/jpg') OR public_path LIKE '%.jpg' OR public_path LIKE '%.jpeg'")
		case "png":
			query = query.Where("mime_type = 'image/png' OR public_path LIKE '%.png'")
		case "webp":
			query = query.Where("mime_type = 'image/webp' OR public_path LIKE '%.webp'")
		case "gif":
			query = query.Where("mime_type = 'image/gif' OR public_path LIKE '%.gif'")
		case "svg":
			query = query.Where("mime_type = 'image/svg+xml' OR public_path LIKE '%.svg'")
		case "other":
			query = query.Where("is_image = 0")
		}
	}

	if timeRange := c.Query("time_range"); timeRange != "" && timeRange != "all" {
		now := time.Now()
		switch timeRange {
		case "today":
			startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			query = query.Where("created_at >= ?", startOfDay)
		case "7d":
			query = query.Where("created_at >= ?", now.Add(-7*24*time.Hour))
		case "30d":
			query = query.Where("created_at >= ?", now.Add(-30*24*time.Hour))
		}
	}

	if hasLinks := c.Query("has_temp_links"); hasLinks == "true" {
		query = query.Where("id IN (SELECT DISTINCT file_id FROM temporary_links WHERE revoked_at IS NULL AND expires_at > ?)", time.Now())
	}

	var total int64
	query.Count(&total)

	var files []model.FileRecord
	query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&files)

	userIDs := make([]uint, 0, len(files))
	policyIDs := make([]uint, 0, len(files))
	fileIDs := make([]uint, 0, len(files))
	for _, f := range files {
		userIDs = append(userIDs, f.UserID)
		policyIDs = append(policyIDs, f.PolicyID)
		fileIDs = append(fileIDs, f.ID)
	}

	usersMap := make(map[uint]model.User)
	if len(userIDs) > 0 {
		var users []model.User
		ctrl.db.Where("id IN (?)", userIDs).Find(&users)
		for _, u := range users {
			usersMap[u.ID] = u
		}
	}

	policiesMap := make(map[uint]model.StoragePolicy)
	if len(policyIDs) > 0 {
		var policies []model.StoragePolicy
		ctrl.db.Where("id IN (?)", policyIDs).Find(&policies)
		for _, p := range policies {
			policiesMap[p.ID] = p
		}
	}

	linkCountMap := make(map[uint]int64)
	if len(fileIDs) > 0 {
		type countResult struct {
			FileID uint  `gorm:"column:file_id"`
			Cnt    int64 `gorm:"column:cnt"`
		}
		var counts []countResult
		ctrl.db.Model(&model.TemporaryLink{}).
			Select("file_id, count(*) as cnt").
			Where("file_id IN (?) AND revoked_at IS NULL AND expires_at > ?", fileIDs, time.Now()).
			Group("file_id").
			Scan(&counts)
		for _, cr := range counts {
			linkCountMap[cr.FileID] = cr.Cnt
		}
	}

	callCountMap := make(map[uint]int64)
	if len(fileIDs) > 0 {
		type callResult struct {
			FileID uint  `gorm:"column:file_id"`
			Cnt    int64 `gorm:"column:cnt"`
		}
		var calls []callResult
		ctrl.db.Model(&model.ImageCallStat{}).
			Select("file_id, sum(count) as cnt").
			Where("file_id IN (?)", fileIDs).
			Group("file_id").
			Scan(&calls)
		for _, cr := range calls {
			callCountMap[cr.FileID] = cr.Cnt
		}
	}

	currentBaseURL := ctrl.resolveBaseURL(c)
	items := make([]AdminFileItem, 0, len(files))
	for _, f := range files {
		cleanPath := strings.TrimPrefix(f.PublicPath, "/")
		var fileURL, thumbURL string
		if currentBaseURL != "" {
			fileURL = fmt.Sprintf("%s/%s", currentBaseURL, cleanPath)
			if f.IsImage {
				thumbURL = fmt.Sprintf("%s/%s?s=thumb", currentBaseURL, cleanPath)
			} else {
				thumbURL = fileURL
			}
		} else {
			fileURL = fmt.Sprintf("/%s", cleanPath)
			if f.IsImage {
				thumbURL = fmt.Sprintf("/%s?s=thumb", cleanPath)
			} else {
				thumbURL = fileURL
			}
		}
		item := AdminFileItem{
			ID:            f.ID,
			PublicPath:    f.PublicPath,
			StorageKey:    f.StorageKey,
			DisplayName:   f.DisplayName,
			MimeType:      f.MimeType,
			Size:          f.Size,
			Width:         f.Width,
			Height:        f.Height,
			Sha256:        f.Sha256,
			IsImage:       f.IsImage,
			CreatedAt:     f.CreatedAt,
			UpdatedAt:     f.UpdatedAt,
			URL:           fileURL,
			ThumbnailURL:  thumbURL,
			TempLinkCount: linkCountMap[f.ID],
			TotalCalls:    callCountMap[f.ID],
		}
		if u, ok := usersMap[f.UserID]; ok {
			item.User.ID = u.ID
			item.User.Username = u.Username
			item.User.Nickname = u.Nickname
			if u.Email != nil {
				item.User.Email = *u.Email
			}
		}
		if p, ok := policiesMap[f.PolicyID]; ok {
			item.Policy.ID = p.ID
			item.Policy.Name = p.Name
			item.Policy.Driver = p.Driver
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"items":     items,
		"base_url":  currentBaseURL,
		"base_urls": GetConfiguredBaseURLs(c, ctrl.db, ctrl.baseURL),
	})
}

func (ctrl *AdminController) GetAllFileDetails(c *gin.Context) {
	id := c.Param("id")
	var file model.FileRecord
	if err := ctrl.db.Where("id = ?", id).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var user model.User
	ctrl.db.Where("id = ?", file.UserID).First(&user)

	var policy model.StoragePolicy
	ctrl.db.Where("id = ?", file.PolicyID).First(&policy)

	var tempLinks []model.TemporaryLink
	ctrl.db.Where("file_id = ?", file.ID).Order("id DESC").Find(&tempLinks)

	currentBaseURL := ctrl.resolveBaseURL(c)
	cleanPath := strings.TrimPrefix(file.PublicPath, "/")
	ext := filepath.Ext(file.PublicPath)
	baseNoExt := strings.TrimSuffix(cleanPath, ext)

	var rawURL, webpURL string
	if currentBaseURL != "" {
		rawURL = fmt.Sprintf("%s/%s", currentBaseURL, cleanPath)
		webpURL = fmt.Sprintf("%s/%s.webp", currentBaseURL, baseNoExt)
	} else {
		rawURL = fmt.Sprintf("/%s", cleanPath)
		webpURL = fmt.Sprintf("/%s.webp", baseNoExt)
	}

	safeHTMLName := html.EscapeString(file.DisplayName)
	safeMdName := strings.ReplaceAll(strings.ReplaceAll(file.DisplayName, "[", "\\["), "]", "\\]")

	links := map[string]string{
		"url":           rawURL,
		"webp_url":      webpURL,
		"markdown":      fmt.Sprintf("![%s](%s)", safeMdName, rawURL),
		"bbcode":        fmt.Sprintf("[img]%s[/img]", rawURL),
		"html":          fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", rawURL, safeHTMLName),
		"webp_markdown": fmt.Sprintf("![%s](%s)", safeMdName, webpURL),
		"webp_html":     fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", webpURL, safeHTMLName),
	}

	c.JSON(http.StatusOK, gin.H{
		"file": file,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
			"email":    user.Email,
		},
		"policy":     policy,
		"links":      links,
		"temp_links": tempLinks,
		"call_stats": func() []model.ImageCallStat {
			var stats []model.ImageCallStat
			ctrl.db.Where("file_id = ?", file.ID).Order("count DESC").Find(&stats)
			return stats
		}(),
		"total_calls": func() int64 {
			var sum *int64
			ctrl.db.Model(&model.ImageCallStat{}).Where("file_id = ?", file.ID).Select("sum(count)").Scan(&sum)
			if sum != nil {
				return *sum
			}
			return 0
		}(),
	})
}

func (ctrl *AdminController) DeleteSiteFile(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	err := ctrl.uploadService.DeleteFile(c.Request.Context(), id, 0, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
}

func (ctrl *AdminController) BatchDeleteSiteFiles(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	deletedCount := 0
	for _, id := range req.IDs {
		if err := ctrl.uploadService.DeleteFile(c.Request.Context(), id, 0, true); err == nil {
			deletedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deletedCount})
}
