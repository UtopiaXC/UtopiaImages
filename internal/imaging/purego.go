package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"

	"github.com/KarpelesLab/gowebp"
	"github.com/gen2brain/h265/heic"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type PureGoProcessor struct{}

func NewPureGoProcessor() *PureGoProcessor {
	return &PureGoProcessor{}
}

func (p *PureGoProcessor) GetDimensions(src io.Reader) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(src)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func (p *PureGoProcessor) Process(src io.Reader, opts ProcessOptions) ([]byte, string, error) {
	var rs io.ReadSeeker
	if s, ok := src.(io.ReadSeeker); ok {
		rs = s
	} else {
		buf, err := io.ReadAll(src)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read image stream: %w", err)
		}
		rs = bytes.NewReader(buf)
	}

	// 0. Pre-validate dimensions to protect against decompression bomb (OOM attack)
	cfg, _, err := image.DecodeConfig(rs)
	if err == nil {
		if cfg.Width > 16384 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
			return nil, "", fmt.Errorf("image dimensions too large (%dx%d exceeds safe limit)", cfg.Width, cfg.Height)
		}
	}
	_, _ = rs.Seek(0, io.SeekStart)

	img, format, err := image.Decode(rs)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	origBounds := img.Bounds()
	origW := origBounds.Dx()
	origH := origBounds.Dy()

	targetW := opts.Width
	targetH := opts.Height

	// 1. Resize if required
	var processedImg image.Image = img
	if targetW > 0 || targetH > 0 {
		if targetW <= 0 {
			targetW = int(float64(targetH) * float64(origW) / float64(origH))
		}
		if targetH <= 0 {
			targetH = int(float64(targetW) * float64(origH) / float64(origW))
		}

		// Keep original aspect ratio by default (fit within target bounding box)
		if opts.Fit != "cover" && opts.Fit != "fill" {
			ratioW := float64(targetW) / float64(origW)
			ratioH := float64(targetH) / float64(origH)
			ratio := ratioW
			if ratioH < ratio {
				ratio = ratioH
			}
			newW := int(float64(origW) * ratio)
			newH := int(float64(origH) * ratio)
			if newW > 0 {
				targetW = newW
			}
			if newH > 0 {
				targetH = newH
			}
		}

		if targetW < origW || targetH < origH {
			dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
			xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
			processedImg = dst
		}
	}

	// 2. Apply Watermark if enabled
	if opts.Watermark != nil && opts.Watermark.Enabled {
		processedImg = p.applyWatermark(processedImg, opts.Watermark)
	}

	// 3. Encode to target format
	outFormat := opts.Format
	if outFormat == "" {
		outFormat = format
	}
	outFormat = strings.ToLower(outFormat)

	var buf bytes.Buffer
	mime := "image/jpeg"

	switch outFormat {
	case "png":
		if err := png.Encode(&buf, processedImg); err != nil {
			return nil, "", err
		}
		mime = "image/png"
	case "webp":
		quality := opts.Quality
		if quality <= 0 {
			quality = 85
		}
		if err := gowebp.Encode(&buf, processedImg, &gowebp.Options{Lossy: true, Quality: float32(quality)}); err != nil {
			if err2 := jpeg.Encode(&buf, processedImg, &jpeg.Options{Quality: quality}); err2 != nil {
				return nil, "", err
			}
			mime = "image/jpeg"
		} else {
			mime = "image/webp"
		}
	case "heif", "heic":
		quality := opts.Quality
		if quality <= 0 {
			quality = 70
		}
		if err := heic.Encode(&buf, processedImg, heic.EncodeOptions{Quality: quality}); err != nil {
			if err2 := jpeg.Encode(&buf, processedImg, &jpeg.Options{Quality: quality}); err2 != nil {
				return nil, "", err
			}
			mime = "image/jpeg"
		} else {
			mime = "image/heic"
		}
	case "jpeg", "jpg":
		fallthrough
	default:
		quality := opts.Quality
		if quality <= 0 {
			quality = 85
		}
		if err := jpeg.Encode(&buf, processedImg, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", err
		}
		mime = "image/jpeg"
	}

	return buf.Bytes(), mime, nil
}

func (p *PureGoProcessor) applyWatermark(base image.Image, wm *WatermarkConfig) image.Image {
	bounds := base.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, base, bounds.Min, draw.Src)

	if wm.Type == "text" && wm.Text != "" {
		if wm.Layout == "tile" {
			p.renderTileText(dst, wm)
		} else {
			p.renderPositionText(dst, wm)
		}
	} else if wm.Type == "image" && len(wm.ImageBuf) > 0 {
		if wmImg, _, err := image.Decode(bytes.NewReader(wm.ImageBuf)); err == nil {
			p.renderWatermarkImage(dst, wmImg, wm.Position, wm.Opacity)
		}
	}

	if wm.EnableBlindWatermark != nil && *wm.EnableBlindWatermark {
		blindText := wm.BlindWatermarkText
		if blindText == "" {
			blindText = wm.Text
		}
		if blindText != "" {
			dst = EmbedBlindWatermark(dst, blindText)
		}
	}

	return dst
}

func parseHexColor(hexStr string, alpha uint8) color.RGBA {
	hexStr = strings.TrimPrefix(hexStr, "#")
	if len(hexStr) == 3 {
		hexStr = string([]byte{hexStr[0], hexStr[0], hexStr[1], hexStr[1], hexStr[2], hexStr[2]})
	}
	if len(hexStr) != 6 {
		return color.RGBA{R: 255, G: 255, B: 255, A: alpha}
	}
	var r, g, b uint8
	fmt.Sscanf(hexStr, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: alpha}
}

func createTextBadge(text string, fontFamily string, fontSize int, col color.RGBA, strokeCol color.RGBA, strokeWidth int) *image.RGBA {
	if fontSize <= 0 {
		fontSize = 24
	}

	face, err := GetFontManager().GetFace(fontFamily, fontSize)
	if err != nil || face == nil {
		face = basicfont.Face7x13
	}

	d := &font.Drawer{
		Face: face,
	}

	rawW := d.MeasureString(text).Ceil()
	ascent := face.Metrics().Ascent.Ceil()
	descent := face.Metrics().Descent.Ceil()
	rawH := ascent + descent
	if rawH <= 0 {
		rawH = fontSize
		ascent = fontSize * 3 / 4
	}

	pad := strokeWidth + 4
	badgeW := rawW + pad*2
	badgeH := rawH + pad*2
	badge := image.NewRGBA(image.Rect(0, 0, badgeW, badgeH))

	// Draw stroke/shadow if configured
	if strokeWidth > 0 && strokeCol.A > 0 {
		strokeDrawer := &font.Drawer{
			Dst:  badge,
			Src:  image.NewUniform(strokeCol),
			Face: face,
		}
		for dx := -strokeWidth; dx <= strokeWidth; dx++ {
			for dy := -strokeWidth; dy <= strokeWidth; dy++ {
				if dx*dx+dy*dy <= strokeWidth*strokeWidth {
					strokeDrawer.Dot = fixed.P(pad+dx, pad+ascent+dy)
					strokeDrawer.DrawString(text)
				}
			}
		}
	}

	// Draw main text
	textDrawer := &font.Drawer{
		Dst:  badge,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(pad, pad+ascent),
	}
	textDrawer.DrawString(text)

	// Enhance alpha contrast so font strokes remain crisp and solid on low-resolution images
	for i := 3; i < len(badge.Pix); i += 4 {
		a := badge.Pix[i]
		if a > 0 && a < 255 {
			fa := float64(a) / 255.0
			fa = math.Pow(fa, 0.72)
			badge.Pix[i] = uint8(math.Min(255.0, math.Round(fa*255.0)))
		}
	}

	return badge
}

func drawRotated(dst *image.RGBA, src image.Image, centerX, centerY int, angleDeg float64) {
	if angleDeg == 0 {
		srcB := src.Bounds()
		x0 := centerX - srcB.Dx()/2
		y0 := centerY - srcB.Dy()/2
		draw.Draw(dst, image.Rect(x0, y0, x0+srcB.Dx(), y0+srcB.Dy()), src, srcB.Min, draw.Over)
		return
	}

	rad := angleDeg * math.Pi / 180.0
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)

	srcB := src.Bounds()
	w := srcB.Dx()
	h := srcB.Dy()
	halfW := float64(w) / 2.0
	halfH := float64(h) / 2.0

	maxR := int(math.Ceil(math.Sqrt(halfW*halfW + halfH*halfH)))

	dstB := dst.Bounds()
	minX := centerX - maxR
	if minX < dstB.Min.X {
		minX = dstB.Min.X
	}
	maxX := centerX + maxR
	if maxX > dstB.Max.X {
		maxX = dstB.Max.X
	}
	minY := centerY - maxR
	if minY < dstB.Min.Y {
		minY = dstB.Min.Y
	}
	maxY := centerY + maxR
	if maxY > dstB.Max.Y {
		maxY = dstB.Max.Y
	}

	srcRGBA, ok := src.(*image.RGBA)
	if !ok {
		return
	}

	for y := minY; y < maxY; y++ {
		dy := float64(y - centerY)
		for x := minX; x < maxX; x++ {
			dx := float64(x - centerX)

			srcX := dx*cosA + dy*sinA + halfW
			srcY := -dx*sinA + dy*cosA + halfH

			if srcX < -0.5 || srcX >= float64(w)-0.5 || srcY < -0.5 || srcY >= float64(h)-0.5 {
				continue
			}

			// Bilinear interpolation for smooth, crisp edges on rotated text
			x0 := int(math.Floor(srcX))
			y0 := int(math.Floor(srcY))
			x1 := x0 + 1
			y1 := y0 + 1

			fx := srcX - float64(x0)
			fy := srcY - float64(y0)

			var sR, sG, sB, sA float64

			samplePixel := func(px, py int, weight float64) {
				if px >= 0 && px < w && py >= 0 && py < h {
					idx := py*srcRGBA.Stride + px*4
					a := float64(srcRGBA.Pix[idx+3])
					if a > 0 {
						sA += a * weight
						sR += float64(srcRGBA.Pix[idx]) * weight
						sG += float64(srcRGBA.Pix[idx+1]) * weight
						sB += float64(srcRGBA.Pix[idx+2]) * weight
					}
				}
			}

			samplePixel(x0, y0, (1.0-fx)*(1.0-fy))
			samplePixel(x1, y0, fx*(1.0-fy))
			samplePixel(x0, y1, (1.0-fx)*fy)
			samplePixel(x1, y1, fx*fy)

			uintA := uint32(math.Round(sA))
			if uintA == 0 {
				continue
			}
			uintR := uint32(math.Round(sR))
			uintG := uint32(math.Round(sG))
			uintB := uint32(math.Round(sB))

			dstIdx := (y-dstB.Min.Y)*dst.Stride + (x-dstB.Min.X)*4
			dA := uint32(dst.Pix[dstIdx+3])
			dR := uint32(dst.Pix[dstIdx])
			dG := uint32(dst.Pix[dstIdx+1])
			dB := uint32(dst.Pix[dstIdx+2])

			invA := 255 - uintA
			outR := (uintR*uintA + dR*invA) / 255
			outG := (uintG*uintA + dG*invA) / 255
			outB := (uintB*uintA + dB*invA) / 255
			outA := uintA + (dA*invA)/255

			dst.Pix[dstIdx] = uint8(outR)
			dst.Pix[dstIdx+1] = uint8(outG)
			dst.Pix[dstIdx+2] = uint8(outB)
			dst.Pix[dstIdx+3] = uint8(outA)
		}
	}
}

func computeAverageLuminance(img *image.RGBA) float64 {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w == 0 || h == 0 {
		return 128.0
	}
	var totalLum float64
	samples := 0
	stepX := w / 32
	if stepX < 1 {
		stepX = 1
	}
	stepY := h / 32
	if stepY < 1 {
		stepY = 1
	}
	for y := 0; y < h; y += stepY {
		for x := 0; x < w; x += stepX {
			idx := y*img.Stride + x*4
			if idx+2 < len(img.Pix) {
				r := float64(img.Pix[idx])
				g := float64(img.Pix[idx+1])
				b := float64(img.Pix[idx+2])
				totalLum += 0.299*r + 0.587*g + 0.114*b
				samples++
			}
		}
	}
	if samples == 0 {
		return 128.0
	}
	return totalLum / float64(samples)
}

func (p *PureGoProcessor) renderTileText(dst *image.RGBA, wm *WatermarkConfig) {
	b := dst.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()

	opacity := wm.Opacity
	if opacity <= 0 {
		opacity = 0.2
	}
	alpha := uint8(opacity * 255)

	fontCol := parseHexColor(wm.FontColor, alpha)
	if wm.FontColor == "" {
		fontCol = color.RGBA{R: 255, G: 255, B: 255, A: alpha}
	}

	strokeAlpha := uint8(float64(alpha) * 0.7)
	strokeCol := parseHexColor(wm.StrokeColor, strokeAlpha)
	if wm.StrokeColor == "" {
		strokeCol = color.RGBA{R: 0, G: 0, B: 0, A: strokeAlpha}
	}

	// Dynamic adaptive contrast
	if wm.AdaptiveColor != nil && *wm.AdaptiveColor {
		lum := computeAverageLuminance(dst)
		if lum > 135 {
			fontCol = color.RGBA{R: 24, G: 28, B: 36, A: alpha}
			strokeCol = color.RGBA{R: 255, G: 255, B: 255, A: strokeAlpha}
		} else {
			fontCol = color.RGBA{R: 248, G: 250, B: 252, A: alpha}
			strokeCol = color.RGBA{R: 15, G: 20, B: 30, A: strokeAlpha}
		}
	}

	// Resolution adaptive density scaling with dampened curve for low-res readability
	scale := 1.0
	if wm.AdaptiveScaling == nil || *wm.AdaptiveScaling {
		maxDim := math.Max(float64(imgW), float64(imgH))
		if maxDim < 1920.0 {
			scale = math.Pow(maxDim/1920.0, 0.40)
			if scale < 0.65 {
				scale = 0.65
			}
		} else {
			scale = maxDim / 1920.0
			if scale > 2.5 {
				scale = 2.5
			}
		}
	}

	baseFontSize := wm.FontSize
	if baseFontSize <= 0 {
		baseFontSize = 24
	}
	effectiveFontSize := int(math.Round(float64(baseFontSize) * scale))
	minLegibleSize := 16
	if baseFontSize < 16 {
		minLegibleSize = baseFontSize
	}
	if effectiveFontSize < minLegibleSize {
		effectiveFontSize = minLegibleSize
	}

	baseSpacingX := wm.SpacingX
	if baseSpacingX <= 0 {
		baseSpacingX = 260
	}
	effectiveSpacingX := int(math.Round(float64(baseSpacingX) * scale))

	baseSpacingY := wm.SpacingY
	if baseSpacingY <= 0 {
		baseSpacingY = 140
	}
	effectiveSpacingY := int(math.Round(float64(baseSpacingY) * scale))

	effectiveStrokeWidth := int(math.Round(float64(wm.StrokeWidth) * scale))
	if wm.StrokeWidth > 0 && effectiveStrokeWidth < 1 {
		effectiveStrokeWidth = 1
	}

	badge := createTextBadge(wm.Text, wm.FontFamily, effectiveFontSize, fontCol, strokeCol, effectiveStrokeWidth)

	angle := wm.Angle
	if angle == 0 && wm.Layout == "tile" {
		angle = -30
	}

	// Rotated lattice vector covering
	diag := math.Hypot(float64(imgW), float64(imgH))
	rad := angle * math.Pi / 180.0
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)

	ux, uy := cosA, sinA
	vx, vy := -sinA, cosA

	cx := float64(imgW) / 2.0
	cy := float64(imgH) / 2.0

	span := diag/2.0 + float64(effectiveSpacingX+effectiveSpacingY)
	badgeW := float64(badge.Bounds().Dx())
	badgeH := float64(badge.Bounds().Dy())

	stepY := float64(effectiveSpacingY)
	if stepY < badgeH+24 {
		stepY = badgeH + math.Max(24, float64(effectiveSpacingY)*0.25)
	}

	stepX := float64(effectiveSpacingX)
	if stepX < badgeW+36 {
		stepX = badgeW + math.Max(36, float64(effectiveSpacingX)*0.25)
	}

	for vDist := -span; vDist <= span; vDist += stepY {
		rowIndex := int(math.Round(vDist / stepY))
		stagger := float64(rowIndex%2) * (stepX / 2.0)

		for uDist := -span + stagger; uDist <= span+stagger; uDist += stepX {
			posX := cx + uDist*ux + vDist*vx
			posY := cy + uDist*uy + vDist*vy

			if posX >= -float64(effectiveSpacingX) && posX <= float64(imgW+effectiveSpacingX) &&
				posY >= -float64(effectiveSpacingY) && posY <= float64(imgH+effectiveSpacingY) {
				drawRotated(dst, badge, int(posX), int(posY), angle)
			}
		}
	}
}

func (p *PureGoProcessor) renderPositionText(dst *image.RGBA, wm *WatermarkConfig) {
	b := dst.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()

	opacity := wm.Opacity
	if opacity <= 0 {
		opacity = 0.8
	}
	alpha := uint8(opacity * 255)

	fontCol := parseHexColor(wm.FontColor, alpha)
	if wm.FontColor == "" {
		fontCol = color.RGBA{R: 255, G: 255, B: 255, A: alpha}
	}

	strokeAlpha := uint8(float64(alpha) * 0.7)
	strokeCol := parseHexColor(wm.StrokeColor, strokeAlpha)
	if wm.StrokeColor == "" {
		strokeCol = color.RGBA{R: 0, G: 0, B: 0, A: strokeAlpha}
	}

	if wm.AdaptiveColor != nil && *wm.AdaptiveColor {
		lum := computeAverageLuminance(dst)
		if lum > 135 {
			fontCol = color.RGBA{R: 24, G: 28, B: 36, A: alpha}
			strokeCol = color.RGBA{R: 255, G: 255, B: 255, A: strokeAlpha}
		} else {
			fontCol = color.RGBA{R: 248, G: 250, B: 252, A: alpha}
			strokeCol = color.RGBA{R: 15, G: 20, B: 30, A: strokeAlpha}
		}
	}

	scale := 1.0
	if wm.AdaptiveScaling == nil || *wm.AdaptiveScaling {
		maxDim := math.Max(float64(imgW), float64(imgH))
		if maxDim < 1920.0 {
			scale = math.Pow(maxDim/1920.0, 0.40)
			if scale < 0.65 {
				scale = 0.65
			}
		} else {
			scale = maxDim / 1920.0
			if scale > 2.5 {
				scale = 2.5
			}
		}
	}

	baseFontSize := wm.FontSize
	if baseFontSize <= 0 {
		baseFontSize = 24
	}
	effectiveFontSize := int(math.Round(float64(baseFontSize) * scale))
	minLegibleSize := 16
	if baseFontSize < 16 {
		minLegibleSize = baseFontSize
	}
	if effectiveFontSize < minLegibleSize {
		effectiveFontSize = minLegibleSize
	}

	effectiveStrokeWidth := int(math.Round(float64(wm.StrokeWidth) * scale))
	if wm.StrokeWidth > 0 && effectiveStrokeWidth < 1 {
		effectiveStrokeWidth = 1
	}

	badge := createTextBadge(wm.Text, wm.FontFamily, effectiveFontSize, fontCol, strokeCol, effectiveStrokeWidth)
	bw := badge.Bounds().Dx()
	bh := badge.Bounds().Dy()

	margin := int(math.Round(24.0 * scale))
	var x, y int

	switch wm.Position {
	case "top-left":
		x = margin
		y = margin
	case "top-center":
		x = (b.Dx() - bw) / 2
		y = margin
	case "top-right":
		x = b.Dx() - bw - margin
		y = margin
	case "center-left":
		x = margin
		y = (b.Dy() - bh) / 2
	case "center":
		x = (b.Dx() - bw) / 2
		y = (b.Dy() - bh) / 2
	case "center-right":
		x = b.Dx() - bw - margin
		y = (b.Dy() - bh) / 2
	case "bottom-left":
		x = margin
		y = b.Dy() - bh - margin
	case "bottom-center":
		x = (b.Dx() - bw) / 2
		y = b.Dy() - bh - margin
	case "bottom-right":
		fallthrough
	default:
		x = b.Dx() - bw - margin
		y = b.Dy() - bh - margin
	}

	drawRotated(dst, badge, x+bw/2, y+bh/2, 0)
}

func (p *PureGoProcessor) renderWatermarkImage(dst *image.RGBA, wmImg image.Image, position string, opacity float64) {
	if opacity <= 0 {
		opacity = 0.8
	}

	b := dst.Bounds()
	wB := wmImg.Bounds()

	margin := 24
	var x, y int

	switch position {
	case "top-left":
		x = margin
		y = margin
	case "top-right":
		x = b.Dx() - wB.Dx() - margin
		y = margin
	case "center":
		x = (b.Dx() - wB.Dx()) / 2
		y = (b.Dy() - wB.Dy()) / 2
	case "bottom-left":
		x = margin
		y = b.Dy() - wB.Dy() - margin
	case "bottom-right":
		fallthrough
	default:
		x = b.Dx() - wB.Dx() - margin
		y = b.Dy() - wB.Dy() - margin
	}

	// Alpha blend
	for wy := 0; wy < wB.Dy(); wy++ {
		dy := y + wy
		if dy < 0 || dy >= b.Dy() {
			continue
		}
		for wx := 0; wx < wB.Dx(); wx++ {
			dx := x + wx
			if dx < 0 || dx >= b.Dx() {
				continue
			}

			sr, sg, sb, sa := wmImg.At(wB.Min.X+wx, wB.Min.Y+wy).RGBA()
			if sa == 0 {
				continue
			}

			alphaFactor := opacity * (float64(sa) / 65535.0)
			sR := uint32(float64(sr>>8) * alphaFactor)
			sG := uint32(float64(sg>>8) * alphaFactor)
			sB := uint32(float64(sb>>8) * alphaFactor)
			sA := uint32(alphaFactor * 255.0)

			dstIdx := dy*dst.Stride + dx*4
			dA := uint32(dst.Pix[dstIdx+3])
			dR := uint32(dst.Pix[dstIdx])
			dG := uint32(dst.Pix[dstIdx+1])
			dB := uint32(dst.Pix[dstIdx+2])

			invA := 255 - sA
			dst.Pix[dstIdx] = uint8((sR*sA + dR*invA) / 255)
			dst.Pix[dstIdx+1] = uint8((sG*sA + dG*invA) / 255)
			dst.Pix[dstIdx+2] = uint8((sB*sA + dB*invA) / 255)
			dst.Pix[dstIdx+3] = uint8(sA + (dA*invA)/255)
		}
	}
}
