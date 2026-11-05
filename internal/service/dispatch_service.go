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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

type callStatRecord struct {
	FileID    uint
	Format    string
	Size      string
	RealIP    string
	VisitorID string
}

type clientTransformBucket struct {
	tokens    int
	lastCheck time.Time
}

type DispatchService struct {
	db               *gorm.DB
	storage          storage.StorageDriver
	driverManager    *storage.PolicyDriverManager
	processor        imaging.ImageProcessor
	cache            *cache.DerivedCache
	statChan         chan callStatRecord
	transformLimiter sync.Mutex
	transformBuckets map[string]*clientTransformBucket
}

func NewDispatchService(
	db *gorm.DB,
	st storage.StorageDriver,
	proc imaging.ImageProcessor,
	dc *cache.DerivedCache,
) *DispatchService {
	ds := &DispatchService{
		db:               db,
		storage:          st,
		processor:        proc,
		cache:            dc,
		statChan:         make(chan callStatRecord, 5000),
		transformBuckets: make(map[string]*clientTransformBucket),
	}
	go ds.processStatsWorker()
	return ds
}

func (s *DispatchService) processStatsWorker() {
	for rec := range s.statChan {
		func() {
			defer func() { _ = recover() }()
			if s.db == nil {
				return
			}
			silentDB := s.db.Session(&gorm.Session{Logger: s.db.Logger.LogMode(glogger.Silent)})

			// 1. Update or create ImageCallStat for PV
			res := silentDB.Model(&model.ImageCallStat{}).
				Where("file_id = ? AND format = ? AND size = ?", rec.FileID, rec.Format, rec.Size).
				Update("count", gorm.Expr("count + 1"))
			if res.Error == nil && res.RowsAffected == 0 {
				stat := model.ImageCallStat{
					FileID: rec.FileID,
					Format: rec.Format,
					Size:   rec.Size,
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
			silentDB.Model(&model.ImageCallVisitor{}).
				Where("file_id = ? AND format = ? AND size = ? AND visitor_id = ?", rec.FileID, rec.Format, rec.Size, rec.VisitorID).
				Count(&exists)
			if exists == 0 {
				vis := model.ImageCallVisitor{
					FileID:    rec.FileID,
					Format:    rec.Format,
					Size:      rec.Size,
					VisitorID: rec.VisitorID,
					IP:        rec.RealIP,
				}
				resVis := silentDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&vis)
				if resVis.Error == nil && resVis.RowsAffected > 0 {
					silentDB.Model(&model.ImageCallStat{}).
						Where("file_id = ? AND format = ? AND size = ?", rec.FileID, rec.Format, rec.Size).
						Update("uv", gorm.Expr("uv + 1"))
				}
			}
		}()
	}
}

func (s *DispatchService) allowTransformRequest(ip string) bool {
	if ip == "" {
		return true
	}
	s.transformLimiter.Lock()
	defer s.transformLimiter.Unlock()

	now := time.Now()
	// Periodic cleanup if map grows too large
	if len(s.transformBuckets) > 2000 {
		for k, b := range s.transformBuckets {
			if now.Sub(b.lastCheck) > 5*time.Minute {
				delete(s.transformBuckets, k)
			}
		}
	}

	bucket, ok := s.transformBuckets[ip]
	if !ok {
		s.transformBuckets[ip] = &clientTransformBucket{
			tokens:    30, // max burst
			lastCheck: now,
		}
		return true
	}

	// Refill 2 tokens per second (up to 30)
	elapsed := now.Sub(bucket.lastCheck).Seconds()
	bucket.tokens += int(elapsed * 2)
	if bucket.tokens > 30 {
		bucket.tokens = 30
	}
	bucket.lastCheck = now

	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	return false
}

func (s *DispatchService) SetDB(db *gorm.DB) {
	s.db = db
}

func (s *DispatchService) SetDriverManager(dm *storage.PolicyDriverManager) {
	s.driverManager = dm
}

func (s *DispatchService) abortWithError(c *gin.Context, code int) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.AbortWithStatus(code)
}

func isRemoteTrustedProxy(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	remote := c.Request.RemoteAddr
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// GetRealClientIP extracts the real client IP considering CDNs and proxies.
func GetRealClientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if isRemoteTrustedProxy(c) {
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
				if parsed := net.ParseIP(val); parsed != nil {
					return parsed.String()
				}
			}
		}
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				first := strings.TrimSpace(parts[0])
				if parsed := net.ParseIP(first); parsed != nil {
					return parsed.String()
				}
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
	var preferredReplica *model.FilePolicyReplica
	var requestedFormat string

	// 2. Query file replica or record (direct lookup or dynamic extension lookup)
	var matchedReplica model.FilePolicyReplica
	if err := s.db.Where("public_path = ? AND status = 'ready'", publicPath).First(&matchedReplica).Error; err == nil {
		if err := s.db.First(&file, matchedReplica.FileID).Error; err == nil {
			preferredReplica = &matchedReplica
		}
	}

	if file.ID == 0 {
		if err := s.db.Where("public_path = ?", publicPath).First(&file).Error; err == nil {
			var rep model.FilePolicyReplica
			if err := s.db.Where("file_id = ? AND (public_path = ? OR policy_id = ?) AND status = 'ready'", file.ID, publicPath, file.PolicyID).First(&rep).Error; err == nil {
				preferredReplica = &rep
			}
		}
	}

	if file.ID == 0 {
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

			var candRep model.FilePolicyReplica
			if err := s.db.Where("public_path IN ? AND status = 'ready'", candidatePaths).First(&candRep).Error; err == nil {
				if err := s.db.First(&file, candRep.FileID).Error; err == nil {
					preferredReplica = &candRep
					requestedFormat = reqExt
				}
			}

			if file.ID == 0 {
				var candidateRecord model.FileRecord
				if err := s.db.Where("public_path IN ? AND is_image = ?", candidatePaths, true).First(&candidateRecord).Error; err == nil {
					file = candidateRecord
					requestedFormat = reqExt
					var rep model.FilePolicyReplica
					if err := s.db.Where("file_id = ? AND status = 'ready'", file.ID).First(&rep).Error; err == nil {
						preferredReplica = &rep
					}
				}
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

	// 5. Parse and validate requested format
	validFormats := map[string]bool{
		"jpg": true, "jpeg": true, "png": true, "webp": true, "gif": true, "bmp": true, "tiff": true,
	}
	if fmtQuery := c.Query("fmt"); fmtQuery != "" {
		cleanFmt := strings.ToLower(strings.TrimPrefix(fmtQuery, "."))
		if validFormats[cleanFmt] {
			requestedFormat = cleanFmt
		}
	}
	if formatQuery := c.Query("format"); formatQuery != "" {
		cleanFmt := strings.ToLower(strings.TrimPrefix(formatQuery, "."))
		if validFormats[cleanFmt] {
			requestedFormat = cleanFmt
		}
	}

	origExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
	if requestedFormat != "" && requestedFormat != origExt && !s.isFormatConversionEnabled() {
		s.abortWithError(c, http.StatusNotFound)
		return
	}

	targetFormat := origExt
	if requestedFormat != "" && validFormats[requestedFormat] {
		targetFormat = requestedFormat
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
		s.serveDirect(c, file, preferredReplica, isRawRedirect)
		return
	}

	// 8. Handle Image Processing with 2-Stage Caching
	s.serveProcessed(c, file, preferredReplica, reqW, reqH, reqStyle, requestedFormat, needsWatermark, isRawRedirect)
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
	if len(format) > 16 {
		format = format[:16]
	}
	if format == "" {
		format = "original"
	}
	size = strings.ToLower(strings.TrimSpace(size))
	if len(size) > 32 {
		size = size[:32]
	}
	if size == "" {
		size = "original"
	}

	realIP := GetRealClientIP(c)
	visitorID := GenerateVisitorID(c)

	select {
	case s.statChan <- callStatRecord{
		FileID:    fileID,
		Format:    format,
		Size:      size,
		RealIP:    realIP,
		VisitorID: visitorID,
	}:
	default:
		// Queue is full under extreme load, drop telemetry to prevent memory exhaustion and db lock
	}
}

func shortHash(h string) string {
	if len(h) >= 16 {
		return h[:16]
	}
	return h
}

type fileSource struct {
	driver     storage.StorageDriver
	storageKey string
	localPath  string
	isLocal    bool
}

func (s *DispatchService) getFileStream(ctx context.Context, file *model.FileRecord, preferredReplica *model.FilePolicyReplica) (*fileSource, io.ReadCloser, error) {
	type candidate struct {
		policyID   uint
		storageKey string
	}
	var candidates []candidate
	seen := make(map[string]bool)

	addCandidate := func(pID uint, key string) {
		if key == "" {
			return
		}
		sig := fmt.Sprintf("%d:%s", pID, key)
		if !seen[sig] {
			seen[sig] = true
			candidates = append(candidates, candidate{policyID: pID, storageKey: key})
		}
	}

	if preferredReplica != nil && preferredReplica.StorageKey != "" {
		addCandidate(preferredReplica.PolicyID, preferredReplica.StorageKey)
	}

	if s.db != nil && file != nil && file.ID > 0 {
		var reps []model.FilePolicyReplica
		if err := s.db.Where("file_id = ? AND status = 'ready'", file.ID).Find(&reps).Error; err == nil {
			for _, r := range reps {
				addCandidate(r.PolicyID, r.StorageKey)
			}
		}
	}

	if file != nil && file.StorageKey != "" {
		addCandidate(file.PolicyID, file.StorageKey)
	}

	for _, cand := range candidates {
		var driver storage.StorageDriver
		if s.driverManager != nil && cand.policyID > 0 {
			d, _, err := s.driverManager.GetDriver(ctx, cand.policyID)
			if err == nil {
				driver = d
			}
		}
		if driver == nil && file != nil && (cand.policyID == file.PolicyID || cand.policyID == 0) {
			driver = s.storage
		}
		if driver == nil {
			continue
		}

		if localPath, ok := driver.GetLocalPath(cand.storageKey); ok {
			if _, statErr := os.Stat(localPath); statErr == nil {
				f, openErr := os.Open(localPath)
				if openErr == nil {
					return &fileSource{
						driver:     driver,
						storageKey: cand.storageKey,
						localPath:  localPath,
						isLocal:    true,
					}, f, nil
				}
			}
		}

		rc, err := driver.Get(ctx, cand.storageKey)
		if err == nil && rc != nil {
			return &fileSource{
				driver:     driver,
				storageKey: cand.storageKey,
			}, rc, nil
		}
		slog.Warn("Failed to get file stream from driver", "policy_id", cand.policyID, "storage_key", cand.storageKey, "error", err)
	}

	// Fallback to default storage
	if s.storage != nil && file != nil && file.StorageKey != "" {
		if localPath, ok := s.storage.GetLocalPath(file.StorageKey); ok {
			if _, statErr := os.Stat(localPath); statErr == nil {
				f, openErr := os.Open(localPath)
				if openErr == nil {
					return &fileSource{
						driver:     s.storage,
						storageKey: file.StorageKey,
						localPath:  localPath,
						isLocal:    true,
					}, f, nil
				}
			}
		}
		rc, err := s.storage.Get(ctx, file.StorageKey)
		if err == nil && rc != nil {
			return &fileSource{
				driver:     s.storage,
				storageKey: file.StorageKey,
			}, rc, nil
		}
	}

	fileID := uint(0)
	if file != nil {
		fileID = file.ID
	}
	return nil, nil, fmt.Errorf("file source not found for file %d", fileID)
}

func (s *DispatchService) serveDirect(c *gin.Context, file model.FileRecord, preferredReplica *model.FilePolicyReplica, isRawRedirect bool) {
	mimeType := file.MimeType
	origExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.PublicPath), "."))
	if origExt == "ico" && (mimeType == "" || mimeType == "application/octet-stream") {
		mimeType = "image/x-icon"
	} else if origExt == "svg" && (mimeType == "" || mimeType == "application/octet-stream") {
		mimeType = "image/svg+xml"
	}
	if origExt == "svg" || origExt == "html" || origExt == "htm" {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(file.PublicPath)))
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

	src, rc, err := s.getFileStream(c.Request.Context(), &file, preferredReplica)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if rc != nil {
		defer rc.Close()
	}

	if src != nil && src.isLocal && src.localPath != "" {
		http.ServeFile(c.Writer, c.Request, src.localPath)
		return
	}

	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, filepath.Base(file.PublicPath), file.UpdatedAt, rs)
	} else if rc != nil {
		_, _ = io.Copy(c.Writer, rc)
	}
}

func (s *DispatchService) serveProcessed(
	c *gin.Context,
	file model.FileRecord,
	preferredReplica *model.FilePolicyReplica,
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
		// Quantize custom dimensions to steps of 64 and clamp to 4096 to prevent cache exhaustion
		if targetW > 0 {
			targetW = ((targetW + 31) / 64) * 64
			if targetW < 32 {
				targetW = 32
			} else if targetW > 4096 {
				targetW = 4096
			}
		}
		if targetH > 0 {
			targetH = ((targetH + 31) / 64) * 64
			if targetH < 32 {
				targetH = 32
			} else if targetH > 4096 {
				targetH = 4096
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

	// Step 0: Fast 304 Not Modified pre-check if no watermark needed
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
	}

	// Step 1: Resolve Base Image (with dynamic format conversion / resize cached)
	var baseBytes []byte
	var baseMime string
	var err error

	if needsTransform {
		baseCacheKey := fmt.Sprintf("base_%s_f%s_w%d_h%d_q%d", file.Sha256, targetFormat, targetW, targetH, targetQuality)
		baseCacheExt := "." + targetFormat

		if cachedPath, hit := s.cache.Get(baseCacheKey, baseCacheExt); hit {
			if !watermark {
				c.Header("Content-Type", mimeFromFormat(targetFormat))
				http.ServeFile(c.Writer, c.Request, cachedPath)
				return
			}
			baseBytes, err = os.ReadFile(cachedPath)
		}

		if len(baseBytes) == 0 || err != nil {
			if !s.allowTransformRequest(GetRealClientIP(c)) {
				c.Header("Retry-After", "10")
				c.AbortWithStatus(http.StatusTooManyRequests)
				return
			}
			baseBytes, baseMime, err = s.cache.ComputeSingleFlight(baseCacheKey, func() ([]byte, string, error) {
				_, r, err := s.getFileStream(context.Background(), &file, preferredReplica)
				if err != nil {
					return nil, "", err
				}
				if r == nil {
					return nil, "", fmt.Errorf("file stream is nil for %s", file.PublicPath)
				}
				defer r.Close()

				opts := imaging.ProcessOptions{
					Width:   targetW,
					Height:  targetH,
					Format:  targetFormat,
					Quality: targetQuality,
				}
				return s.processor.Process(io.LimitReader(r, 100*1024*1024), opts)
			})
			if err != nil {
				slog.Error("Image transform failed", "path", file.PublicPath, "error", err)
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			s.cache.Put(baseCacheKey, baseCacheExt, baseBytes)
		}
	} else if !watermark {
		// Serve unconverted original directly via streaming without buffering 100MB in RAM
		_, r, err := s.getFileStream(context.Background(), &file, preferredReplica)
		if err != nil || r == nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		defer r.Close()
		c.Header("Content-Type", file.MimeType)
		c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
		_, _ = io.Copy(c.Writer, r)
		return
	} else {
		// Watermark is needed, buffer original
		_, r, err := s.getFileStream(context.Background(), &file, preferredReplica)
		if err != nil || r == nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		defer r.Close()
		baseBytes, err = io.ReadAll(io.LimitReader(r, 100*1024*1024))
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
		if err := s.db.Where("id = ? AND (owner_id = ? OR is_system = ? OR owner_id IS NULL)", *tmplID, file.UserID, true).First(&tmpl).Error; err == nil && tmpl.Layers != "" {
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
		if !s.allowTransformRequest(GetRealClientIP(c)) {
			c.Header("Retry-After", "10")
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		// REQUIREMENT 6: Dynamic watermarks MUST NOT be cached on disk!
		wmConfig.Text = s.renderDynamicWatermarkText(wmConfig.Text, &file, c)

		watermarkedBytes, outMime, err := s.processor.Process(bytes.NewReader(baseBytes), imaging.ProcessOptions{
			Format:    targetFormat,
			Quality:   targetQuality,
			Watermark: wmConfig,
		})
		if err != nil {
			slog.Error("Dynamic watermark processing failed", "path", file.PublicPath, "error", err)
			c.AbortWithStatus(http.StatusInternalServerError)
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
		slog.Error("Static watermark processing failed", "path", file.PublicPath, "error", err)
		c.AbortWithStatus(http.StatusInternalServerError)
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
	if h, _, err := net.SplitHostPort(currentHost); err == nil {
		currentHost = h
	}

	// Determine if currentHost is a trusted/configured host (or LAN/localhost)
	isCurrentHostTrusted := false
	if currentHost == "localhost" || currentHost == "127.0.0.1" || currentHost == "::1" {
		isCurrentHostTrusted = true
	} else if ip := net.ParseIP(currentHost); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			isCurrentHostTrusted = true
		}
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
			if bu, err := url.Parse(rawURL); err == nil {
				h := strings.ToLower(bu.Hostname())
				if h == currentHost {
					isCurrentHostTrusted = true
				}
				if h == refHost {
					return true
				}
			}
		}
	}

	// Check configured allowed domains whitelist
	for _, rule := range allowedRules {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if rule == "" {
			continue
		}
		if rule == currentHost || (strings.HasPrefix(rule, "*.") && strings.HasSuffix(currentHost, rule[1:])) {
			isCurrentHostTrusted = true
		}
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

	// Only allow requests from the same host if currentHost is trusted or no restrictive rules configured
	if refHost == currentHost && (isCurrentHostTrusted || (baseUrlSetting.Value == "" && len(allowedRules) == 0)) {
		return true
	}

	return false
}

func parseDomainRules(val string) []string {
	if len(val) > 8192 {
		val = val[:8192]
	}
	var rules []string
	fields := strings.FieldsFunc(val, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';' || r == ' '
	})
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f != "" && len(f) <= 255 {
			rules = append(rules, f)
			if len(rules) >= 500 {
				break
			}
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

	safeName := strings.NewReplacer("\r", "", "\n", "", "\"", "\\\"").Replace(file.DisplayName)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", safeName, url.PathEscape(file.DisplayName)))
	c.Header("Content-Type", file.MimeType)

	src, rc, err := s.getFileStream(c.Request.Context(), &file, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if rc != nil {
		defer rc.Close()
	}

	if src != nil && src.isLocal && src.localPath != "" {
		http.ServeFile(c.Writer, c.Request, src.localPath)
		return
	}

	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, file.DisplayName, file.UpdatedAt, rs)
	} else if rc != nil {
		_, _ = io.Copy(c.Writer, rc)
	}
}
