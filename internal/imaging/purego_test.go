package imaging

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecompressionBombProtection(t *testing.T) {
	proc := NewPureGoProcessor()

	// 1. Create a valid small image - should succeed
	smallImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for x := 0; x < 100; x++ {
		for y := 0; y < 100; y++ {
			smallImg.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, smallImg)
	assert.NoError(t, err)

	result, format, err := proc.Process(bytes.NewReader(buf.Bytes()), ProcessOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "image/png", format)
	assert.NotEmpty(t, result)

	// 2. Construct a mock PNG header with oversized dimensions (20000 x 20000)
	// PNG signature: 89 50 4E 47 0D 0A 1A 0A
	// IHDR chunk: length (13 bytes), "IHDR", width (4 bytes), height (4 bytes), bit depth (1), color type (1), ...
	var bombBuf bytes.Buffer
	bombBuf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})

	ihdrData := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdrData[0:4], 20000) // width > 16384
	binary.BigEndian.PutUint32(ihdrData[4:8], 20000) // height > 16384
	ihdrData[8] = 8                                  // bit depth
	ihdrData[9] = 2                                  // color type: truecolor
	ihdrData[10] = 0
	ihdrData[11] = 0
	ihdrData[12] = 0

	var chunkLen [4]byte
	binary.BigEndian.PutUint32(chunkLen[:], uint32(len(ihdrData)))
	bombBuf.Write(chunkLen[:])
	bombBuf.WriteString("IHDR")
	bombBuf.Write(ihdrData)

	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdrData)
	var crcBytes [4]byte
	binary.BigEndian.PutUint32(crcBytes[:], crc.Sum32())
	bombBuf.Write(crcBytes[:])

	// Try processing decompression bomb header
	_, _, err = proc.Process(bytes.NewReader(bombBuf.Bytes()), ProcessOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds safe limit")
}
