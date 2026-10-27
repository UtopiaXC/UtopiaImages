package imaging

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"math/cmplx"
	"runtime"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// fftRadix2 computes an in-place Radix-2 Cooley-Tukey FFT for slice length power of 2
func fftRadix2(a []complex128, inverse bool) {
	n := len(a)
	if n <= 1 {
		return
	}

	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}

	sign := -1.0
	if inverse {
		sign = 1.0
	}

	for length := 2; length <= n; length <<= 1 {
		half := length >> 1
		angle := sign * 2.0 * math.Pi / float64(length)
		wStep := cmplx.Rect(1, angle)

		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for k := 0; k < half; k++ {
				u := a[i+k]
				v := a[i+k+half] * w
				a[i+k] = u + v
				a[i+k+half] = u - v
				w *= wStep
			}
		}
	}

	if inverse {
		invN := complex(1.0/float64(n), 0)
		for i := 0; i < n; i++ {
			a[i] *= invN
		}
	}
}

func nextPowerOf2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// fftBluestein computes FFT for arbitrary slice length N in O(N log N) using Bluestein's chirp-z algorithm
func fftBluestein(x []complex128, inverse bool) []complex128 {
	n := len(x)
	if n <= 1 {
		res := make([]complex128, n)
		copy(res, x)
		return res
	}

	// If n is already a power of 2, use fast Radix-2 directly
	if (n & (n - 1)) == 0 {
		res := make([]complex128, n)
		copy(res, x)
		fftRadix2(res, inverse)
		return res
	}

	sign := -1.0
	if inverse {
		sign = 1.0
	}

	m := nextPowerOf2(2*n - 1)
	a := make([]complex128, m)
	b := make([]complex128, m)
	w := make([]complex128, n)

	for i := 0; i < n; i++ {
		angle := sign * math.Pi * float64(i*i) / float64(n)
		w[i] = cmplx.Rect(1, angle)
		a[i] = x[i] * w[i]
	}

	b[0] = cmplx.Conj(w[0])
	for i := 1; i < n; i++ {
		cw := cmplx.Conj(w[i])
		b[i] = cw
		b[m-i] = cw
	}

	fftRadix2(a, false)
	fftRadix2(b, false)

	for i := 0; i < m; i++ {
		a[i] *= b[i]
	}

	fftRadix2(a, true)

	res := make([]complex128, n)
	scale := complex(1.0, 0)
	if inverse {
		scale = complex(1.0/float64(n), 0)
	}

	for i := 0; i < n; i++ {
		res[i] = a[i] * w[i] * scale
	}

	return res
}

// fft2D computes parallel 2D FFT/IFFT across rows and columns using worker pools
func fft2D(grid [][]complex128, inverse bool) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	if w == 0 {
		return
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	// 1. Parallelize Rows
	var wg sync.WaitGroup
	rowChunk := (h + numWorkers - 1) / numWorkers
	for i := 0; i < numWorkers; i++ {
		start := i * rowChunk
		end := start + rowChunk
		if start >= h {
			break
		}
		if end > h {
			end = h
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for y := s; y < e; y++ {
				grid[y] = fftBluestein(grid[y], inverse)
			}
		}(start, end)
	}
	wg.Wait()

	// 2. Parallelize Columns
	colChunk := (w + numWorkers - 1) / numWorkers
	for i := 0; i < numWorkers; i++ {
		start := i * colChunk
		end := start + colChunk
		if start >= w {
			break
		}
		if end > w {
			end = w
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			col := make([]complex128, h)
			for x := s; x < e; x++ {
				for y := 0; y < h; y++ {
					col[y] = grid[y][x]
				}
				colRes := fftBluestein(col, inverse)
				for y := 0; y < h; y++ {
					grid[y][x] = colRes[y]
				}
			}
		}(start, end)
	}
	wg.Wait()
}

// fftShift moves zero frequency from (0, 0) to center (h/2, w/2)
func fftShift(grid [][]complex128) [][]complex128 {
	h := len(grid)
	w := len(grid[0])
	shifted := make([][]complex128, h)
	for i := range shifted {
		shifted[i] = make([]complex128, w)
	}

	halfH := h / 2
	halfW := w / 2

	for y := 0; y < h; y++ {
		ny := (y + halfH) % h
		for x := 0; x < w; x++ {
			nx := (x + halfW) % w
			shifted[ny][nx] = grid[y][x]
		}
	}
	return shifted
}

// ifftShift moves zero frequency from center (h/2, w/2) back to (0, 0)
func ifftShift(grid [][]complex128) [][]complex128 {
	h := len(grid)
	w := len(grid[0])
	shifted := make([][]complex128, h)
	for i := range shifted {
		shifted[i] = make([]complex128, w)
	}

	halfH := (h + 1) / 2
	halfW := (w + 1) / 2

	for y := 0; y < h; y++ {
		ny := (y + halfH) % h
		for x := 0; x < w; x++ {
			nx := (x + halfW) % w
			shifted[ny][nx] = grid[y][x]
		}
	}
	return shifted
}

// coordPhase generates deterministic pseudo-random phase for phase dispersion
func coordPhase(y, x int) float64 {
	h := uint32(y)*31337 ^ uint32(x)*7001
	h = h ^ (h >> 16)
	h = h * 0x85ebca6b
	h = h ^ (h >> 13)
	val := float64(h%10000) / 10000.0
	return val*2.0*math.Pi - math.Pi
}

// EmbedBlindWatermark embeds a 2D Fourier spectrum blind watermark into the image.
// When viewed normally, spatial modifications are imperceptible (delta <= 8.5).
// When taking cv2.dft + np.fft.fftshift + 20*log(magnitude + 1), the watermark text appears clearly.
func EmbedBlindWatermark(src image.Image, watermarkText string) *image.RGBA {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	if w < 16 || h < 16 || watermarkText == "" {
		return dst
	}

	fontParsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return dst
	}

	// Measure text at baseline size 20 to compute optimal font scale
	faceBase, err := opentype.NewFace(fontParsed, &opentype.FaceOptions{Size: 20, DPI: 72})
	if err != nil {
		return dst
	}
	dBase := &font.Drawer{Face: faceBase}
	wBase := dBase.MeasureString(watermarkText).Ceil()
	if wBase <= 0 {
		return dst
	}

	// Fit text horizontally into ~70% of image width
	targetW := float64(w) * 0.70
	fontSize := math.Round(20.0 * targetW / float64(wBase))
	if fontSize < 14 {
		fontSize = 14
	} else if fontSize > 48 {
		fontSize = 48
	}

	face, err := opentype.NewFace(fontParsed, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return dst
	}

	// Render text to mask
	textMask := image.NewRGBA(image.Rect(0, 0, w, h))
	d := &font.Drawer{
		Dst:  textMask,
		Src:  image.White,
		Face: face,
	}
	textW := d.MeasureString(watermarkText).Ceil()

	posX := (w - textW) / 2
	if posX < 5 {
		posX = 5
	}
	posY := h / 4
	d.Dot = fixed.P(posX, posY)
	d.DrawString(watermarkText)

	// Frequency domain grid
	cy := h / 2
	cx := w / 2
	wmShifted := make([][]complex128, h)
	for y := 0; y < h; y++ {
		wmShifted[y] = make([]complex128, w)
	}

	// Populate frequency grid with Hermitian conjugate symmetry and phase dispersion
	for y := 0; y < cy; y++ {
		for x := 0; x < w; x++ {
			idx := y*textMask.Stride + x*4
			alpha := float64(textMask.Pix[idx]) / 255.0
			if alpha > 0.05 {
				dy := y - cy
				dx := x - cx
				symY := cy - dy
				symX := cx - dx

				theta := coordPhase(y, x)
				c := cmplx.Rect(alpha, theta)
				cSym := cmplx.Rect(alpha, -theta)

				wmShifted[y][x] = c
				if symY >= 0 && symY < h && symX >= 0 && symX < w {
					wmShifted[symY][symX] = cSym
				}
			}
		}
	}

	// Transform back to spatial domain via 2D IDFT
	wmUnshifted := ifftShift(wmShifted)
	fft2D(wmUnshifted, true)

	// Find peak spatial amplitude to calibrate visibility
	maxAbs := 0.0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := math.Abs(real(wmUnshifted[y][x]))
			if v > maxAbs {
				maxAbs = v
			}
		}
	}

	if maxAbs <= 0 {
		return dst
	}

	// Calibrate target peak delta to ~8.0 for crisp spectrum recognition
	targetPeakDelta := 8.0
	scale := targetPeakDelta / maxAbs

	// Blend spatial perturbation into RGB channels
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := (y)*dst.Stride + (x)*4
			r := float64(dst.Pix[idx])
			g := float64(dst.Pix[idx+1])
			b := float64(dst.Pix[idx+2])

			delta := real(wmUnshifted[y][x]) * scale

			nR := math.Round(r + delta)
			nG := math.Round(g + delta)
			nB := math.Round(b + delta)

			if nR < 0 {
				nR = 0
			} else if nR > 255 {
				nR = 255
			}
			if nG < 0 {
				nG = 0
			} else if nG > 255 {
				nG = 255
			}
			if nB < 0 {
				nB = 0
			} else if nB > 255 {
				nB = 255
			}

			dst.Pix[idx] = uint8(nR)
			dst.Pix[idx+1] = uint8(nG)
			dst.Pix[idx+2] = uint8(nB)
		}
	}

	return dst
}

// ExtractBlindWatermark computes the 2D FFT magnitude spectrum of the input image,
// normalizes it into a grayscale PNG image, and returns a base64 data URI for direct visualization.
func ExtractBlindWatermark(src image.Image) (string, string, error) {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w < 16 || h < 16 {
		return "", "", errors.New("image dimensions too small for blind watermark extraction")
	}

	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, src, bounds.Min, draw.Src)
	}

	// Extract grayscale luminance for 2D FFT
	grid := make([][]complex128, h)
	for y := 0; y < h; y++ {
		grid[y] = make([]complex128, w)
		for x := 0; x < w; x++ {
			idx := (y)*rgba.Stride + (x)*4
			gray := 0.299*float64(rgba.Pix[idx]) + 0.587*float64(rgba.Pix[idx+1]) + 0.114*float64(rgba.Pix[idx+2])
			grid[y][x] = complex(gray, 0)
		}
	}

	// Forward 2D FFT & shift DC to center
	fft2D(grid, false)
	shifted := fftShift(grid)

	// Compute log magnitude spectrum: 20 * ln(|F| + 1)
	maxMag := 0.0
	minMag := 1e9
	magGrid := make([][]float64, h)
	for y := 0; y < h; y++ {
		magGrid[y] = make([]float64, w)
		for x := 0; x < w; x++ {
			mag := 20.0 * math.Log(cmplx.Abs(shifted[y][x])+1.0)
			magGrid[y][x] = mag
			if mag > maxMag {
				maxMag = mag
			}
			if mag < minMag {
				minMag = mag
			}
		}
	}

	// Normalize spectrum to 8-bit grayscale image
	diff := maxMag - minMag
	if diff <= 0 {
		diff = 1.0
	}

	specImg := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			norm := (magGrid[y][x] - minMag) / diff
			val := uint8(math.Min(255, math.Max(0, norm*255.0)))
			specImg.Set(x, y, color.RGBA{val, val, val, 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, specImg); err != nil {
		return "", "", err
	}

	specBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	msg := "二维傅里叶变换 (FFT) 频谱图已生成，隐藏的盲水印已显现在频谱图中。亦可使用 Python OpenCV 脚本直接查看。"

	return msg, specBase64, nil
}
