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
	img, format, err := image.Decode(src)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	origBounds := img.Bounds()
	origW := origBounds.Dx()
	origH := origBounds.Dy()

	targetW := opts.Width
	targetH := opts.Height

	// 1. Resize if target dimensions specified and smaller than original (never upscale)
	var processedImg image.Image = img
	if targetW > 0 || targetH > 0 {
		if targetW <= 0 {
			targetW = int(float64(targetH) * float64(origW) / float64(origH))
		}
		if targetH <= 0 {
			targetH = int(float64(targetW) * float64(origH) / float64(origW))
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
	case "webp", "jpeg", "jpg":
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
			// Slanted tile
			p.renderTileText(dst, wm.Text, wm.Opacity)
		} else {
			// Corner positioning (default bottom-right)
			p.renderCornerText(dst, wm.Text, wm.Position, wm.Opacity)
		}
	} else if wm.Type == "image" && len(wm.ImageBuf) > 0 {
		if wmImg, _, err := image.Decode(bytes.NewReader(wm.ImageBuf)); err == nil {
			p.renderWatermarkImage(dst, wmImg, wm.Position, wm.Opacity)
		}
	}

	return dst
}

func (p *PureGoProcessor) renderCornerText(dst *image.RGBA, text, position string, opacity float64) {
	col := color.RGBA{R: 255, G: 255, B: 255, A: uint8(opacity * 255)}
	if opacity <= 0 {
		col.A = 180
	}

	b := dst.Bounds()
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
	}

	textW := d.MeasureString(text).Ceil()
	textH := 13

	margin := 20
	var x, y int

	switch position {
	case "top-left":
		x = margin
		y = margin + textH
	case "top-right":
		x = b.Dx() - textW - margin
		y = margin + textH
	case "bottom-left":
		x = margin
		y = b.Dy() - margin
	case "center":
		x = (b.Dx() - textW) / 2
		y = (b.Dy() + textH) / 2
	case "bottom-right":
		fallthrough
	default:
		x = b.Dx() - textW - margin
		y = b.Dy() - margin
	}

	d.Dot = fixed.P(x, y)
	d.DrawString(text)
}

func (p *PureGoProcessor) renderTileText(dst *image.RGBA, text string, opacity float64) {
	col := color.RGBA{R: 255, G: 255, B: 255, A: uint8(opacity * 255)}
	if opacity <= 0 {
		col.A = 70 // faint watermark
	}

	b := dst.Bounds()
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
	}

	stepX := 160
	stepY := 80

	for y := 20; y < b.Dy()+50; y += stepY {
		for x := 10; x < b.Dx()+50; x += stepX {
			d.Dot = fixed.P(x, y)
			d.DrawString(text)
		}
	}
}

func (p *PureGoProcessor) renderWatermarkImage(dst *image.RGBA, wm image.Image, position string, opacity float64) {
	b := dst.Bounds()
	wmB := wm.Bounds()

	margin := 20
	var offset image.Point

	switch position {
	case "top-left":
		offset = image.Pt(margin, margin)
	case "top-right":
		offset = image.Pt(b.Dx()-wmB.Dx()-margin, margin)
	case "bottom-left":
		offset = image.Pt(margin, b.Dy()-wmB.Dy()-margin)
	case "center":
		offset = image.Pt((b.Dx()-wmB.Dx())/2, (b.Dy()-wmB.Dy())/2)
	case "bottom-right":
		fallthrough
	default:
		offset = image.Pt(b.Dx()-wmB.Dx()-margin, b.Dy()-wmB.Dy()-margin)
	}

	targetRect := image.Rectangle{Min: offset, Max: offset.Add(wmB.Size())}

	alpha := uint8(opacity * 255)
	if opacity <= 0 {
		alpha = 200
	}
	mask := image.NewUniform(color.Alpha{A: alpha})

	draw.DrawMask(dst, targetRect, wm, wmB.Min, mask, image.Point{}, draw.Over)
}

func Distance(x1, y1, x2, y2 int) float64 {
	return math.Sqrt(float64((x1-x2)*(x1-x2) + (y1-y2)*(y1-y2)))
}
