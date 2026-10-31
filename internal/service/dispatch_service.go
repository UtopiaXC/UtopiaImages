package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/imaging"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/UtopiaXC/UtopiaImages/internal/pkg/pathclean"
	"github.com/UtopiaXC/UtopiaImages/internal/storage"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	glogger "gorm.io/gorm/logger"
)

type DispatchService struct {
	db        *gorm.DB
	storage   storage.StorageDriver
	processor imaging.ImageProcessor
	cache     *cache.DerivedCache
}

func NewDispatchService(
	db *gorm.DB,
	st storage.StorageDriver,
	proc imaging.ImageProcessor,
	dc *cache.DerivedCache,
) *DispatchService {
	return &DispatchService{
		db:        db,
		storage:   st,
		processor: proc,
		cache:     dc,
	}
}

func (s *DispatchService) SetDB(db *gorm.DB) {
	s.db = db
}

func (s *DispatchService) abortWithError(c *gin.Context, code int) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.AbortWithStatus(code)
}

// GetRealClientIP extracts the real client IP considering CDNs and proxies.
func GetRealClientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	headers := []string{
		"CF-Connecting-IP",
		"True-Client-IP",
		"X-Real-IP",
		"Ali-CDN-Real-IP",
		"Cdn-Src-Ip",
		"X-Client-IP",
		"Fastly-Client-IP",
	}
	for _, h := range headers {
		if val := strings.TrimSpace(c.GetHeader(h)); val != "" {
			return val
		}
	}
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if first != "" {
				return first
			}
		}
	}
	return c.ClientIP()
}

// GenerateVisitorID creates a unique client fingerprint based on IP and User-Agent
func GenerateVisitorID(c *gin.Context) string {
	ip := GetRealClientIP(c)
	ua := ""
	if c != nil && c.Request != nil {
		ua = c.Request.UserAgent()
	}
	h := sha256.Sum256([]byte(ip + "|" + ua))
	return hex.EncodeToString(h[:16])
}

// ServeFile handles client requests for public files
func (s *DispatchService) ServeFile(c *gin.Context) {
	s.handleFileRequest(c, strings.TrimPrefix(c.Request.URL.Path, "/"), false)
}

// ServeRawImage handles requests redirected to the raw image API endpoint
func (s *DispatchService) ServeRawImage(c *gin.Context, rawPath string) {
	s.handleFileRequest(c, rawPath, true)
}

func (s *DispatchService) handleFileRequest(c *gin.Context, rawPath string, isRawRedirect bool) {
	// 1. Sanitize request path
	publicPath := pathclean.CleanPath(rawPath)

	if publicPath == "" {
		s.abortWithError(c, http.StatusNotFound)
		return
	}

	var file model.FileRecord
	var requestedFormat string

	// 2. Query file record (direct lookup or dynamic extension lookup)
	if err := s.db.Where("public_path = ?", publicPath).First(&file).Error; err != nil {
		// Not matched directly, attempt dynamic format matching (e.g. photo.webp from photo.png, photo.heif)
		reqExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(publicPath), "."))
		supportedExts := []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".heif", ".heic"}
		isSupportedExt := false
		for _, ext := range supportedExts {
			if strings.TrimPrefix(ext, ".") == reqExt {
				isSupportedExt = true
				break
			}
		}

		if isSupportedExt {
			baseWithoutExt := strings.TrimSuffix(publicPath, filepath.Ext(publicPath))
			candidatePaths := make([]string, 0, len(supportedExts))
			for _, ext := range supportedExts {
				candidatePaths = append(candidatePaths, baseWithoutExt+ext)
			}
			var candidateRecord model.FileRecord
			if err := s.db.Where("public_path IN ? AND is_image = 1", candidatePaths).First(&candidateRecord).Error; err == nil {
				file = candidateRecord
				requestedFormat = reqExt
			}
		}

		if file.ID == 0 {
			s.abortWithError(c, http.StatusNotFound)
			return
		}
	}

	// 3. Multi-level Hotlink Protection Check (File -> User -> System)
	if !s.checkReferer(c, &file) {
		s.abortWithError(c, http.StatusForbidden)
		return
	}

	// 4. Security headers
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Access-Control-Allow-Origin", "*")

	// 5. Check format conversion permission
	if requestedFormat != "" && !s.isFormatConversionEnabled() {
		origExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
		if requestedFormat != origExt {
			s.abortWithError(c, http.StatusNotFound)
			return
		}
	}

	// 6. Check if dynamic processing is requested
	reqW, _ := strconv.Atoi(c.Query("w"))
	reqH, _ := strconv.Atoi(c.Query("h"))
	reqStyle := c.Query("s")
	if reqStyle == "" {
		reqStyle = c.Query("preset")
	}
	if reqStyle == "" {
		reqStyle = c.Query("style")
	}
	if sizeParam := strings.ToLower(c.Query("size")); sizeParam != "" {
		switch sizeParam {
		case "large":
			reqW = 1920
		case "medium":
			reqW = 1280
		case "small":
			reqW = 640
		case "thumb", "thumbnail":
			reqW = 200
		}
	}
	if fmtQuery := c.Query("fmt"); fmtQuery != "" {
		requestedFormat = strings.ToLower(fmtQuery)
	}
	if formatQuery := c.Query("format"); formatQuery != "" {
		requestedFormat = strings.ToLower(formatQuery)
	}

	origExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
	targetFormat := origExt
	if requestedFormat != "" {
		targetFormat = requestedFormat
	}

	targetSizeLabel := "original"
	if reqStyle != "" {
		targetSizeLabel = reqStyle
	} else if sizeParam := c.Query("size"); sizeParam != "" {
		targetSizeLabel = strings.ToLower(sizeParam)
	} else if reqW > 0 || reqH > 0 {
		if reqW > 0 && reqH > 0 {
			targetSizeLabel = fmt.Sprintf("%dx%d", reqW, reqH)
		} else if reqW > 0 {
			targetSizeLabel = fmt.Sprintf("%dw", reqW)
		} else if reqH > 0 {
			targetSizeLabel = fmt.Sprintf("%dh", reqH)
		}
	}

	// 7. Check if request should be intercepted for Call Statistics
	if !isRawRedirect {
		isPrefetch := strings.EqualFold(c.GetHeader("Purpose"), "prefetch") ||
			strings.EqualFold(c.GetHeader("Sec-Purpose"), "prefetch") ||
			strings.EqualFold(c.GetHeader("Sec-Purpose"), "prerender") ||
			strings.EqualFold(c.GetHeader("X-Moz"), "prefetch") ||
			strings.EqualFold(c.GetHeader("X-Purpose"), "preview")

		referer := c.Request.Referer()
		isConsole := c.Query("from") == "console" ||
			c.Query("internal") == "1" ||
			strings.Contains(referer, "/console") ||
			c.Request.Method == http.MethodHead ||
			c.Request.Method == http.MethodOptions

		if !isConsole && s.isCallStatEnabled(file.UserID) {
			if !isPrefetch {
				s.recordCallStat(file.ID, targetFormat, targetSizeLabel, c)
			}

			redirectURL := "/api/v1/images/raw/" + strings.TrimPrefix(publicPath, "/")
			if c.Request.URL.RawQuery != "" {
				redirectURL += "?" + c.Request.URL.RawQuery
			}

			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Header("CDN-Cache-Control", "no-store")
			c.Header("Cloudflare-CDN-Cache-Control", "no-store")
			c.Redirect(http.StatusFound, redirectURL)
			return
		}
	}

	needsWatermark := file.WatermarkMode == "on"
	if file.WatermarkMode == "inherit" || file.WatermarkMode == "" {
		var userSetting model.Setting
		if err := s.db.Where("key = ?", fmt.Sprintf("user_wm_mode_%d", file.UserID)).First(&userSetting).Error; err == nil {
			needsWatermark = userSetting.Value == "on"
		}
	}

	// Check if temporary unwatermarked link is requested (via ?token=... or ?t=...)
	tempToken := c.Query("token")
	if tempToken == "" {
		tempToken = c.Query("t")
	}
	if tempToken != "" {
		var tlink model.TemporaryLink
		if err := s.db.Where("file_id = ? AND token = ? AND revoked_at IS NULL AND expires_at > ?", file.ID, tempToken, time.Now()).First(&tlink).Error; err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		needsWatermark = false
	}

	hasTransformParams := reqW > 0 || reqH > 0 || reqStyle != "" || (requestedFormat != "" && requestedFormat != origExt)

	// Vector images (.svg) and icon containers (.ico) or non-images should bypass raster processing
	if !file.IsImage || (!needsWatermark && !hasTransformParams) || origExt == "ico" || origExt == "svg" {
		s.serveDirect(c, file, isRawRedirect)
		return
	}

	// 8. Handle Image Processing with 2-Stage Caching
	s.serveProcessed(c, file, reqW, reqH, reqStyle, requestedFormat, needsWatermark, isRawRedirect)
}

func (s *DispatchService) isFormatConversionEnabled() bool {
	if s.db == nil {
		return true
	}
	var setting model.Setting
	if err := s.db.Where("key = 'enable_format_conversion'").First(&setting).Error; err == nil {
		return setting.Value != "false" && setting.Value != "0"
	}
	return true
}

func (s *DispatchService) isCallStatEnabled(userID uint) bool {
	if s.db == nil {
		return false
	}
	var globalSetting model.Setting
	if err := s.db.Where("key = 'global_call_stat'").First(&globalSetting).Error; err == nil {
		if globalSetting.Value == "true" {
			return true
		}
	}
	if userID == 0 {
		return false
	}
	var userSetting model.Setting
	if err := s.db.Where("key = ?", fmt.Sprintf("user_call_stat_%d", userID)).First(&userSetting).Error; err == nil {
		return userSetting.Value == "true"
	}
	return false
}

func (s *DispatchService) recordCallStat(fileID uint, format, size string, c *gin.Context) {
	if fileID == 0 || s.db == nil {
		return
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "original"
	}
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "" {
		size = "original"
	}

	realIP := GetRealClientIP(c)
	visitorID := GenerateVisitorID(c)

	go func() {
		defer func() {
			_ = recover()
		}()

		silentDB := s.db.Session(&gorm.Session{Logger: s.db.Logger.LogMode(glogger.Silent)})

		// 1. Update or create ImageCallStat for PV
		res := s.db.Model(&model.ImageCallStat{}).
			Where("file_id = ? AND format = ? AND size = ?", fileID, format, size).
			Update("count", gorm.Expr("count + 1"))
		if res.Error == nil && res.RowsAffected == 0 {
			stat := model.ImageCallStat{
				FileID: fileID,
				Format: format,
				Size:   size,
				Count:  1,
				UV:     0,
			}
			_ = silentDB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "file_id"}, {Name: "format"}, {Name: "size"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"count": gorm.Expr("image_call_stats.count + 1")}),
			}).Create(&stat).Error
		}

		// 2. Track unique visitor for UV
		var exists int64
		s.db.Model(&model.ImageCallVisitor{}).
			Where("file_id = ? AND format = ? AND size = ? AND visitor_id = ?", fileID, format, size, visitorID).
			Count(&exists)
		if exists == 0 {
			vis := model.ImageCallVisitor{
				FileID:    fileID,
				Format:    format,
				Size:      size,
				VisitorID: visitorID,
				IP:        realIP,
			}
			resVis := silentDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&vis)
			if resVis.Error == nil && resVis.RowsAffected > 0 {
				s.db.Model(&model.ImageCallStat{}).
					Where("file_id = ? AND format = ? AND size = ?", fileID, format, size).
					Update("uv", gorm.Expr("uv + 1"))
			}
		}
	}()
}

func shortHash(h string) string {
	if len(h) >= 16 {
		return h[:16]
	}
	return h
}

func (s *DispatchService) serveDirect(c *gin.Context, file model.FileRecord, isRawRedirect bool) {
	mimeType := file.MimeType
	origExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
	if origExt == "ico" && (mimeType == "" || mimeType == "application/octet-stream") {
		mimeType = "image/x-icon"
	} else if origExt == "svg" && (mimeType == "" || mimeType == "application/octet-stream") {
		mimeType = "image/svg+xml"
	}
	c.Header("Content-Type", mimeType)
	if isRawRedirect {
		c.Header("Cache-Control", "public, max-age=2592000, must-revalidate")
	} else {
		c.Header("Cache-Control", "public, no-cache, must-revalidate")
	}
	etag := fmt.Sprintf("\"%s-%d\"", shortHash(file.Sha256), file.SignVersion)
	c.Header("ETag", etag)
	c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
		c.Status(http.StatusNotModified)
		return
	}

	if localPath, ok := s.storage.GetLocalPath(file.StorageKey); ok {
		http.ServeFile(c.Writer, c.Request, localPath)
		return
	}

	r, err := s.storage.Get(c.Request.Context(), file.StorageKey)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer r.Close()

	if rs, ok := r.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, filepath.Base(file.PublicPath), file.UpdatedAt, rs)
	} else {
		_, _ = io.Copy(c.Writer, r)
	}
}

func (s *DispatchService) serveProcessed(
	c *gin.Context,
	file model.FileRecord,
	reqW, reqH int,
	style, format string,
	watermark bool,
	isRawRedirect bool,
) {
	// Find preset
	var presets []model.ImagePreset
	s.db.Find(&presets)
	matcher := imaging.NewPresetMatcher(presets)

	var targetPreset *model.ImagePreset
	if style != "" {
		targetPreset = matcher.FindByName(style)
	} else if reqW > 0 || reqH > 0 {
		targetPreset = matcher.MatchNearest(reqW, reqH)
	}

	targetW := 0
	targetH := 0
	targetFormat := format
	targetQuality := 85

	if targetPreset != nil {
		targetW = targetPreset.Width
		targetH = targetPreset.Height
		if targetFormat == "" {
			targetFormat = targetPreset.Format
		}
		targetQuality = targetPreset.Quality
	} else {
		targetW = reqW
		targetH = reqH
		// Quantize custom dimensions to steps of 32 and clamp to 8192 to prevent cache exhaustion
		if targetW > 0 {
			targetW = ((targetW + 15) / 32) * 32
			if targetW < 32 {
				targetW = 32
			} else if targetW > 8192 {
				targetW = 8192
			}
		}
		if targetH > 0 {
			targetH = ((targetH + 15) / 32) * 32
			if targetH < 32 {
				targetH = 32
			} else if targetH > 8192 {
				targetH = 8192
			}
		}
	}

	origExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
	if targetFormat == "" {
		targetFormat = origExt
	}
	if targetFormat == "" {
		targetFormat = "webp"
	}

	needsTransform := (targetW > 0 || targetH > 0) || (targetFormat != origExt)

	// Step 1: Resolve Base Image (with dynamic format conversion / resize cached)
	var baseBytes []byte
	var baseMime string
	var err error

	if needsTransform {
		baseCacheKey := fmt.Sprintf("base_%s_f%s_w%d_h%d_q%d", file.Sha256, targetFormat, targetW, targetH, targetQuality)
		baseCacheExt := "." + targetFormat

		if cachedPath, hit := s.cache.Get(baseCacheKey, baseCacheExt); hit {
			baseBytes, err = os.ReadFile(cachedPath)
		}

		if len(baseBytes) == 0 || err != nil {
			baseBytes, baseMime, err = s.cache.ComputeSingleFlight(baseCacheKey, func() ([]byte, string, error) {
				r, err := s.storage.Get(context.Background(), file.StorageKey)
				if err != nil {
					return nil, "", err
				}
				defer r.Close()

				opts := imaging.ProcessOptions{
					Width:   targetW,
					Height:  targetH,
					Format:  targetFormat,
					Quality: targetQuality,
				}
				return s.processor.Process(r, opts)
			})
			if err != nil {
				slog.Warn("Image transform failed, falling back to direct serving", "path", file.PublicPath, "error", err)
				s.serveDirect(c, file, isRawRedirect)
				return
			}
			s.cache.Put(baseCacheKey, baseCacheExt, baseBytes)
		}
	} else {
		// Read unconverted original directly
		r, err := s.storage.Get(context.Background(), file.StorageKey)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		defer r.Close()
		baseBytes, err = io.ReadAll(r)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		baseMime = file.MimeType
	}

	if baseMime == "" {
		baseMime = mimeFromFormat(targetFormat)
	}

	// If no watermark needed: serve base image directly
	if !watermark {
		etag := fmt.Sprintf("\"%s-%d-f%s-w%d-h%d-q%d-nowm\"", shortHash(file.Sha256), file.SignVersion, targetFormat, targetW, targetH, targetQuality)
		if isRawRedirect {
			c.Header("Cache-Control", "public, max-age=2592000, must-revalidate")
		} else {
			c.Header("Cache-Control", "public, no-cache, must-revalidate")
		}
		c.Header("ETag", etag)
		c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))
		if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
			c.Status(http.StatusNotModified)
			return
		}
		c.Header("Content-Type", baseMime)
		c.Data(http.StatusOK, baseMime, baseBytes)
		return
	}

	// Step 2: Watermark configuration
	trueVal := true
	wmConfig := &imaging.WatermarkConfig{
		Enabled:         true,
		Type:            "text",
		Text:            "Utopia Images",
		Layout:          "tile",
		Position:        "bottom-right",
		Opacity:         0.2,
		Angle:           -30,
		SpacingX:        260,
		SpacingY:        140,
		FontSize:        24,
		FontFamily:      "default",
		FontColor:       "#ffffff",
		StrokeColor:     "#000000",
		StrokeWidth:     1,
		AdaptiveScaling: &trueVal,
	}

	// Load template if assigned to file or user default
	tmplID := file.WatermarkTemplateID
	if tmplID == nil {
		var userTmplSetting model.Setting
		if err := s.db.Where("key = ?", fmt.Sprintf("user_wm_tmpl_%d", file.UserID)).First(&userTmplSetting).Error; err == nil && userTmplSetting.Value != "" {
			if idVal, err := strconv.ParseUint(userTmplSetting.Value, 10, 64); err == nil && idVal > 0 {
				uID := uint(idVal)
				tmplID = &uID
			}
		}
		// Fallback to first available template for this user or global template
		if tmplID == nil {
			var fallbackTmpl model.WatermarkTemplate
			if err := s.db.Where("owner_id = ? OR owner_id IS NULL", file.UserID).Order("id ASC").First(&fallbackTmpl).Error; err == nil {
				tmplID = &fallbackTmpl.ID
			}
		}
	}

	if tmplID != nil {
		var tmpl model.WatermarkTemplate
		if err := s.db.Where("id = ?", *tmplID).First(&tmpl).Error; err == nil && tmpl.Layers != "" {
			_ = json.Unmarshal([]byte(tmpl.Layers), wmConfig)
			wmConfig.Enabled = true
		}
	}

	// Force angle to 0 for position layout (must be straight, not slanted)
	if wmConfig.Layout == "position" {
		wmConfig.Angle = 0
	}

	// Check if watermark text contains dynamic elements: {...}
	isDynamic := strings.Contains(wmConfig.Text, "{") && strings.Contains(wmConfig.Text, "}")

	if isDynamic {
		// REQUIREMENT 6: Dynamic watermarks MUST NOT be cached on disk!
		wmConfig.Text = s.renderDynamicWatermarkText(wmConfig.Text, &file, c)

		watermarkedBytes, outMime, err := s.processor.Process(bytes.NewReader(baseBytes), imaging.ProcessOptions{
			Format:    targetFormat,
			Quality:   targetQuality,
			Watermark: wmConfig,
		})
		if err != nil {
			slog.Warn("Dynamic watermark processing failed, falling back to direct serving", "path", file.PublicPath, "error", err)
			s.serveDirect(c, file, isRawRedirect)
			return
		}

		c.Header("Cache-Control", "private, no-store")
		c.Header("Content-Type", outMime)
		c.Data(http.StatusOK, outMime, watermarkedBytes)
		return
	}

	// Static Watermark: Can be cached
	wmConfigJSON, _ := json.Marshal(wmConfig)
	wmParam := fmt.Sprintf("%s_%s_f%s_w%d_h%d_q%d", file.Sha256, shortHash(HashSHA256(wmConfigJSON)), targetFormat, targetW, targetH, targetQuality)
	wmCacheKey := s.cache.GenerateKey(file.PublicPath, wmParam, strconv.FormatInt(file.SignVersion, 10))
	wmCacheExt := "." + targetFormat

	etag := fmt.Sprintf("\"%s-%d-f%s-w%d-h%d-q%d-wm\"", shortHash(file.Sha256), file.SignVersion, targetFormat, targetW, targetH, targetQuality)
	if isRawRedirect {
		c.Header("Cache-Control", "public, max-age=2592000, must-revalidate")
	} else {
		c.Header("Cache-Control", "public, no-cache, must-revalidate")
	}
	c.Header("ETag", etag)
	c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
		c.Status(http.StatusNotModified)
		return
	}

	if cachedPath, hit := s.cache.Get(wmCacheKey, wmCacheExt); hit {
		http.ServeFile(c.Writer, c.Request, cachedPath)
		return
	}

	watermarkedBytes, outMime, err := s.cache.ComputeSingleFlight(wmCacheKey, func() ([]byte, string, error) {
		return s.processor.Process(bytes.NewReader(baseBytes), imaging.ProcessOptions{
			Format:    targetFormat,
			Quality:   targetQuality,
			Watermark: wmConfig,
		})
	})
	if err != nil {
		slog.Warn("Static watermark processing failed, falling back to direct serving", "path", file.PublicPath, "error", err)
		s.serveDirect(c, file, isRawRedirect)
		return
	}

	s.cache.Put(wmCacheKey, wmCacheExt, watermarkedBytes)
	c.Header("Content-Type", outMime)
	c.Data(http.StatusOK, outMime, watermarkedBytes)
}

func (s *DispatchService) renderDynamicWatermarkText(text string, file *model.FileRecord, c *gin.Context) string {
	now := time.Now()
	text = strings.ReplaceAll(text, "{now}", now.Format("2006-01-02 15:04:05"))
	text = strings.ReplaceAll(text, "{datetime}", now.Format("2006-01-02 15:04:05"))
	text = strings.ReplaceAll(text, "{year}", now.Format("2006"))
	text = strings.ReplaceAll(text, "{date}", now.Format("2006-01-02"))
	text = strings.ReplaceAll(text, "{time}", now.Format("15:04:05"))
	text = strings.ReplaceAll(text, "{ip}", c.ClientIP())
	text = strings.ReplaceAll(text, "{ext}", strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
	text = strings.ReplaceAll(text, "{dimensions}", fmt.Sprintf("%dx%d", file.Width, file.Height))
	text = strings.ReplaceAll(text, "{filesize}", formatBytesHelper(file.Size))
	text = strings.ReplaceAll(text, "{domain}", c.Request.Host)
	text = strings.ReplaceAll(text, "{copyright}", fmt.Sprintf("© %s %s", now.Format("2006"), c.Request.Host))
	text = strings.ReplaceAll(text, "{confidential}", "CONFIDENTIAL")
	text = strings.ReplaceAll(text, "{preview_only}", "FOR PREVIEW ONLY")

	if strings.Contains(text, "{owner}") {
		var u model.User
		if err := s.db.Select("username").Where("id = ?", file.UserID).First(&u).Error; err == nil {
			text = strings.ReplaceAll(text, "{owner}", u.Username)
		} else {
			text = strings.ReplaceAll(text, "{owner}", "admin")
		}
	}

	text = strings.ReplaceAll(text, "{filename}", file.DisplayName)
	return text
}

func mimeFromFormat(fmtStr string) string {
	switch strings.ToLower(fmtStr) {
	case "webp":
		return "image/webp"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "heif", "heic":
		return "image/heic"
	case "jpeg", "jpg":
		return "image/jpeg"
	default:
		return "image/jpeg"
	}
}

func formatBytesHelper(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (s *DispatchService) checkReferer(c *gin.Context, file *model.FileRecord) bool {
	// Level 1: Image (file) level check
	if file != nil && file.HotlinkMode == "off" {
		return true // Explicitly disabled for this file
	}

	var (
		hotlinkEnabled bool
		allowEmpty     = true
		allowedRules   []string
	)

	var user model.User
	if file != nil && file.UserID > 0 {
		_ = s.db.Where("id = ?", file.UserID).First(&user)
	}

	if file != nil && file.HotlinkMode == "on" {
		hotlinkEnabled = true
		if file.HotlinkAllowedDomains != "" {
			allowedRules = append(allowedRules, parseDomainRules(file.HotlinkAllowedDomains)...)
		}
		if user.HotlinkAllowedDomains != "" {
			allowedRules = append(allowedRules, parseDomainRules(user.HotlinkAllowedDomains)...)
		}
		allowEmpty = user.HotlinkAllowEmpty
	} else if user.HotlinkEnabled { // Level 2: User-level
		hotlinkEnabled = true
		allowEmpty = user.HotlinkAllowEmpty
		if user.HotlinkAllowedDomains != "" {
			allowedRules = append(allowedRules, parseDomainRules(user.HotlinkAllowedDomains)...)
		}
	} else { // Level 3: System-level fallback
		var enableSetting model.Setting
		if err := s.db.Where("key = 'enable_hotlink'").First(&enableSetting).Error; err == nil && enableSetting.Value == "true" {
			hotlinkEnabled = true
			var sysAllowEmpty model.Setting
			if err := s.db.Where("key = 'hotlink_allow_empty'").First(&sysAllowEmpty).Error; err == nil {
				allowEmpty = sysAllowEmpty.Value != "false" && sysAllowEmpty.Value != "0"
			}
		}
	}

	if !hotlinkEnabled {
		return true // Hotlink protection disabled
	}

	// Always merge system allowed domains
	var sysDomains model.Setting
	if err := s.db.Where("key = 'hotlink_allowed_domains'").First(&sysDomains).Error; err == nil && sysDomains.Value != "" {
		allowedRules = append(allowedRules, parseDomainRules(sysDomains.Value)...)
	}

	referer := c.Request.Referer()
	if referer == "" {
		return allowEmpty
	}

	u, err := url.Parse(referer)
	if err != nil {
		return false
	}

	refHost := strings.ToLower(u.Hostname())
	currentHost := strings.ToLower(c.Request.Host)
	if xfh := c.Request.Header.Get("X-Forwarded-Host"); xfh != "" {
		currentHost = strings.ToLower(xfh)
	}
	if idx := strings.Index(currentHost, ":"); idx != -1 {
		currentHost = currentHost[:idx]
	}

	// Always allow requests from the same host, localhost, or base_url
	if refHost == currentHost || refHost == "localhost" || refHost == "127.0.0.1" {
		return true
	}

	var baseUrlSetting model.Setting
	if err := s.db.Where("key = 'base_url'").First(&baseUrlSetting).Error; err == nil && baseUrlSetting.Value != "" {
		for _, rawURL := range strings.FieldsFunc(baseUrlSetting.Value, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == ';' || r == ' '
		}) {
			rawURL = strings.TrimSpace(rawURL)
			if !strings.Contains(rawURL, "://") {
				rawURL = "https://" + rawURL
			}
			if bu, err := url.Parse(rawURL); err == nil && strings.ToLower(bu.Hostname()) == refHost {
				return true
			}
		}
	}

	// Check configured allowed domains whitelist
	for _, rule := range allowedRules {
		if strings.HasPrefix(rule, "*.") {
			domain := strings.TrimPrefix(rule, "*.")
			if refHost == domain || strings.HasSuffix(refHost, "."+domain) {
				return true
			}
		} else {
			if refHost == rule || strings.HasSuffix(refHost, "."+rule) {
				return true
			}
		}
	}

	return false
}

func parseDomainRules(val string) []string {
	var rules []string
	fields := strings.FieldsFunc(val, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';' || r == ' '
	})
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f != "" {
			rules = append(rules, f)
		}
	}
	return rules
}

// DownloadRaw serves unwatermarked source for owner or admin
func (s *DispatchService) ServeRaw(c *gin.Context, fileID uint, currentUserID uint, isAdmin bool) {
	var file model.FileRecord
	query := s.db.Where("id = ?", fileID)
	if !isAdmin {
		query = query.Where("user_id = ?", currentUserID)
	}
	if err := query.First(&file).Error; err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	key := file.StorageKey
	if file.SourceKey != "" {
		key = file.SourceKey
	}

	safeName := strings.NewReplacer("\r", "", "\n", "", "\"", "\\\"").Replace(file.DisplayName)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", safeName, url.PathEscape(file.DisplayName)))
	c.Header("Content-Type", file.MimeType)

	if localPath, ok := s.storage.GetLocalPath(key); ok {
		http.ServeFile(c.Writer, c.Request, localPath)
		return
	}

	r, err := s.storage.Get(c.Request.Context(), key)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer r.Close()

	if rs, ok := r.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, file.DisplayName, file.UpdatedAt, rs)
	} else {
		_, _ = io.Copy(c.Writer, r)
	}
}
