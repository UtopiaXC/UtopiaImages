package http

import (
	"context"
	"encoding/base64"
	"errors"
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
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type FileController struct {
	db              *gorm.DB
	uploadService   *service.UploadService
	dispatchService *service.DispatchService
	importService   *service.ImportService
	chunkService    *service.ChunkService
	driverManager   *storage.PolicyDriverManager
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

func (ctrl *FileController) SetDriverManager(dm *storage.PolicyDriverManager) {
	ctrl.driverManager = dm
}

func (ctrl *FileController) SetDB(db *gorm.DB) {
	ctrl.db = db
}

func (ctrl *FileController) SetBaseURL(baseURL string) {
	ctrl.baseURL = baseURL
}

func (ctrl *FileController) resolveBaseURL(c *gin.Context) string {
	return ResolveBaseURL(c, ctrl.db, ctrl.baseURL)
}

func (ctrl *FileController) getBaseURLs() []string {
	return GetConfiguredBaseURLs(nil, ctrl.db, ctrl.baseURL)
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
		files.POST("/:id/policies", auth.RequirePermission("file.write"), ctrl.SyncFilePolicy)
		files.POST("/batch-sync-policy", auth.RequirePermission("file.write"), ctrl.BatchSyncFilePolicy)
		files.DELETE("/:id/policies/:policy_id", auth.RequirePermission("file.delete"), ctrl.RemoveFilePolicy)
		files.POST("/batch-remove-policy", auth.RequirePermission("file.delete"), ctrl.BatchRemoveFilePolicy)
		files.POST("/:id/edit", auth.RequirePermission("file.write"), ctrl.EditFile)
	}

	rg.POST("/guest/upload", ctrl.GuestUpload)
	rg.GET("/guest/files", ctrl.ListGuestFiles)
	rg.GET("/guest/gallery", ctrl.ListGuestFiles)
	rg.GET("/guest/gallery/:id", ctrl.GetGuestFileDetails)
	rg.GET("/guest/files/:id", ctrl.GetGuestFileDetails)
	rg.GET("/square/files", ctrl.ListSquareFiles)
	rg.GET("/square/files/:id", ctrl.GetSquareFileDetails)
	rg.GET("/public/files/:id", ctrl.GetPublicFileDetails)

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

func (ctrl *FileController) checkRoleUploadLimits(user *model.User, filename string, size int64) error {
	if user == nil {
		return errors.New("用户未找到")
	}
	var role model.Role
	if user.Role != nil {
		role = *user.Role
	} else if err := ctrl.db.Where("id = ?", user.RoleID).First(&role).Error; err != nil {
		return nil
	}

	// 1. Format check
	allowed := strings.TrimSpace(role.AllowedFormats)
	if allowed != "" && allowed != "*" {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
		allowedList := strings.Split(allowed, ",")
		matched := false
		for _, a := range allowedList {
			a = strings.ToLower(strings.TrimSpace(a))
			if a == ext {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("当前角色不允许上传 .%s 格式的文件，仅支持: %s", ext, allowed)
		}
	}

	// 2. System single file size check
	sysMaxMB := int64(0)
	var s model.Setting
	if err := ctrl.db.Where("key = 'max_single_file_size'").First(&s).Error; err == nil {
		sysMaxMB, _ = strconv.ParseInt(strings.TrimSpace(s.Value), 10, 64)
	}

	if sysMaxMB > 0 && size > sysMaxMB*1024*1024 {
		return fmt.Errorf("文件大小超过系统上限: %d MB", sysMaxMB)
	}

	return nil
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

	if err := ctrl.checkRoleUploadLimits(user, fileHeader.Filename, fileHeader.Size); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var policyIDs []uint
	if pidsRaw := c.PostForm("policy_ids"); pidsRaw != "" {
		for _, part := range strings.Split(pidsRaw, ",") {
			if id := ParseUintSafe(strings.TrimSpace(part), 0); id > 0 {
				policyIDs = append(policyIDs, id)
			}
		}
	}
	if len(policyIDs) == 0 {
		for _, v := range c.Request.PostForm["policy_ids[]"] {
			if id := ParseUintSafe(strings.TrimSpace(v), 0); id > 0 {
				policyIDs = append(policyIDs, id)
			}
		}
	}
	policyID := ParseUintSafe(c.PostForm("policy_id"), 0)
	if len(policyIDs) == 0 && policyID > 0 {
		policyIDs = append(policyIDs, policyID)
	}

	watermarkMode := c.DefaultPostForm("watermark_mode", "inherit")
	if !isValidMode(watermarkMode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark_mode, must be inherit, on, or off"})
		return
	}
	publicMode := c.DefaultPostForm("public_mode", "inherit")
	if publicMode == "on" {
		publicMode = "public"
	} else if publicMode == "off" {
		publicMode = "private"
	}
	if publicMode != "inherit" && publicMode != "public" && publicMode != "private" {
		publicMode = "inherit"
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
		PolicyIDs:           policyIDs,
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
	PolicyIDs           []uint `json:"policy_ids,omitempty"`
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

	pIDs := req.PolicyIDs
	if len(pIDs) == 0 && req.PolicyID > 0 {
		pIDs = []uint{req.PolicyID}
	}

	meta, uploaded, err := ctrl.chunkService.InitChunkUpload(user, service.ChunkUploadMeta{
		Filename:            req.Filename,
		TotalSize:           req.TotalSize,
		ChunkSize:           req.ChunkSize,
		TotalChunks:         req.TotalChunks,
		FileSHA256:          strings.ToLower(strings.TrimSpace(req.FileSHA256)),
		UploadType:          req.UploadType,
		PolicyID:            req.PolicyID,
		PolicyIDs:           pIDs,
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
	cleanPath := strings.TrimPrefix(file.PublicPath, "/")
	var baseURL string
	if len(customBaseURL) > 0 && customBaseURL[0] != "" {
		baseURL = strings.TrimRight(customBaseURL[0], "/")
	} else {
		baseURL = ctrl.resolveBaseURL(nil)
	}

	var directURL, webpURL string
	if baseURL != "" {
		directURL = fmt.Sprintf("%s/%s", baseURL, cleanPath)
	} else {
		directURL = fmt.Sprintf("/%s", cleanPath)
	}

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
		ext := filepath.Ext(cleanPath)
		baseNoExt := strings.TrimSuffix(cleanPath, ext)
		if baseURL != "" {
			webpURL = fmt.Sprintf("%s/%s.webp", baseURL, baseNoExt)
		} else {
			webpURL = fmt.Sprintf("/%s.webp", baseNoExt)
		}
		links["webp_url"] = webpURL
		links["webp_markdown"] = fmt.Sprintf("![%s](%s)", safeMdName, webpURL)
		links["webp_html"] = fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", webpURL, safeHTMLName)
		links["webp_bbcode"] = fmt.Sprintf("[img]%s[/img]", webpURL)
	}

	type replicaLinkItem struct {
		ID         uint   `json:"id"`
		PolicyID   uint   `json:"policy_id"`
		PolicyName string `json:"policy_name"`
		PublicPath string `json:"public_path"`
		URL        string `json:"url"`
		DirectURL  string `json:"direct_url,omitempty"`
		Status     string `json:"status"`
	}

	var replicas []*model.FilePolicyReplica
	if len(file.Replicas) > 0 {
		replicas = file.Replicas
	} else if ctrl.db != nil && file.ID > 0 {
		ctrl.db.Preload("Policy").Where("file_id = ?", file.ID).Find(&replicas)
	}

	if len(replicas) > 0 {
		policyIDs := make([]uint, 0, len(replicas))
		for _, r := range replicas {
			policyIDs = append(policyIDs, r.PolicyID)
		}
		var policies []model.StoragePolicy
		policyMap := make(map[uint]model.StoragePolicy)
		if len(policyIDs) > 0 && ctrl.db != nil {
			ctrl.db.Where("id IN (?)", policyIDs).Find(&policies)
			for _, p := range policies {
				policyMap[p.ID] = p
			}
		}

		replicaItems := make([]replicaLinkItem, 0, len(replicas))
		for _, r := range replicas {
			pName := ""
			if r.Policy != nil && r.Policy.Name != "" {
				pName = r.Policy.Name
			} else if p, ok := policyMap[r.PolicyID]; ok {
				pName = p.Name
			}
			rPath := strings.TrimPrefix(r.PublicPath, "/")
			rURL := "/" + rPath
			if baseURL != "" {
				rURL = baseURL + "/" + rPath
			}

			dLink := ""
			if ctrl.uploadService != nil && r.Status == "ready" {
				if drv, dErr := ctrl.uploadService.GetDriver(context.Background(), r.PolicyID); dErr == nil {
					if dlp, ok := drv.(storage.DirectLinkProvider); ok {
						if dl, ok := dlp.GetDirectURL(r.StorageKey); ok && dl != "" {
							dLink = dl
						}
					}
				}
			}

			replicaItems = append(replicaItems, replicaLinkItem{
				ID:         r.ID,
				PolicyID:   r.PolicyID,
				PolicyName: pName,
				PublicPath: r.PublicPath,
				URL:        rURL,
				DirectURL:  dLink,
				Status:     r.Status,
			})
		}
		links["replicas"] = replicaItems
	}

	return links
}

func (ctrl *FileController) applyFileFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	return ApplyAdvancedFileFilters(query, ctrl.db, c, false)
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

	if len(files) > 0 {
		fileIDs := make([]uint, len(files))
		for i, f := range files {
			fileIDs[i] = f.ID
		}
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
		callMap := make(map[uint]int64)
		for _, cr := range calls {
			callMap[cr.FileID] = cr.Cnt
		}

		type uvResult struct {
			FileID uint  `gorm:"column:file_id"`
			UV     int64 `gorm:"column:uv"`
		}
		var uvs []uvResult
		ctrl.db.Model(&model.ImageCallVisitor{}).
			Select("file_id, count(distinct visitor_id) as uv").
			Where("file_id IN (?)", fileIDs).
			Group("file_id").
			Scan(&uvs)
		uvMap := make(map[uint]int64)
		for _, ur := range uvs {
			uvMap[ur.FileID] = ur.UV
		}

		var allReplicas []model.FilePolicyReplica
		ctrl.db.Preload("Policy").Where("file_id IN (?)", fileIDs).Find(&allReplicas)
		repMap := make(map[uint][]*model.FilePolicyReplica)
		for i := range allReplicas {
			if allReplicas[i].Policy != nil {
				allReplicas[i].PolicyName = allReplicas[i].Policy.Name
			}
			repMap[allReplicas[i].FileID] = append(repMap[allReplicas[i].FileID], &allReplicas[i])
		}

		for i := range files {
			files[i].TotalCalls = callMap[files[i].ID]
			files[i].TotalUV = uvMap[files[i].ID]
			files[i].Replicas = repMap[files[i].ID]
		}
	}

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

	var replicas []*model.FilePolicyReplica
	ctrl.db.Preload("Policy").Where("file_id = ?", file.ID).Find(&replicas)
	for i := range replicas {
		if replicas[i].Policy != nil {
			replicas[i].PolicyName = replicas[i].Policy.Name
		}
	}
	file.Replicas = replicas

	links := ctrl.buildFileLinks(&file)
	var callStats []model.ImageCallStat
	ctrl.db.Where("file_id = ?", file.ID).Order("count DESC").Find(&callStats)
	var totalCalls int64
	for _, cs := range callStats {
		totalCalls += cs.Count
	}
	var totalUV int64
	ctrl.db.Model(&model.ImageCallVisitor{}).Where("file_id = ?", file.ID).Distinct("visitor_id").Count(&totalUV)
	file.TotalCalls = totalCalls
	file.TotalUV = totalUV

	c.JSON(http.StatusOK, gin.H{
		"file":        file,
		"links":       links,
		"call_stats":  callStats,
		"total_calls": totalCalls,
		"total_uv":    totalUV,
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
		PublicMode            *string `json:"public_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.DisplayName != nil && strings.TrimSpace(*req.DisplayName) != "" {
		updates["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	if req.PublicMode != nil && *req.PublicMode != "" {
		batchPm := *req.PublicMode
		if batchPm == "on" {
			batchPm = "public"
		} else if batchPm == "off" {
			batchPm = "private"
		}
		if batchPm == "inherit" || batchPm == "public" || batchPm == "private" {
			updates["public_mode"] = batchPm
		}
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
	if req.PublicMode != nil && *req.PublicMode != "" {
		pm := *req.PublicMode
		if pm == "on" {
			pm = "public"
		} else if pm == "off" {
			pm = "private"
		}
		if pm == "inherit" || pm == "public" || pm == "private" {
			updates["public_mode"] = pm
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
		PublicMode          *string `json:"public_mode"`
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
	if req.PublicMode != nil && *req.PublicMode != "" {
		pm := *req.PublicMode
		if pm == "on" {
			pm = "public"
		} else if pm == "off" {
			pm = "private"
		}
		if pm == "inherit" || pm == "public" || pm == "private" {
			updates["public_mode"] = pm
		}
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

	// Guest files do not support temporary original links
	var guestUser model.User
	if err := ctrl.db.Where("username = ?", "guest").First(&guestUser).Error; err == nil && guestUser.ID > 0 {
		if file.UserID == guestUser.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "游客图片不支持生成临时原图链接"})
			return
		}
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
	cleanPath := strings.TrimPrefix(file.PublicPath, "/")
	var fullURL string
	if currentBaseURL == "" {
		fullURL = fmt.Sprintf("/%s?token=%s", cleanPath, token)
	} else {
		fullURL = fmt.Sprintf("%s/%s?token=%s", currentBaseURL, cleanPath, token)
	}

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

	var guestUser model.User
	if err := ctrl.db.Where("username = ?", "guest").First(&guestUser).Error; err == nil && guestUser.ID > 0 {
		if file.UserID == guestUser.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "游客图片不支持临时原图链接"})
			return
		}
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
	cleanPath := strings.TrimPrefix(file.PublicPath, "/")
	items := make([]TempLinkItem, 0, len(links))
	for _, l := range links {
		isExpired := l.ExpiresAt.Before(now)
		isRevoked := l.RevokedAt != nil
		var linkURL string
		if currentBaseURL == "" {
			linkURL = fmt.Sprintf("/%s?token=%s", cleanPath, l.Token)
		} else {
			linkURL = fmt.Sprintf("%s/%s?token=%s", currentBaseURL, cleanPath, l.Token)
		}
		items = append(items, TempLinkItem{
			ID:        l.ID,
			Token:     l.Token,
			URL:       linkURL,
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

func (ctrl *FileController) GuestUpload(c *gin.Context) {
	var s model.Setting
	ctrl.db.Where("key = 'enable_guest_upload'").First(&s)
	if s.Value != "true" && s.Value != "1" {
		c.JSON(http.StatusForbidden, gin.H{"error": "游客上传功能未开启"})
		return
	}

	var guestUser model.User
	if err := ctrl.db.Preload("Role").Where("username = ?", "guest").First(&guestUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "游客账户不存在或尚未初始化"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的文件"})
		return
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fileHeader.Filename), "."))
	isImgExt := ext == "jpg" || ext == "jpeg" || ext == "png" || ext == "gif" || ext == "webp" || ext == "heif" || ext == "heic" || ext == "bmp" || ext == "svg"
	if !isImgExt {
		c.JSON(http.StatusBadRequest, gin.H{"error": "游客仅支持上传图片文件（jpg, png, gif, webp, heif 等）"})
		return
	}

	if err := ctrl.checkRoleUploadLimits(&guestUser, fileHeader.Filename, fileHeader.Size); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取上传文件"})
		return
	}
	defer src.Close()

	var policyID uint
	var rolePolicy model.RolePolicy
	if err := ctrl.db.Where("role_id = ?", guestUser.RoleID).First(&rolePolicy).Error; err == nil {
		policyID = rolePolicy.PolicyID
	}

	// Guest upload is always public
	publicMode := "public"

	wmMode := "off"
	var wmTemplateID *uint
	var guestWmSetting model.Setting
	if err := ctrl.db.Where("key = 'guest_watermark_id'").First(&guestWmSetting).Error; err == nil && guestWmSetting.Value != "" {
		if wID, err := strconv.ParseUint(guestWmSetting.Value, 10, 64); err == nil && wID > 0 {
			uID := uint(wID)
			wmMode = "on"
			wmTemplateID = &uID
		}
	}

	record, err := ctrl.uploadService.Upload(c.Request.Context(), &guestUser, src, fileHeader.Filename, fileHeader.Size, service.UploadOptions{
		PolicyID:            policyID,
		DisplayName:         fileHeader.Filename,
		WatermarkMode:       wmMode,
		WatermarkTemplateID: wmTemplateID,
		PublicMode:          publicMode,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	currentBaseURL := ctrl.resolveBaseURL(c)
	links := ctrl.buildFileLinks(record, currentBaseURL)

	c.JSON(http.StatusOK, gin.H{
		"file":  record,
		"links": links,
	})
}

func (ctrl *FileController) ListGuestFiles(c *gin.Context) {
	var s model.Setting
	ctrl.db.Where("key = 'enable_guest_upload'").First(&s)
	if s.Value != "true" && s.Value != "1" {
		c.JSON(http.StatusForbidden, gin.H{"error": "游客图库未开放"})
		return
	}

	var guestUser model.User
	if err := ctrl.db.Where("username = ?", "guest").First(&guestUser).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}, "total": 0})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "24"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}

	query := ctrl.db.Model(&model.FileRecord{}).Where("user_id = ?", guestUser.ID)
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		like := "%" + q + "%"
		query = query.Where("display_name LIKE ? OR public_path LIKE ?", like, like)
	}

	var total int64
	query.Count(&total)

	var files []model.FileRecord
	query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&files)

	currentBaseURL := ctrl.resolveBaseURL(c)
	type GuestFileItem struct {
		model.FileRecord
		Links gin.H `json:"links"`
	}
	items := make([]GuestFileItem, 0, len(files))
	for _, f := range files {
		links := ctrl.buildFileLinks(&f, currentBaseURL)
		items = append(items, GuestFileItem{
			FileRecord: f,
			Links:      links,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (ctrl *FileController) ListSquareFiles(c *gin.Context) {
	var s model.Setting
	ctrl.db.Where("key = 'enable_square'").First(&s)
	if s.Value != "true" && s.Value != "1" {
		c.JSON(http.StatusForbidden, gin.H{"error": "广场功能未开启"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}

	query := ctrl.db.Model(&model.FileRecord{}).
		Joins("LEFT JOIN users ON users.id = file_records.user_id").
		Where("file_records.is_image = ? AND (file_records.public_mode = 'public' OR (file_records.public_mode = 'inherit' AND users.default_public = ?))", true, true)

	if q := strings.TrimSpace(c.Query("q")); q != "" {
		like := "%" + q + "%"
		query = query.Where("file_records.display_name LIKE ?", like)
	}

	var total int64
	query.Count(&total)

	var files []model.FileRecord
	query.Order("file_records.id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&files)

	currentBaseURL := ctrl.resolveBaseURL(c)
	type SquareFileItem struct {
		model.FileRecord
		Links gin.H `json:"links"`
	}
	items := make([]SquareFileItem, 0, len(files))
	for _, f := range files {
		links := ctrl.buildFileLinks(&f, currentBaseURL)
		items = append(items, SquareFileItem{
			FileRecord: f,
			Links:      links,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (ctrl *FileController) GetGuestFileDetails(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	var s model.Setting
	ctrl.db.Where("key = 'enable_guest_upload'").First(&s)
	if s.Value != "true" && s.Value != "1" {
		c.JSON(http.StatusForbidden, gin.H{"error": "游客图库未开放"})
		return
	}

	var guestUser model.User
	if err := ctrl.db.Where("username = ?", "guest").First(&guestUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var file model.FileRecord
	if err := ctrl.db.Where("id = ? AND user_id = ?", id, guestUser.ID).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	currentBaseURL := ctrl.resolveBaseURL(c)
	links := ctrl.buildFileLinks(&file, currentBaseURL)
	c.JSON(http.StatusOK, gin.H{
		"file":  file,
		"links": links,
	})
}

func (ctrl *FileController) GetSquareFileDetails(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	var s model.Setting
	ctrl.db.Where("key = 'enable_square'").First(&s)
	if s.Value != "true" && s.Value != "1" {
		c.JSON(http.StatusForbidden, gin.H{"error": "广场功能未开启"})
		return
	}

	var file model.FileRecord
	if err := ctrl.db.Where("id = ?", id).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	isPublic := false
	if file.IsImage {
		if file.PublicMode == "public" {
			isPublic = true
		} else if file.PublicMode == "inherit" {
			var u model.User
			if err := ctrl.db.Where("id = ?", file.UserID).First(&u).Error; err == nil && u.DefaultPublic {
				isPublic = true
			}
		}
	}
	if !isPublic {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	currentBaseURL := ctrl.resolveBaseURL(c)
	links := ctrl.buildFileLinks(&file, currentBaseURL)
	c.JSON(http.StatusOK, gin.H{
		"file":  file,
		"links": links,
	})
}

func (ctrl *FileController) GetPublicFileDetails(c *gin.Context) {
	id := ParseUintSafe(c.Param("id"), 0)
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	var file model.FileRecord
	if err := ctrl.db.Where("id = ?", id).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	var guestUser model.User
	ctrl.db.Where("username = ?", "guest").First(&guestUser)
	isGuestFile := guestUser.ID > 0 && file.UserID == guestUser.ID

	isPublicFile := false
	if file.IsImage {
		if file.PublicMode == "public" {
			isPublicFile = true
		} else if file.PublicMode == "inherit" {
			var u model.User
			if err := ctrl.db.Where("id = ?", file.UserID).First(&u).Error; err == nil && u.DefaultPublic {
				isPublicFile = true
			}
		}
	}

	if !isGuestFile && !isPublicFile {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	currentBaseURL := ctrl.resolveBaseURL(c)
	links := ctrl.buildFileLinks(&file, currentBaseURL)
	c.JSON(http.StatusOK, gin.H{
		"file":  file,
		"links": links,
	})
}

func (ctrl *FileController) SyncFilePolicy(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	isAdmin := isUserAdmin(user)

	fileID := ParseUintSafe(c.Param("id"), 0)
	if fileID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件ID无效"})
		return
	}

	var req struct {
		PolicyIDs []uint `json:"policy_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	replicas, err := ctrl.uploadService.SyncFileToPolicies(c.Request.Context(), user, fileID, req.PolicyIDs, isAdmin)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var file model.FileRecord
	ctrl.db.First(&file, fileID)
	file.Replicas = replicas

	c.JSON(http.StatusOK, gin.H{
		"message":  "策略同步完成",
		"file":     file,
		"replicas": replicas,
		"links":    ctrl.buildFileLinks(&file),
	})
}

func (ctrl *FileController) BatchSyncFilePolicy(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	isAdmin := isUserAdmin(user)

	var req struct {
		FileIDs   []uint `json:"file_ids" binding:"required"`
		PolicyIDs []uint `json:"policy_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	successCount := 0
	failedCount := 0
	var errMessages []string

	for _, fID := range req.FileIDs {
		_, err := ctrl.uploadService.SyncFileToPolicies(c.Request.Context(), user, fID, req.PolicyIDs, isAdmin)
		if err != nil {
			failedCount++
			errMessages = append(errMessages, fmt.Sprintf("文件 #%d: %s", fID, err.Error()))
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":   len(req.FileIDs),
		"success": successCount,
		"failed":  failedCount,
		"errors":  errMessages,
	})
}

func (ctrl *FileController) RemoveFilePolicy(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	isAdmin := isUserAdmin(user)

	fileID := ParseUintSafe(c.Param("id"), 0)
	policyID := ParseUintSafe(c.Param("policy_id"), 0)
	if fileID == 0 || policyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效"})
		return
	}

	err := ctrl.uploadService.RemoveFileFromPolicy(c.Request.Context(), user, fileID, policyID, isAdmin)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "副本移除成功"})
}

func (ctrl *FileController) BatchRemoveFilePolicy(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	isAdmin := isUserAdmin(user)

	var req struct {
		FileIDs  []uint `json:"file_ids" binding:"required"`
		PolicyID uint   `json:"policy_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	successCount := 0
	failedCount := 0
	var errMessages []string

	for _, fID := range req.FileIDs {
		err := ctrl.uploadService.RemoveFileFromPolicy(c.Request.Context(), user, fID, req.PolicyID, isAdmin)
		if err != nil {
			failedCount++
			errMessages = append(errMessages, fmt.Sprintf("文件 #%d: %s", fID, err.Error()))
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":   len(req.FileIDs),
		"success": successCount,
		"failed":  failedCount,
		"errors":  errMessages,
	})
}
