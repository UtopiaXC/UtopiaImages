package http

import (
	"net/http"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminController struct {
	db *gorm.DB
}

func NewAdminController(db *gorm.DB) *AdminController {
	return &AdminController{db: db}
}

func (ctrl *AdminController) RegisterRoutes(rg *gin.RouterGroup) {
	admin := rg.Group("/admin")
	admin.Use(auth.RequireAuth(), auth.RequireAdmin())
	{
		admin.GET("/stats", ctrl.GetStats)

		// Users management
		admin.GET("/users", ctrl.ListUsers)
		admin.POST("/users", ctrl.CreateUser)
		admin.PATCH("/users/:id", ctrl.UpdateUser)

		// Storage policies
		admin.GET("/policies", ctrl.ListPolicies)
		admin.POST("/policies", ctrl.CreatePolicy)
		admin.DELETE("/policies/:id", ctrl.DeletePolicy)

		// Image Presets
		admin.GET("/presets", ctrl.ListPresets)
		admin.POST("/presets", ctrl.CreatePreset)
		admin.DELETE("/presets/:id", ctrl.DeletePreset)
	}
}

func (ctrl *AdminController) GetStats(c *gin.Context) {
	var userCount int64
	var fileCount int64
	var totalBytes int64

	ctrl.db.Model(&model.User{}).Count(&userCount)
	ctrl.db.Model(&model.FileRecord{}).Count(&fileCount)

	type Result struct {
		Total int64
	}
	var res Result
	ctrl.db.Model(&model.FileRecord{}).Select("COALESCE(SUM(size), 0) as total").Scan(&res)
	totalBytes = res.Total

	c.JSON(http.StatusOK, gin.H{
		"users_count": userCount,
		"files_count": fileCount,
		"total_bytes": totalBytes,
	})
}

func (ctrl *AdminController) ListUsers(c *gin.Context) {
	var users []model.User
	ctrl.db.Preload("Group").Order("id ASC").Find(&users)
	c.JSON(http.StatusOK, users)
}

func (ctrl *AdminController) CreateUser(c *gin.Context) {
	var req struct {
		Username     string `json:"username" binding:"required"`
		Password     string `json:"password" binding:"required,min=6"`
		Nickname     string `json:"nickname"`
		Email        string `json:"email"`
		Role         string `json:"role"`
		GroupID      uint   `json:"group_id"`
		StorageLimit *int64 `json:"storage_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}
	if req.GroupID == 0 {
		req.GroupID = 2 // Uploader group
	}

	user := model.User{
		Username:     req.Username,
		Nickname:     req.Nickname,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         req.Role,
		GroupID:      req.GroupID,
		Status:       "active",
		StorageLimit: req.StorageLimit,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := ctrl.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create default user storage policy (namespaced under username)
	policy := model.StoragePolicy{
		UserID:     user.ID,
		BackendID:  1,
		Name:       "Default Policy",
		UseRoot:    false,
		PathPrefix: "upload",
		NamingRule: "{Y}/{m}/{d}/{random:16}.{ext}",
		IsDefault:  true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	ctrl.db.Create(&policy)

	c.JSON(http.StatusOK, user)
}

func (ctrl *AdminController) UpdateUser(c *gin.Context) {
	userID := c.Param("id")
	var user model.User
	if err := ctrl.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req struct {
		Nickname     string `json:"nickname"`
		Status       string `json:"status"`
		GroupID      uint   `json:"group_id"`
		StorageLimit *int64 `json:"storage_limit"`
		Password     string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.GroupID > 0 {
		updates["group_id"] = req.GroupID
	}
	if req.StorageLimit != nil {
		updates["storage_limit"] = req.StorageLimit
	}
	if req.Password != "" {
		h, _ := auth.HashPassword(req.Password)
		updates["password_hash"] = h
		updates["token_version"] = gorm.Expr("token_version + 1")
	}

	ctrl.db.Model(&user).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"message": "User updated"})
}

func (ctrl *AdminController) ListPolicies(c *gin.Context) {
	var policies []model.StoragePolicy
	ctrl.db.Find(&policies)
	c.JSON(http.StatusOK, policies)
}

func (ctrl *AdminController) CreatePolicy(c *gin.Context) {
	var req struct {
		UserID     uint   `json:"user_id"`
		BackendID  uint   `json:"backend_id"`
		Name       string `json:"name" binding:"required"`
		UseRoot    bool   `json:"use_root"`
		PathPrefix string `json:"path_prefix"`
		NamingRule string `json:"naming_rule"`
		IsDefault  bool   `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.BackendID == 0 {
		req.BackendID = 1
	}

	policy := model.StoragePolicy{
		UserID:     req.UserID,
		BackendID:  req.BackendID,
		Name:       req.Name,
		UseRoot:    req.UseRoot,
		PathPrefix: req.PathPrefix,
		NamingRule: req.NamingRule,
		IsDefault:  req.IsDefault,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := ctrl.db.Create(&policy).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (ctrl *AdminController) DeletePolicy(c *gin.Context) {
	id := c.Param("id")
	ctrl.db.Where("id = ?", id).Delete(&model.StoragePolicy{})
	c.JSON(http.StatusOK, gin.H{"message": "Policy deleted"})
}

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
	preset.CreatedAt = time.Now()
	preset.UpdatedAt = time.Now()
	ctrl.db.Create(&preset)
	c.JSON(http.StatusOK, preset)
}

func (ctrl *AdminController) DeletePreset(c *gin.Context) {
	id := c.Param("id")
	ctrl.db.Where("id = ?", id).Delete(&model.ImagePreset{})
	c.JSON(http.StatusOK, gin.H{"message": "Preset deleted"})
}
