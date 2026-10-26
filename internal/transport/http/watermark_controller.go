package http

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/auth"
	"github.com/UtopiaXC/UtopiaImages/internal/cache"
	"github.com/UtopiaXC/UtopiaImages/internal/imaging"
	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WatermarkController struct {
	db           *gorm.DB
	processor    imaging.ImageProcessor
	derivedCache *cache.DerivedCache
}

func NewWatermarkController(db *gorm.DB, proc imaging.ImageProcessor, dc *cache.DerivedCache) *WatermarkController {
	return &WatermarkController{
		db:           db,
		processor:    proc,
		derivedCache: dc,
	}
}

func (ctrl *WatermarkController) RegisterRoutes(rg *gin.RouterGroup) {
	wm := rg.Group("/watermarks")
	wm.Use(auth.RequireAuth())
	{
		wm.GET("", ctrl.ListWatermarks)
		wm.POST("", ctrl.CreateWatermark)
		wm.GET("/:id", ctrl.GetWatermark)
		wm.PATCH("/:id", ctrl.UpdateWatermark)
		wm.DELETE("/:id", ctrl.DeleteWatermark)
		wm.POST("/preview", ctrl.PreviewWatermark)
	}
}

func (ctrl *WatermarkController) ListWatermarks(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	var templates []model.WatermarkTemplate
	ctrl.db.Where("owner_id = ? OR owner_id IS NULL", userID).Order("id DESC").Find(&templates)
	c.JSON(http.StatusOK, templates)
}

func (ctrl *WatermarkController) CreateWatermark(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)

	var req struct {
		Name     string `json:"name" binding:"required"`
		Layers   string `json:"layers" binding:"required"`
		IsGlobal bool   `json:"is_global"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Watermark template name cannot be empty"})
		return
	}

	var count int64
	ctrl.db.Model(&model.WatermarkTemplate{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Watermark template name already exists"})
		return
	}

	tmpl := model.WatermarkTemplate{
		Name:      req.Name,
		Layers:    req.Layers,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if req.IsGlobal && isUserAdmin(user) {
		tmpl.OwnerID = nil
	} else {
		tmpl.OwnerID = &user.ID
	}

	if err := ctrl.db.Create(&tmpl).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Invalidate derived caches and bump file versions so new watermark applies immediately
	if ctrl.derivedCache != nil {
		ctrl.derivedCache.Clear()
	}
	ctrl.db.Model(&model.FileRecord{}).Where("1 = 1").Update("sign_version", gorm.Expr("sign_version + 1"))

	c.JSON(http.StatusOK, tmpl)
}

func (ctrl *WatermarkController) GetWatermark(c *gin.Context) {
	userID := c.GetUint(auth.CtxUserIDKey)
	id := c.Param("id")

	var tmpl model.WatermarkTemplate
	if err := ctrl.db.Where("id = ? AND (owner_id = ? OR owner_id IS NULL)", id, userID).First(&tmpl).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark template not found"})
		return
	}

	c.JSON(http.StatusOK, tmpl)
}

func (ctrl *WatermarkController) UpdateWatermark(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	id := c.Param("id")

	var tmpl model.WatermarkTemplate
	query := ctrl.db.Where("id = ?", id)
	if !isUserAdmin(user) {
		query = query.Where("owner_id = ?", user.ID)
	}
	if err := query.First(&tmpl).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark template not found"})
		return
	}

	var req struct {
		Name   string `json:"name"`
		Layers string `json:"layers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{
		"updated_at": time.Now(),
		"version":    gorm.Expr("version + 1"),
	}
	if req.Name != "" {
		req.Name = strings.TrimSpace(req.Name)
		var count int64
		ctrl.db.Model(&model.WatermarkTemplate{}).Where("name = ? AND id != ?", req.Name, id).Count(&count)
		if count > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Watermark template name already exists"})
			return
		}
		updates["name"] = req.Name
	}
	if req.Layers != "" {
		updates["layers"] = req.Layers
	}

	ctrl.db.Model(&tmpl).Updates(updates)
	ctrl.db.Where("id = ?", id).First(&tmpl)

	// Immediately clear derived image caches and invalidate client keys
	if ctrl.derivedCache != nil {
		ctrl.derivedCache.Clear()
	}
	ctrl.db.Model(&model.FileRecord{}).Where("1 = 1").Update("sign_version", gorm.Expr("sign_version + 1"))

	c.JSON(http.StatusOK, tmpl)
}

func (ctrl *WatermarkController) DeleteWatermark(c *gin.Context) {
	uVal, _ := c.Get(auth.CtxUserKey)
	user := uVal.(*model.User)
	id := c.Param("id")

	query := ctrl.db.Where("id = ?", id)
	if !isUserAdmin(user) {
		query = query.Where("owner_id = ?", user.ID)
	}

	res := query.Delete(&model.WatermarkTemplate{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark template not found or unauthorized"})
		return
	}

	if ctrl.derivedCache != nil {
		ctrl.derivedCache.Clear()
	}
	ctrl.db.Model(&model.FileRecord{}).Where("watermark_template_id = ?", id).Updates(map[string]any{
		"watermark_template_id": nil,
	})
	ctrl.db.Model(&model.FileRecord{}).Where("1 = 1").Update("sign_version", gorm.Expr("sign_version + 1"))

	c.JSON(http.StatusOK, gin.H{"message": "Watermark template deleted"})
}

func (ctrl *WatermarkController) PreviewWatermark(c *gin.Context) {
	var req struct {
		Type            string  `json:"type"`     // text / image
		Text            string  `json:"text"`     // watermark text
		Layout          string  `json:"layout"`   // tile / position
		Position        string  `json:"position"` // bottom-right, etc.
		Opacity         float64 `json:"opacity"`  // 0.0 - 1.0
		Angle           float64 `json:"angle"`    // degrees
		SpacingX        int     `json:"spacing_x"`
		SpacingY        int     `json:"spacing_y"`
		FontSize        int     `json:"font_size"`
		FontFamily      string  `json:"font_family"`
		FontColor       string  `json:"font_color"`
		StrokeColor     string  `json:"stroke_color"`
		StrokeWidth     int     `json:"stroke_width"`
		AdaptiveScaling *bool   `json:"adaptive_scaling"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type == "" {
		req.Type = "text"
	}
	if req.Text == "" {
		req.Text = "Utopia Images"
	}
	if req.Opacity <= 0 {
		req.Opacity = 0.2
	}
	if req.FontSize <= 0 {
		req.FontSize = 16
	}
	if req.FontColor == "" {
		req.FontColor = "#ffffff"
	}

	// Replace variables in preview
	now := time.Now()
	displayText := req.Text
	displayText = strings.ReplaceAll(displayText, "{now}", now.Format("2006-01-02 15:04:05"))
	displayText = strings.ReplaceAll(displayText, "{datetime}", now.Format("2006-01-02 15:04:05"))
	displayText = strings.ReplaceAll(displayText, "{year}", now.Format("2006"))
	displayText = strings.ReplaceAll(displayText, "{date}", now.Format("2006-01-02"))
	displayText = strings.ReplaceAll(displayText, "{time}", now.Format("15:04:05"))
	displayText = strings.ReplaceAll(displayText, "{ip}", c.ClientIP())
	displayText = strings.ReplaceAll(displayText, "{owner}", "admin")
	displayText = strings.ReplaceAll(displayText, "{filename}", "preview_sample.jpg")
	displayText = strings.ReplaceAll(displayText, "{ext}", "jpg")
	displayText = strings.ReplaceAll(displayText, "{dimensions}", "1920x1080")
	displayText = strings.ReplaceAll(displayText, "{filesize}", "1.8 MB")
	displayText = strings.ReplaceAll(displayText, "{domain}", c.Request.Host)
	displayText = strings.ReplaceAll(displayText, "{copyright}", fmt.Sprintf("© %s %s", now.Format("2006"), c.Request.Host))
	displayText = strings.ReplaceAll(displayText, "{confidential}", "CONFIDENTIAL")
	displayText = strings.ReplaceAll(displayText, "{preview_only}", "FOR PREVIEW ONLY")

	// Create sample image: dark navy gradient with grid lines
	img := image.NewRGBA(image.Rect(0, 0, 520, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 520; x++ {
			r := uint8(24 + (x * 20 / 520))
			g := uint8(28 + (y * 30 / 320))
			b := uint8(44 + ((x + y) * 20 / 840))
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Draw sample center rectangle
	draw.Draw(img, image.Rect(40, 40, 480, 280), &image.Uniform{C: color.RGBA{R: 35, G: 42, B: 60, A: 255}}, image.Point{}, draw.Src)

	// Process watermark directly on sample
	angle := req.Angle
	if req.Layout == "position" {
		angle = 0
	}
	outBytes, _, err := ctrl.processor.Process(bytes.NewReader(imgToJpegBytes(img)), imaging.ProcessOptions{
		Quality: 90,
		Watermark: &imaging.WatermarkConfig{
			Enabled:         true,
			Type:            req.Type,
			Text:            displayText,
			Layout:          req.Layout,
			Position:        req.Position,
			Opacity:         req.Opacity,
			Angle:           angle,
			SpacingX:        req.SpacingX,
			SpacingY:        req.SpacingY,
			FontSize:        req.FontSize,
			FontFamily:      req.FontFamily,
			FontColor:       req.FontColor,
			StrokeColor:     req.StrokeColor,
			StrokeWidth:     req.StrokeWidth,
			AdaptiveScaling: req.AdaptiveScaling,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Preview generation failed: " + err.Error()})
		return
	}

	base64URI := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(outBytes)
	c.JSON(http.StatusOK, gin.H{"data_url": base64URI})
}

func imgToJpegBytes(img image.Image) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	return buf.Bytes()
}
