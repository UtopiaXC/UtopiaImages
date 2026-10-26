package imaging

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

var (
	fontCacheLock sync.RWMutex
	parsedFonts   = make(map[string]*sfnt.Font)
)

// FontManager loads and caches TrueType/OpenType fonts for crisp rendering
type FontManager struct{}

var defaultFontManager = &FontManager{}

func GetFontManager() *FontManager {
	return defaultFontManager
}

func (fm *FontManager) GetFace(fontFamily string, fontSize int) (font.Face, error) {
	if fontSize <= 0 {
		fontSize = 24
	}

	sfntFont := fm.resolveFont(fontFamily)
	if sfntFont == nil {
		// Fallback to embedded Go Regular
		var err error
		sfntFont, err = opentype.Parse(goregular.TTF)
		if err != nil {
			return nil, err
		}
	}

	return opentype.NewFace(sfntFont, &opentype.FaceOptions{
		Size:    float64(fontSize),
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func (fm *FontManager) resolveFont(family string) *sfnt.Font {
	family = strings.ToLower(strings.TrimSpace(family))

	fontCacheLock.RLock()
	if f, exists := parsedFonts[family]; exists {
		fontCacheLock.RUnlock()
		return f
	}
	fontCacheLock.RUnlock()

	var candidatePaths []string

	switch family {
	case "yahei", "msyh", "微软雅黑":
		candidatePaths = []string{
			`C:\Windows\Fonts\msyh.ttc`,
			`C:\Windows\Fonts\msyh.ttf`,
			`data/fonts/msyh.ttc`,
			`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`,
			`/usr/share/fonts/truetype/wqy/wqy-microhei.ttc`,
		}
	case "simhei", "黑体":
		candidatePaths = []string{
			`C:\Windows\Fonts\simhei.ttf`,
			`data/fonts/simhei.ttf`,
			`/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc`,
		}
	case "simsun", "宋体":
		candidatePaths = []string{
			`C:\Windows\Fonts\simsun.ttc`,
			`data/fonts/simsun.ttc`,
		}
	case "arial":
		candidatePaths = []string{
			`C:\Windows\Fonts\arial.ttf`,
			`/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`,
		}
	default:
		// Default: try Microsoft YaHei for complete Chinese/English glyph support
		candidatePaths = []string{
			`C:\Windows\Fonts\msyh.ttc`,
			`C:\Windows\Fonts\msyh.ttf`,
			`data/fonts/msyh.ttc`,
			`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`,
			`/usr/share/fonts/truetype/wqy/wqy-microhei.ttc`,
			`C:\Windows\Fonts\arial.ttf`,
		}
	}

	// Also check if family is a direct file path
	if _, err := os.Stat(family); err == nil {
		candidatePaths = append([]string{family}, candidatePaths...)
	}

	for _, path := range candidatePaths {
		if _, err := os.Stat(path); err == nil {
			if fontObj, err := loadFontFile(path); err == nil && fontObj != nil {
				fontCacheLock.Lock()
				parsedFonts[family] = fontObj
				fontCacheLock.Unlock()
				return fontObj
			}
		}
	}

	// If no system font found, parse embedded Go Regular
	if f, err := opentype.Parse(goregular.TTF); err == nil {
		fontCacheLock.Lock()
		parsedFonts[family] = f
		fontCacheLock.Unlock()
		return f
	}

	return nil
}

func loadFontFile(filePath string) (*sfnt.Font, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".ttc" {
		col, err := sfnt.ParseCollection(data)
		if err == nil && col.NumFonts() > 0 {
			return col.Font(0)
		}
	}

	return opentype.Parse(data)
}
