package imaging

import (
	"io"
)

type WatermarkConfig struct {
	Enabled   bool
	Type      string // "text" or "image"
	Text      string
	ImageBuf  []byte
	Layout    string  // "tile" (slant) or "position" (corner)
	Position  string  // "top-left", "top-right", "bottom-left", "bottom-right", "center"
	Opacity   float64 // 0.0 - 1.0
	Angle     float64 // e.g. -30 for slant
	SpacingX  int
	SpacingY  int
	FontSize  int
	FontColor string // hex e.g. "#ffffff"
}

type ProcessOptions struct {
	Width     int
	Height    int
	Fit       string // "contain", "cover", "fill"
	Format    string // "webp", "jpeg", "png"
	Quality   int    // 1 - 100
	Watermark *WatermarkConfig
}

type ImageProcessor interface {
	Process(src io.Reader, opts ProcessOptions) ([]byte, string, error)
	GetDimensions(src io.Reader) (width, height int, err error)
}
