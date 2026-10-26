package http

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		files.GET("/ids", ctrl.ListFileIDs)
		files.GET("/:id", ctrl.GetFile)
		files.PATCH("/:id", ctrl.UpdateFile)
		files.DELETE("/:id", ctrl.DeleteFile)
		files.POST("/batch-delete", ctrl.BatchDeleteFiles)
		files.POST("/batch-update", ctrl.BatchUpdateFiles)
		files.GET("/:id/raw", ctrl.DownloadRaw)
		files.GET("/:id/temp-links", ctrl.ListTempLinks)
		files.POST("/:id/temp-links", ctrl.CreateTempLink)
		files.POST("/:id/temp-link", ctrl.CreateTempLink)
		files.DELETE("/:id/temp-links/:link_id", ctrl.RevokeTempLink)
		files.DELETE("/:id/temp-links", ctrl.RevokeAllTempLinks)
		files.POST("/:id/revoke-links", ctrl.RevokeAllTempLinks)
		files.POST("/:id/edit", ctrl.EditFile)
	}

	rg.POST("/upload", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.Upload)
	rg.POST("/import/archive", auth.RequireAuth(), auth.RequirePermission("import.use"), ctrl.ImportArchive)
	rg.POST("/import/zip", auth.RequireAuth(), auth.RequirePermission("import.use"), ctrl.ImportArchive)
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
	watermarkMode := c.DefaultPostForm("watermark_mode", "inherit")
	displayName := c.PostForm("display_name")
	if displayName == "" {
		displayName = fileHeader.Filename
	}

	var wmTemplateID *uint
	if tidStr := c.PostForm("watermark_template_id"); tidStr != "" {
		val := ParseUintSafe(tidStr, 0)
		if val > 0 {
			wmTemplateID = &val
		}
	}

	record, err := ctrl.uploadService.Upload(c.Request.Context(), user, src, fileHeader.Filename, fileHeader.Size, service.UploadOptions{
		PolicyID:            policyID,
		DisplayName:         displayName,
		WatermarkMode:       watermarkMode,
		WatermarkTemplateID: wmTemplateID,
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate links
	links := ctrl.buildFileLinks(record)

	c.JSON(http.StatusOK, gin.H{
		"file":  record,
		"links": links,
	})
}

func (ctrl *FileController) buildFileLinks(file *model.FileRecord) gin.H {
	directURL := fmt.Sprintf("%s/%s", ctrl.baseURL, file.PublicPath)
	links := gin.H{
		"url":       directURL,
		"markdown":  fmt.Sprintf("![%s](%s)", file.DisplayName, directURL),
		"html":      fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", directURL, file.DisplayName),
		"bbcode":    fmt.Sprintf("[img]%s[/img]", directURL),
		"thumb_url": fmt.Sprintf("%s?s=thumb", directURL),
	}

	if file.IsImage {
		ext := filepath.Ext(file.PublicPath)
		baseNoExt := strings.TrimSuffix(file.PublicPath, ext)
		webpURL := fmt.Sprintf("%s/%s.webp", ctrl.baseURL, baseNoExt)
		links["webp_url"] = webpURL
		links["webp_markdown"] = fmt.Sprintf("![%s](%s)", file.DisplayName, webpURL)
		links["webp_html"] = fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", webpURL, file.DisplayName)
		links["webp_bbcode"] = fmt.Sprintf("[img]%s[/img]", webpURL)
	}

	return links
}

func (ctrl *FileController) applyFileFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	if search := c.Query("search"); search != "" {
		query = query.Where("display_name LIKE ? OR public_path LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if isImage := c.Query("is_image"); isImage != "" {
		query = query.Where("is_image = ?", isImage == "true" || isImage == "1")
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
		case "heif", "heic":
			query = query.Where("mime_type IN ('image/heif', 'image/heic') OR public_path LIKE '%.heif' OR public_path LIKE '%.heic'")
		case "other":
			query = query.Where("is_image = 0")
		}
	}

	if wm := c.Query("watermark_mode"); wm != "" && wm != "all" {
		query = query.Where("watermark_mode = ?", wm)
	}

	if hl := c.Query("hotlink_mode"); hl != "" && hl != "all" {
		query = query.Where("hotlink_mode = ?", hl)
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

	return query
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
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}

	query = ctrl.applyFileFilters(query, c)

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

func (ctrl *FileController) ListFileIDs(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	query := ctrl.db.Model(&model.FileRecord{})
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}

	query = ctrl.applyFileFilters(query, c)

	var ids []uint
	query.Pluck("id", &ids)

	c.JSON(http.StatusOK, gin.H{
		"ids": ids,
	})
}

func (ctrl *FileController) GetFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}

	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	links := ctrl.buildFileLinks(&file)
	c.JSON(http.StatusOK, gin.H{
		"file":  file,
		"links": links,
	})
}

func isUserAdmin(user *model.User) bool {
	if user == nil {
		return false
	}
	return user.RoleID == 1 || (user.Role != nil && user.Role.Name == "admin")
}

func (ctrl *FileController) UpdateFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var req struct {
		DisplayName           string `json:"display_name"`
		WatermarkMode         string `json:"watermark_mode"`
		WatermarkTemplateID   *uint  `json:"watermark_template_id"`
		HotlinkMode           string `json:"hotlink_mode"`
		HotlinkAllowedDomains string `json:"hotlink_allowed_domains"`
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
	if req.WatermarkTemplateID != nil {
		updates["watermark_template_id"] = req.WatermarkTemplateID
	}
	if req.HotlinkMode != "" {
		updates["hotlink_mode"] = req.HotlinkMode
	}
	updates["hotlink_allowed_domains"] = req.HotlinkAllowedDomains

	ctrl.db.Model(&file).Updates(updates)
	ctrl.db.Where("id = ?", file.ID).First(&file)
	c.JSON(http.StatusOK, gin.H{"message": "File updated successfully", "file": file, "links": ctrl.buildFileLinks(&file)})
}

func (ctrl *FileController) DeleteFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := ParseUintSafe(c.Param("id"), 0)

	err := ctrl.uploadService.DeleteFile(c.Request.Context(), fileID, user.ID, isUserAdmin(user))
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
		if err := ctrl.uploadService.DeleteFile(c.Request.Context(), id, user.ID, isUserAdmin(user)); err == nil {
			deletedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deletedCount})
}

func (ctrl *FileController) BatchUpdateFiles(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		IDs                 []uint  `json:"ids" binding:"required"`
		WatermarkMode       *string `json:"watermark_mode"`
		WatermarkTemplateID *uint   `json:"watermark_template_id"`
		HotlinkMode         *string `json:"hotlink_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if req.WatermarkMode != nil {
		updates["watermark_mode"] = *req.WatermarkMode
	}
	if req.WatermarkTemplateID != nil {
		updates["watermark_template_id"] = *req.WatermarkTemplateID
	}
	if req.HotlinkMode != nil {
		updates["hotlink_mode"] = *req.HotlinkMode
	}

	query := ctrl.db.Model(&model.FileRecord{}).Where("id IN ?", req.IDs)
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}

	res := query.Updates(updates)
	c.JSON(http.StatusOK, gin.H{"updated": res.RowsAffected})
}

func (ctrl *FileController) DownloadRaw(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := ParseUintSafe(c.Param("id"), 0)

	ctrl.dispatchService.ServeRaw(c, fileID, user.ID, isUserAdmin(user))
}

func (ctrl *FileController) ImportArchive(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	fileHeader, err := c.FormFile("archive_file")
	if err != nil {
		fileHeader, err = c.FormFile("zip_file")
	}
	if err != nil {
		fileHeader, err = c.FormFile("file")
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "archive_file is required"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".tar.gz") {
		ext = ".tar.gz"
	}
	tmp, err := os.CreateTemp("", "import-*"+ext)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp file"})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if err := c.SaveUploadedFile(fileHeader, tmp.Name()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded archive"})
		return
	}

	preservePath := c.PostForm("preserve_path") == "true" || c.PostForm("preserve_path") == "1"
	stripTopDir := c.PostForm("strip_top_dir") == "true" || c.PostForm("strip_top_dir") == "1"
	conflictAction := c.DefaultPostForm("conflict_action", "skip")
	policyID := ParseUintSafe(c.PostForm("policy_id"), 0)
	watermarkMode := c.DefaultPostForm("watermark_mode", "inherit")
	hotlinkMode := c.DefaultPostForm("hotlink_mode", "inherit")

	var wmTemplateID *uint
	if tidStr := c.PostForm("watermark_template_id"); tidStr != "" {
		val := ParseUintSafe(tidStr, 0)
		if val > 0 {
			wmTemplateID = &val
		}
	}

	total, imported, skipped, err := ctrl.importService.ImportArchive(c.Request.Context(), user, tmp.Name(), fileHeader.Filename, policyID, service.ImportOptions{
		PreservePath:        preservePath,
		StripTopDir:         stripTopDir,
		ConflictAction:      conflictAction,
		WatermarkMode:       watermarkMode,
		WatermarkTemplateID: wmTemplateID,
		HotlinkMode:         hotlinkMode,
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
	if !isUserAdmin(user) {
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

	token, err := auth.GenerateRandomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	expiresAt := time.Now().Add(time.Duration(req.ExpiresHours) * time.Hour)
	link := model.TemporaryLink{
		FileID:    file.ID,
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	if err := ctrl.db.Create(&link).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save temporary link"})
		return
	}

	fullURL := fmt.Sprintf("%s/%s?token=%s", ctrl.baseURL, file.PublicPath, token)

	c.JSON(http.StatusOK, gin.H{
		"id":            link.ID,
		"token":         token,
		"temp_url":      fullURL,
		"expires_at":    expiresAt,
		"expires_hours": req.ExpiresHours,
		"created_at":    link.CreatedAt,
	})
}

func (ctrl *FileController) ListTempLinks(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var links []model.TemporaryLink
	ctrl.db.Where("file_id = ?", file.ID).Order("created_at desc").Find(&links)

	type TempLinkItem struct {
		ID        uint       `json:"id"`
		Token     string     `json:"token"`
		URL       string     `json:"url"`
		ExpiresAt time.Time  `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
		RevokedAt *time.Time `json:"revoked_at,omitempty"`
		IsExpired bool       `json:"is_expired"`
		IsRevoked bool       `json:"is_revoked"`
		IsActive  bool       `json:"is_active"`
	}

	now := time.Now()
	items := make([]TempLinkItem, 0, len(links))
	for _, l := range links {
		isExpired := l.ExpiresAt.Before(now)
		isRevoked := l.RevokedAt != nil
		items = append(items, TempLinkItem{
			ID:        l.ID,
			Token:     l.Token,
			URL:       fmt.Sprintf("%s/%s?token=%s", ctrl.baseURL, file.PublicPath, l.Token),
			ExpiresAt: l.ExpiresAt,
			CreatedAt: l.CreatedAt,
			RevokedAt: l.RevokedAt,
			IsExpired: isExpired,
			IsRevoked: isRevoked,
			IsActive:  !isExpired && !isRevoked,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"file_id": file.ID,
		"links":   items,
	})
}

func (ctrl *FileController) RevokeTempLink(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")
	linkID := c.Param("link_id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	now := time.Now()
	res := ctrl.db.Model(&model.TemporaryLink{}).
		Where("id = ? AND file_id = ? AND revoked_at IS NULL", linkID, file.ID).
		Update("revoked_at", &now)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Temporary link revoked successfully"})
}

func (ctrl *FileController) RevokeAllTempLinks(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := c.Param("id")

	var file model.FileRecord
	query := ctrl.db.Where("id = ?", fileID)
	if !isUserAdmin(user) {
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	now := time.Now()
	ctrl.db.Model(&model.TemporaryLink{}).
		Where("file_id = ? AND revoked_at IS NULL", file.ID).
		Update("revoked_at", &now)

	ctrl.db.Model(&file).Update("sign_version", gorm.Expr("sign_version + 1"))
	c.JSON(http.StatusOK, gin.H{"message": "All previous temporary links have been revoked"})
}

func (ctrl *FileController) EditFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := ParseUintSafe(c.Param("id"), 0)
	if fileID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	var req struct {
		ImageData   string `json:"image_data" binding:"required"`
		Mode        string `json:"mode"` // "overwrite" or "copy"
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	b64Data := req.ImageData
	if idx := strings.Index(b64Data, ","); idx != -1 {
		b64Data = b64Data[idx+1:]
	}

	rawBytes, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid base64 image data"})
		return
	}

	mode := req.Mode
	if mode != "copy" {
		mode = "overwrite"
	}

	savedFile, err := ctrl.uploadService.SaveEditedFile(c.Request.Context(), user, fileID, rawBytes, mode, req.DisplayName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"file":  savedFile,
		"links": ctrl.buildFileLinks(savedFile),
	})
}
