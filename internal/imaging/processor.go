package imaging

import (
	"io"
)

type WatermarkConfig struct {
	Enabled         bool    `json:"enabled"`
	Type            string  `json:"type"` // "text" or "image"
	Text            string  `json:"text"`
	ImageBuf        []byte  `json:"-"`
	Layout          string  `json:"layout"` // "tile" (slant) or "position" (corner)
	Position        string  `json:"position"`
	Opacity         float64 `json:"opacity"` // 0.0 - 1.0
	Angle           float64 `json:"angle"`   // degrees, e.g. -30 for slant
	SpacingX        int     `json:"spacing_x"`
	SpacingY        int     `json:"spacing_y"`
	FontSize        int     `json:"font_size"`
	FontFamily      string  `json:"font_family"` // "yahei", "simhei", "simsun", "arial", "default"
	FontColor       string  `json:"font_color"`  // hex e.g. "#ffffff"
	StrokeColor     string  `json:"stroke_color"`
	StrokeWidth     int     `json:"stroke_width"`
	AdaptiveScaling *bool   `json:"adaptive_scaling"` // auto-scale density with image resolution, default true
	AdaptiveColor   *bool   `json:"adaptive_color"`   // auto-adapt text color/contrast according to image luminance
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
