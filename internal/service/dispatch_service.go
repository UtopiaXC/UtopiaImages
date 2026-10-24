package service

import (
	"bytes"
	"context"
	"fmt"
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

// ServeFile handles client requests for public files
func (s *DispatchService) ServeFile(c *gin.Context) {
	// 1. Sanitize request path
	rawPath := strings.TrimPrefix(c.Request.URL.Path, "/")
	publicPath := pathclean.CleanPath(rawPath)

	if publicPath == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	// 2. Query file record
	var file model.FileRecord
	if err := s.db.Where("public_path = ?", publicPath).First(&file).Error; err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	// 3. Hotlinking (Referer) check
	if !s.checkReferer(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// 4. Security headers
	c.Header("X-Content-Type-Options", "nosniff")
	// CSP sandbox prevents scripts in uploaded HTML/SVG from attacking host domain
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Access-Control-Allow-Origin", "*")

	// 5. Check if dynamic processing is requested
	reqW, _ := strconv.Atoi(c.Query("w"))
	reqH, _ := strconv.Atoi(c.Query("h"))
	reqStyle := c.Query("s")
	reqFmt := c.Query("fmt")

	needsWatermark := file.WatermarkMode == "on"

	// Check if valid temporary unwatermarked link (?e=timestamp) is provided
	if expiresStr := c.Query("e"); expiresStr != "" {
		if exp, err := strconv.ParseInt(expiresStr, 10, 64); err == nil && time.Now().Unix() <= exp {
			needsWatermark = false
		}
	}

	hasTransformParams := reqW > 0 || reqH > 0 || reqStyle != "" || reqFmt != ""

	// If NOT an image, or no watermark and no resize requested: direct straight-through delivery
	if !file.IsImage || (!needsWatermark && !hasTransformParams) {
		s.serveDirect(c, file)
		return
	}

	// 6. Handle Image Processing with Derived Cache
	s.serveProcessed(c, file, reqW, reqH, reqStyle, reqFmt, needsWatermark)
}

func (s *DispatchService) serveDirect(c *gin.Context, file model.FileRecord) {
	c.Header("Content-Type", file.MimeType)
	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	// If local file storage is used, leverage zero-copy sendfile and Range handling
	if localPath, ok := s.storage.GetLocalPath(file.StorageKey); ok {
		http.ServeFile(c.Writer, c.Request, localPath)
		return
	}

	// Fallback streaming for non-local drivers
	r, err := s.storage.Get(c.Request.Context(), file.StorageKey)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer r.Close()

	http.ServeContent(c.Writer, c.Request, filepath.Base(file.PublicPath), file.UpdatedAt, r.(readSeekerFallback))
}

type readSeekerFallback interface {
	Read([]byte) (int, error)
	Seek(int64, int) (int64, error)
}

func (s *DispatchService) serveProcessed(
	c *gin.Context,
	file model.FileRecord,
	reqW, reqH int,
	style, format string,
	watermark bool,
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
	}

	// Dynamic watermark config
	var wmConfig *imaging.WatermarkConfig
	var isPrivateCache bool

	if watermark {
		wmConfig = &imaging.WatermarkConfig{
			Enabled:  true,
			Type:     "text",
			Layout:   "tile",
			Opacity:  0.15,
			Position: "bottom-right",
		}

		// Load template if custom
		if file.WatermarkTemplateID != nil {
			var tmpl model.WatermarkTemplate
			if err := s.db.Where("id = ?", *file.WatermarkTemplateID).First(&tmpl).Error; err == nil {
				// Custom watermark template found
			}
		}

		// Render text with variables
		clientIP := c.ClientIP()
		wmConfig.Text = fmt.Sprintf("Utopia Images %s", time.Now().Format("2006-01-02"))
		if strings.Contains(wmConfig.Text, clientIP) {
			isPrivateCache = true
		}
	}

	// Construct cache key
	paramKey := fmt.Sprintf("w%d_h%d_f%s_q%d_wm%v", targetW, targetH, targetFormat, targetQuality, watermark)
	cacheKey := s.cache.GenerateKey(file.PublicPath, paramKey, strconv.FormatInt(file.SignVersion, 10))
	ext := "." + targetFormat
	if targetFormat == "" {
		ext = ".jpg"
	}

	// 1. Try Cache Hit
	if cachedPath, hit := s.cache.Get(cacheKey, ext); hit {
		if isPrivateCache {
			c.Header("Cache-Control", "private, no-store")
		} else {
			c.Header("Cache-Control", "public, max-age=604800")
		}
		http.ServeFile(c.Writer, c.Request, cachedPath)
		return
	}

	// 2. Cache Miss: Execute singleflight processing
	data, mime, err := s.cache.ComputeSingleFlight(cacheKey, func() ([]byte, string, error) {
		r, err := s.storage.Get(context.Background(), file.StorageKey)
		if err != nil {
			return nil, "", err
		}
		defer r.Close()

		opts := imaging.ProcessOptions{
			Width:     targetW,
			Height:    targetH,
			Format:    targetFormat,
			Quality:   targetQuality,
			Watermark: wmConfig,
		}

		return s.processor.Process(r, opts)
	})

	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Store in cache (if not request-specific dynamic watermark)
	if !isPrivateCache {
		s.cache.Put(cacheKey, ext, data)
		c.Header("Cache-Control", "public, max-age=604800")
	} else {
		c.Header("Cache-Control", "private, no-store")
	}

	c.Header("Content-Type", mime)
	c.Data(http.StatusOK, mime, data)
}

func (s *DispatchService) checkReferer(c *gin.Context) bool {
	referer := c.Request.Referer()
	if referer == "" {
		return true // Allow empty referer by default
	}

	u, err := url.Parse(referer)
	if err != nil {
		return true
	}

	// Always allow self
	if strings.EqualFold(u.Host, c.Request.Host) {
		return true
	}

	// In the future: check per-storage-policy referer rules
	return true
}

func (s *DispatchService) CreateTempLink(file *model.FileRecord, expiresDuration time.Duration) string {
	expiresAt := time.Now().Add(expiresDuration).Unix()
	return fmt.Sprintf("/%s?e=%d", file.PublicPath, expiresAt)
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

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.DisplayName))
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

	stat, _ := os.Stat(key)
	http.ServeContent(c.Writer, c.Request, file.DisplayName, stat.ModTime(), bytes.NewReader([]byte{}))
}
