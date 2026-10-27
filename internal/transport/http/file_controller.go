package http

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
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
	chunkService    *service.ChunkService
	baseURL         string
	derivedCache    *cache.DerivedCache
}

func NewFileController(
	db *gorm.DB,
	us *service.UploadService,
	ds *service.DispatchService,
	is *service.ImportService,
	cs *service.ChunkService,
	baseURL string,
	dc *cache.DerivedCache,
) *FileController {
	return &FileController{
		db:              db,
		uploadService:   us,
		dispatchService: ds,
		importService:   is,
		chunkService:    cs,
		baseURL:         baseURL,
		derivedCache:    dc,
	}
}

func (ctrl *FileController) SetDB(db *gorm.DB) {
	ctrl.db = db
}

func (ctrl *FileController) SetBaseURL(baseURL string) {
	ctrl.baseURL = baseURL
}

func (ctrl *FileController) resolveBaseURL(c *gin.Context) string {
	if c != nil {
		if orig := c.Request.Header.Get("Origin"); orig != "" {
			return strings.TrimRight(orig, "/")
		}
		host := c.Request.Host
		if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
			host = xfh
		}
		if host != "" {
			proto := "http"
			if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
				proto = "https"
			}
			return fmt.Sprintf("%s://%s", proto, host)
		}
	}

	if ctrl.db != nil {
		var s model.Setting
		if err := ctrl.db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			urls := strings.FieldsFunc(s.Value, func(r rune) bool {
				return r == '\n' || r == '\r' || r == ',' || r == ';'
			})
			if len(urls) > 0 && strings.TrimSpace(urls[0]) != "" {
				return strings.TrimRight(strings.TrimSpace(urls[0]), "/")
			}
		}
	}

	if ctrl.baseURL != "" {
		urls := strings.FieldsFunc(ctrl.baseURL, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		})
		if len(urls) > 0 && strings.TrimSpace(urls[0]) != "" {
			return strings.TrimRight(strings.TrimSpace(urls[0]), "/")
		}
	}
	return "http://localhost:2983"
}

func (ctrl *FileController) getBaseURLs() []string {
	var raw string
	if ctrl.db != nil {
		var s model.Setting
		if err := ctrl.db.Where("key = 'base_url'").First(&s).Error; err == nil && s.Value != "" {
			raw = s.Value
		}
	}
	if raw == "" {
		raw = ctrl.baseURL
	}
	var result []string
	if raw != "" {
		fields := strings.FieldsFunc(raw, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		})
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if f != "" {
				result = append(result, strings.TrimRight(f, "/"))
			}
		}
	}
	return result
}

func (ctrl *FileController) RegisterRoutes(rg *gin.RouterGroup) {
	files := rg.Group("/files")
	files.Use(auth.RequireAuth())
	{
		files.GET("", auth.RequirePermission("file.read"), ctrl.ListFiles)
		files.GET("/ids", auth.RequirePermission("file.read"), ctrl.ListFileIDs)
		files.GET("/:id", auth.RequirePermission("file.read"), ctrl.GetFile)
		files.PATCH("/:id", auth.RequirePermission("file.write"), ctrl.UpdateFile)
		files.DELETE("/:id", auth.RequirePermission("file.delete"), ctrl.DeleteFile)
		files.POST("/batch-delete", auth.RequirePermission("file.delete"), ctrl.BatchDeleteFiles)
		files.POST("/batch-update", auth.RequirePermission("file.write"), ctrl.BatchUpdateFiles)
		files.GET("/:id/raw", auth.RequirePermission("file.read"), ctrl.DownloadRaw)
		files.GET("/:id/temp-links", auth.RequirePermission("file.read"), ctrl.ListTempLinks)
		files.POST("/:id/temp-links", auth.RequirePermission("file.write"), ctrl.CreateTempLink)
		files.POST("/:id/temp-link", auth.RequirePermission("file.write"), ctrl.CreateTempLink)
		files.DELETE("/:id/temp-links/:link_id", auth.RequirePermission("file.write"), ctrl.RevokeTempLink)
		files.DELETE("/:id/temp-links", auth.RequirePermission("file.write"), ctrl.RevokeAllTempLinks)
		files.POST("/:id/revoke-links", auth.RequirePermission("file.write"), ctrl.RevokeAllTempLinks)
		files.POST("/:id/edit", auth.RequirePermission("file.write"), ctrl.EditFile)
	}

	rg.POST("/upload", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.Upload)
	rg.POST("/upload/chunk/init", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.InitChunkUpload)
	rg.POST("/upload/chunk/part", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.UploadChunkPart)
	rg.GET("/upload/chunk/status", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.GetChunkStatus)
	rg.POST("/upload/chunk/complete", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.CompleteChunkUpload)
	rg.POST("/upload/chunk/abort", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.AbortChunkUpload)
	rg.DELETE("/upload/chunk/:upload_id", auth.RequireAuth(), auth.RequirePermission("file.upload"), ctrl.AbortChunkUpload)
	rg.POST("/import/archive", auth.RequireAuth(), auth.RequirePermission("import.use"), ctrl.ImportArchive)
	rg.POST("/import/zip", auth.RequireAuth(), auth.RequirePermission("import.use"), ctrl.ImportArchive)
}

func (ctrl *FileController) SetChunkService(cs *service.ChunkService) {
	ctrl.chunkService = cs
}

func (ctrl *FileController) getMaxUploadSize() int64 {
	var s model.Setting
	if err := ctrl.db.Where("key = ?", "max_single_file_size").First(&s).Error; err == nil {
		if mb, err := strconv.ParseInt(strings.TrimSpace(s.Value), 10, 64); err == nil {
			if mb <= 0 {
				return 0 // 0 means unlimited
			}
			return mb * 1024 * 1024
		}
	}
	return 50 * 1024 * 1024
}

func isValidMode(mode string) bool {
	return mode == "inherit" || mode == "on" || mode == "off"
}

func (ctrl *FileController) Upload(c *gin.Context) {
	maxSize := ctrl.getMaxUploadSize()
	if maxSize > 0 {
		// Add 50MB tolerance for multipart form boundaries
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize+50*1024*1024)
	}

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
	if !isValidMode(watermarkMode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark_mode, must be inherit, on, or off"})
		return
	}
	displayName := c.PostForm("display_name")
	if displayName == "" {
		displayName = fileHeader.Filename
	}

	var wmTemplateID *uint
	if tidStr := c.PostForm("watermark_template_id"); tidStr != "" {
		val := ParseUintSafe(tidStr, 0)
		if val > 0 {
			var count int64
			ctrl.db.Model(&model.WatermarkTemplate{}).
				Where("id = ? AND (owner_id IS NULL OR owner_id = ?)", val, user.ID).
				Count(&count)
			if count == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark template ID or permission denied"})
				return
			}
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
	currentBaseURL := ctrl.resolveBaseURL(c)
	links := ctrl.buildFileLinks(record, currentBaseURL)

	c.JSON(http.StatusOK, gin.H{
		"file":  record,
		"links": links,
	})
}

type InitChunkUploadRequest struct {
	Filename            string `json:"filename" binding:"required"`
	TotalSize           int64  `json:"total_size" binding:"required"`
	ChunkSize           int64  `json:"chunk_size" binding:"required"`
	TotalChunks         int    `json:"total_chunks" binding:"required"`
	FileSHA256          string `json:"file_sha256"`
	UploadType          string `json:"upload_type"` // "file" or "archive"
	PolicyID            uint   `json:"policy_id"`
	WatermarkMode       string `json:"watermark_mode"`
	WatermarkTemplateID *uint  `json:"watermark_template_id"`
	HotlinkMode         string `json:"hotlink_mode"`
	PreservePath        bool   `json:"preserve_path"`
	StripTopDir         bool   `json:"strip_top_dir"`
	ConflictAction      string `json:"conflict_action"`
}

func (ctrl *FileController) InitChunkUpload(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req InitChunkUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.UploadType == "" {
		req.UploadType = "file"
	}
	if req.WatermarkMode == "" {
		req.WatermarkMode = "inherit"
	}
	if req.HotlinkMode == "" {
		req.HotlinkMode = "inherit"
	}
	if req.ConflictAction == "" {
		req.ConflictAction = "skip"
	}

	meta, uploaded, err := ctrl.chunkService.InitChunkUpload(user, service.ChunkUploadMeta{
		Filename:            req.Filename,
		TotalSize:           req.TotalSize,
		ChunkSize:           req.ChunkSize,
		TotalChunks:         req.TotalChunks,
		FileSHA256:          strings.ToLower(strings.TrimSpace(req.FileSHA256)),
		UploadType:          req.UploadType,
		PolicyID:            req.PolicyID,
		WatermarkMode:       req.WatermarkMode,
		WatermarkTemplateID: req.WatermarkTemplateID,
		HotlinkMode:         req.HotlinkMode,
		PreservePath:        req.PreservePath,
		StripTopDir:         req.StripTopDir,
		ConflictAction:      req.ConflictAction,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_id":       meta.UploadID,
		"total_chunks":    meta.TotalChunks,
		"chunk_size":      meta.ChunkSize,
		"uploaded_chunks": uploaded,
	})
}

func (ctrl *FileController) UploadChunkPart(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	uploadID := c.PostForm("upload_id")
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload_id is required"})
		return
	}

	chunkIndexStr := c.PostForm("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk_index"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required"})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded chunk"})
		return
	}
	defer src.Close()

	if err := ctrl.chunkService.SaveChunkPart(user, uploadID, chunkIndex, src); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"upload_id":   uploadID,
		"chunk_index": chunkIndex,
	})
}

func (ctrl *FileController) GetChunkStatus(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	uploadID := c.Query("upload_id")
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload_id is required"})
		return
	}

	meta, uploaded, err := ctrl.chunkService.GetChunkStatus(user, uploadID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_id":       meta.UploadID,
		"filename":        meta.Filename,
		"total_chunks":    meta.TotalChunks,
		"chunk_size":      meta.ChunkSize,
		"uploaded_chunks": uploaded,
	})
}

func (ctrl *FileController) CompleteChunkUpload(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		UploadID string `json:"upload_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	record, archiveResult, err := ctrl.chunkService.CompleteChunkUpload(c.Request.Context(), user, req.UploadID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if archiveResult != nil {
		c.JSON(http.StatusOK, gin.H{
			"total":    archiveResult.Total,
			"imported": archiveResult.Imported,
			"skipped":  archiveResult.Skipped,
		})
		return
	}

	if record != nil {
		currentBaseURL := ctrl.resolveBaseURL(c)
		links := ctrl.buildFileLinks(record, currentBaseURL)
		c.JSON(http.StatusOK, gin.H{
			"file":  record,
			"links": links,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (ctrl *FileController) AbortChunkUpload(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		UploadID string `json:"upload_id"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.UploadID == "" {
		req.UploadID = c.Param("upload_id")
	}
	if req.UploadID == "" {
		req.UploadID = c.PostForm("upload_id")
	}
	if req.UploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload_id is required"})
		return
	}

	if err := ctrl.chunkService.AbortChunkUpload(user, req.UploadID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (ctrl *FileController) buildFileLinks(file *model.FileRecord, customBaseURL ...string) gin.H {
	baseURL := ctrl.baseURL
	if len(customBaseURL) > 0 && customBaseURL[0] != "" {
		baseURL = customBaseURL[0]
	} else if ctrl.baseURL != "" {
		urls := strings.FieldsFunc(ctrl.baseURL, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		})
		if len(urls) > 0 {
			baseURL = strings.TrimRight(strings.TrimSpace(urls[0]), "/")
		}
	}
	if baseURL == "" {
		baseURL = "http://localhost:2983"
	}

	directURL := fmt.Sprintf("%s/%s", baseURL, file.PublicPath)
	safeHTMLName := html.EscapeString(file.DisplayName)
	safeMdName := strings.ReplaceAll(strings.ReplaceAll(file.DisplayName, "[", "\\["), "]", "\\]")
	links := gin.H{
		"url":       directURL,
		"markdown":  fmt.Sprintf("![%s](%s)", safeMdName, directURL),
		"html":      fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", directURL, safeHTMLName),
		"bbcode":    fmt.Sprintf("[img]%s[/img]", directURL),
		"thumb_url": fmt.Sprintf("%s?s=thumb", directURL),
	}

	if file.IsImage {
		ext := filepath.Ext(file.PublicPath)
		baseNoExt := strings.TrimSuffix(file.PublicPath, ext)
		webpURL := fmt.Sprintf("%s/%s.webp", baseURL, baseNoExt)
		links["webp_url"] = webpURL
		links["webp_markdown"] = fmt.Sprintf("![%s](%s)", safeMdName, webpURL)
		links["webp_html"] = fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", webpURL, safeHTMLName)
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
		case "ico":
			query = query.Where("mime_type IN ('image/x-icon', 'image/vnd.microsoft.icon') OR public_path LIKE '%.ico'")
		case "other":
			query = query.Where("is_image = 0")
		}
	}

	if policyID := c.Query("policy_id"); policyID != "" && policyID != "all" {
		query = query.Where("policy_id = ?", policyID)
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

	query := ctrl.db.Model(&model.FileRecord{}).Where("user_id = ?", user.ID)
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
		"base_url":  ctrl.resolveBaseURL(c),
		"base_urls": ctrl.getBaseURLs(),
	})
}

func (ctrl *FileController) ListFileIDs(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	query := ctrl.db.Model(&model.FileRecord{}).Where("user_id = ?", user.ID)
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
	query := ctrl.db.Where("id = ? AND user_id = ?", fileID, user.ID)

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
	query := ctrl.db.Where("id = ? AND user_id = ?", fileID, user.ID)
	if err := query.First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var req struct {
		DisplayName           *string `json:"display_name"`
		WatermarkMode         *string `json:"watermark_mode"`
		WatermarkTemplateID   *uint   `json:"watermark_template_id"`
		HotlinkMode           *string `json:"hotlink_mode"`
		HotlinkAllowedDomains *string `json:"hotlink_allowed_domains"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.DisplayName != nil && strings.TrimSpace(*req.DisplayName) != "" {
		updates["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	if req.WatermarkMode != nil && *req.WatermarkMode != "" {
		if !isValidMode(*req.WatermarkMode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark_mode, must be inherit, on, or off"})
			return
		}
		updates["watermark_mode"] = *req.WatermarkMode
	}
	if req.WatermarkTemplateID != nil {
		if *req.WatermarkTemplateID > 0 {
			var count int64
			ctrl.db.Model(&model.WatermarkTemplate{}).
				Where("id = ? AND (owner_id IS NULL OR owner_id = ?)", *req.WatermarkTemplateID, user.ID).
				Count(&count)
			if count == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark template ID or permission denied"})
				return
			}
			updates["watermark_template_id"] = req.WatermarkTemplateID
		} else {
			updates["watermark_template_id"] = nil
		}
	}
	if req.HotlinkMode != nil && *req.HotlinkMode != "" {
		if !isValidMode(*req.HotlinkMode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hotlink_mode, must be inherit, on, or off"})
			return
		}
		updates["hotlink_mode"] = *req.HotlinkMode
	}
	if req.HotlinkAllowedDomains != nil {
		updates["hotlink_allowed_domains"] = *req.HotlinkAllowedDomains
	}

	if (req.WatermarkMode != nil && *req.WatermarkMode != "") || req.WatermarkTemplateID != nil {
		updates["sign_version"] = gorm.Expr("sign_version + 1")
	}

	ctrl.db.Model(&file).Updates(updates)
	ctrl.db.Where("id = ?", file.ID).First(&file)
	c.JSON(http.StatusOK, gin.H{"message": "File updated successfully", "file": file, "links": ctrl.buildFileLinks(&file)})
}

func (ctrl *FileController) DeleteFile(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	fileID := ParseUintSafe(c.Param("id"), 0)

	err := ctrl.uploadService.DeleteFile(c.Request.Context(), fileID, user.ID, false)
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
		if err := ctrl.uploadService.DeleteFile(c.Request.Context(), id, user.ID, false); err == nil {
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
	if req.WatermarkMode != nil && *req.WatermarkMode != "" {
		if !isValidMode(*req.WatermarkMode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark_mode, must be inherit, on, or off"})
			return
		}
		updates["watermark_mode"] = *req.WatermarkMode
	}
	if req.WatermarkTemplateID != nil {
		if *req.WatermarkTemplateID > 0 {
			var count int64
			ctrl.db.Model(&model.WatermarkTemplate{}).
				Where("id = ? AND (owner_id IS NULL OR owner_id = ?)", *req.WatermarkTemplateID, user.ID).
				Count(&count)
			if count == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark template ID or permission denied"})
				return
			}
			updates["watermark_template_id"] = *req.WatermarkTemplateID
		} else {
			updates["watermark_template_id"] = nil
		}
	}
	if req.HotlinkMode != nil && *req.HotlinkMode != "" {
		if !isValidMode(*req.HotlinkMode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hotlink_mode, must be inherit, on, or off"})
			return
		}
		updates["hotlink_mode"] = *req.HotlinkMode
	}
	if req.WatermarkMode != nil || req.WatermarkTemplateID != nil {
		updates["sign_version"] = gorm.Expr("sign_version + 1")
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
	// Support up to 100GB archive streaming upload
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 100*1024*1024*1024)

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp file: " + err.Error()})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	src, err := fileHeader.Open()
	if err != nil {
		_ = tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded archive: " + err.Error()})
		return
	}
	defer src.Close()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded archive: " + err.Error()})
		return
	}
	_ = tmp.Close()

	preservePath := c.PostForm("preserve_path") == "true" || c.PostForm("preserve_path") == "1"
	stripTopDir := c.PostForm("strip_top_dir") == "true" || c.PostForm("strip_top_dir") == "1"
	conflictAction := c.DefaultPostForm("conflict_action", "skip")
	policyID := ParseUintSafe(c.PostForm("policy_id"), 0)
	watermarkMode := c.DefaultPostForm("watermark_mode", "inherit")
	hotlinkMode := c.DefaultPostForm("hotlink_mode", "inherit")
	if !isValidMode(watermarkMode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark_mode, must be inherit, on, or off"})
		return
	}
	if !isValidMode(hotlinkMode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hotlink_mode, must be inherit, on, or off"})
		return
	}

	var wmTemplateID *uint
	if tidStr := c.PostForm("watermark_template_id"); tidStr != "" {
		val := ParseUintSafe(tidStr, 0)
		if val > 0 {
			wmTemplateID = &val
		}
	}

	total, imported, skipped, err := ctrl.importService.ImportArchive(c.Request.Context(), user, tmpPath, fileHeader.Filename, policyID, service.ImportOptions{
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

	currentBaseURL := ctrl.resolveBaseURL(c)
	fullURL := fmt.Sprintf("%s/%s?token=%s", currentBaseURL, file.PublicPath, token)

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
	currentBaseURL := ctrl.resolveBaseURL(c)
	items := make([]TempLinkItem, 0, len(links))
	for _, l := range links {
		isExpired := l.ExpiresAt.Before(now)
		isRevoked := l.RevokedAt != nil
		items = append(items, TempLinkItem{
			ID:        l.ID,
			Token:     l.Token,
			URL:       fmt.Sprintf("%s/%s?token=%s", currentBaseURL, file.PublicPath, l.Token),
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
	maxSize := ctrl.getMaxUploadSize()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize*2)

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
	if int64(len(rawBytes)) > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image payload exceeds maximum allowed file size"})
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
