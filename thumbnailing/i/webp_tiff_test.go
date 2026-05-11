package i

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"golang.org/x/image/tiff"
)

const minimalWebP = "UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoBAAEAAgA0JaQAA3AA/vuUAAA="

func makeTestTIFF(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(20 * x), G: uint8(20 * y), B: 0x80, A: 0xff})
		}
	}

	buf := bytes.NewBuffer(nil)
	require.NoError(t, tiff.Encode(buf, img, nil))
	return buf.Bytes()
}

func TestWebPGeneratorGetOriginDimensions(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(minimalWebP)
	require.NoError(t, err)

	ok, width, height, err := webpGenerator{}.GetOriginDimensions(bytes.NewReader(data), "image/webp", rcontext.InitialNoConfig())
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 1, width)
	assert.Equal(t, 1, height)
}

func TestWebPGeneratorGenerateThumbnail(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(minimalWebP)
	require.NoError(t, err)

	thumb, err := webpGenerator{}.GenerateThumbnail(bytes.NewReader(data), "image/webp", 1, 1, "scale", false, rcontext.InitialNoConfig())
	require.NoError(t, err)
	require.NotNil(t, thumb)
	defer thumb.Reader.Close()
	body, err := io.ReadAll(thumb.Reader)
	require.NoError(t, err)

	assert.Equal(t, "image/png", thumb.ContentType)
	assert.False(t, thumb.Animated)
	assert.NotEmpty(t, body)
}

func TestTiffGeneratorGetOriginDimensions(t *testing.T) {
	data := makeTestTIFF(t, 3, 2)

	ok, width, height, err := tiffGenerator{}.GetOriginDimensions(bytes.NewReader(data), "image/tiff", rcontext.InitialNoConfig())
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 3, width)
	assert.Equal(t, 2, height)
}

func TestTiffGeneratorGenerateThumbnail(t *testing.T) {
	data := makeTestTIFF(t, 3, 2)

	thumb, err := tiffGenerator{}.GenerateThumbnail(bytes.NewReader(data), "image/tiff", 2, 2, "scale", false, rcontext.InitialNoConfig())
	require.NoError(t, err)
	require.NotNil(t, thumb)
	defer thumb.Reader.Close()
	body, err := io.ReadAll(thumb.Reader)
	require.NoError(t, err)

	assert.Equal(t, "image/png", thumb.ContentType)
	assert.False(t, thumb.Animated)
	assert.NotEmpty(t, body)
}
