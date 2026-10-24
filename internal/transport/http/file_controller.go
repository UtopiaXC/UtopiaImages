package http

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type FileController struct {
	db              *gorm.DB
	uploadService   *service.UploadService
	dispatchService *service.DispatchService
	importService   *service.ImportService
	baseURL         string
}

func NewFileController(
	db *gorm.DB,
	us *service.UploadService,
	ds *service.DispatchService,
	is *service.ImportService,
	baseURL string,
) *FileController {
	return &FileController{
		db:              db,
		uploadService:   us,
		dispatchService: ds,
		importService:   is,
		baseURL:         baseURL,
	}
}

func (ctrl *FileController) RegisterRoutes(rg *gin.RouterGroup) {
	files := rg.Group("/files")
	files.Use(auth.RequireAuth())
	{
		files.GET("", ctrl.ListFiles)
		files.GET("/:id", ctrl.GetFile)
		files.PATCH("/:id", ctrl.UpdateFile)
		files.DELETE("/:id", ctrl.DeleteFile)
		files.POST("/batch-delete", ctrl.BatchDeleteFiles)
		files.GET("/:id/raw", ctrl.DownloadRaw)
		files.POST("/:id/temp-link", ctrl.CreateTempLink)
		files.POST("/:id/revoke-links", ctrl.RevokeTempLinks)
	}

	rg.POST("/upload", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.Upload)
	rg.POST("/import/zip", auth.RequireAuth(), auth.RequirePermission("import.use"), ctrl.ImportZip)
}

func (ctrl *FileController) Upload(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File form field is required"})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
		return
	}
	defer src.Close()

	policyID := ParseUintSafe(c.PostForm("policy_id"), 0)
	convertToWebP := c.PostForm("convert_webp") == "true" || c.PostForm("convert_webp") == "1"
	keepOriginal := c.PostForm("keep_original") == "true" || c.PostForm("keep_original") == "1"
	quality, _ := strconv.Atoi(c.PostForm("quality"))
	watermarkMode := c.DefaultPostForm("watermark_mode", "inherit")
	displayName := c.PostForm("display_name")
	if displayName == "" {
		displayName = fileHeader.Filename
	}

	record, err := ctrl.uploadService.Upload(c.Request.Context(), user, src, fileHeader.Filename, fileHeader.Size, service.UploadOptions{
		PolicyID:      policyID,
		DisplayName:   displayName,
		ConvertToWebP: convertToWebP,
		Quality:       quality,
		KeepOriginal:  keepOriginal,
		WatermarkMode: watermarkMode,
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate links
	directURL := fmt.Sprintf("%s/%s", ctrl.baseURL, record.PublicPath)
	links := gin.H{
		"url":       directURL,
		"markdown":  fmt.Sprintf("![%s](%s)", record.DisplayName, directURL),
		"html":      fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", directURL, record.DisplayName),
		"bbcode":    fmt.Sprintf("[img]%s[/img]", directURL),
		"thumb_url": fmt.Sprintf("%s?s=thumb", directURL),
	}

	c.JSON(http.StatusOK, gin.H{
		"file":  record,
		"links": links,
	})
}

func (ctrl *FileController) ListFiles(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "24"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}

	query := ctrl.db.Model(&model.FileRecord{})
	if user.Role != "admin" {
		query = query.Where("user_id = ?", user.ID)
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("display_name LIKE ? OR public_path LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if isImage := c.Query("is_image"); isImage != "" {
		query = query.Where("is_image = ?", isImage == "true" || isImage == "1")
	}

	var total int64
	query.Count(&total)

	var files []model.FileRecord
	query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&files)

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"items":     files,
		"base_url":  ctrl.baseURL,
	})
}

func (ctrl *FileController) GetFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if user.Role != "admin" {
		query = query.Where("user_id = ?", user.ID)
	}

	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	directURL := fmt.Sprintf("%s/%s", ctrl.baseURL, file.PublicPath)
	c.JSON(http.StatusOK, gin.H{
		"file": file,
		"links": gin.H{
			"url":       directURL,
			"markdown":  fmt.Sprintf("![%s](%s)", file.DisplayName, directURL),
			"html":      fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", directURL, file.DisplayName),
			"bbcode":    fmt.Sprintf("[img]%s[/img]", directURL),
			"thumb_url": fmt.Sprintf("%s?s=thumb", directURL),
		},
	})
}

func (ctrl *FileController) UpdateFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if user.Role != "admin" {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var req struct {
		DisplayName   string `json:"display_name"`
		WatermarkMode string `json:"watermark_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.DisplayName != "" {
		updates["display_name"] = req.DisplayName
	}
	if req.WatermarkMode != "" {
		updates["watermark_mode"] = req.WatermarkMode
	}

	ctrl.db.Model(&file).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"message": "File updated successfully", "file": file})
}

func (ctrl *FileController) DeleteFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := ParseUintSafe(c.Param("id"), 0)

	err := ctrl.uploadService.DeleteFile(c.Request.Context(), fileID, user.ID, user.Role == "admin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
}

func (ctrl *FileController) BatchDeleteFiles(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	deletedCount := 0
	for _, id := range req.IDs {
		if err := ctrl.uploadService.DeleteFile(c.Request.Context(), id, user.ID, user.Role == "admin"); err == nil {
			deletedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deletedCount})
}

func (ctrl *FileController) DownloadRaw(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := ParseUintSafe(c.Param("id"), 0)

	ctrl.dispatchService.ServeRaw(c, fileID, user.ID, user.Role == "admin")
}

func (ctrl *FileController) ImportZip(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	fileHeader, err := c.FormFile("zip_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zip_file is required"})
		return
	}

	// Save temp zip
	tmp, err := os.CreateTemp("", "import-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp file"})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if err := c.SaveUploadedFile(fileHeader, tmp.Name()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded zip"})
		return
	}

	preservePath := c.PostForm("preserve_path") == "true" || c.PostForm("preserve_path") == "1"
	conflictAction := c.DefaultPostForm("conflict_action", "skip")
	policyID := ParseUintSafe(c.PostForm("policy_id"), 0)

	total, imported, skipped, err := ctrl.importService.ImportZip(c.Request.Context(), user, tmp.Name(), policyID, service.ImportOptions{
		PreservePath:   preservePath,
		ConflictAction: conflictAction,
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"imported": imported,
		"skipped":  skipped,
	})
}

func (ctrl *FileController) CreateTempLink(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if user.Role != "admin" {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var req struct {
		ExpiresHours int `json:"expires_hours"` // e.g. 1, 24, 168
	}
	_ = c.ShouldBindJSON(&req)
	if req.ExpiresHours <= 0 {
		req.ExpiresHours = 24
	}

	tempPath := ctrl.dispatchService.CreateTempLink(&file, time.Duration(req.ExpiresHours)*time.Hour)
	fullURL := fmt.Sprintf("%s%s", ctrl.baseURL, tempPath)

	c.JSON(http.StatusOK, gin.H{
		"temp_url":      fullURL,
		"expires_hours": req.ExpiresHours,
	})
}

func (ctrl *FileController) RevokeTempLinks(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if user.Role != "admin" {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	ctrl.db.Model(&file).Update("sign_version", gorm.Expr("sign_version + 1"))
	c.JSON(http.StatusOK, gin.H{"message": "All previous temporary links have been revoked"})
}

