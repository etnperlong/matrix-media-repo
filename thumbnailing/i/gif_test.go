package i

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/config"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
)

func gifTestContext(maxAnimateBytes int64, maxPixels int) rcontext.RequestContext {
	ctx := rcontext.InitialNoConfig()
	ctx.Config = config.NewDefaultDomainConfig()
	ctx.Config.Thumbnails.MaxAnimateSizeBytes = maxAnimateBytes
	ctx.Config.Thumbnails.MaxPixels = maxPixels
	return ctx
}

func makeTestGIF(t *testing.T, logicalWidth int, logicalHeight int, frameWidth int, frameHeight int, frames int) []byte {
	t.Helper()
	g := &gif.GIF{
		Image: make([]*image.Paletted, 0, frames),
		Delay: make([]int, 0, frames),
		Config: image.Config{
			ColorModel: color.Palette{color.Black, color.White},
			Width:      logicalWidth,
			Height:     logicalHeight,
		},
	}
	for i := 0; i < frames; i++ {
		x := 0
		y := 0
		if logicalWidth > frameWidth {
			x = i % (logicalWidth - frameWidth + 1)
		}
		if logicalHeight > frameHeight {
			y = i % (logicalHeight - frameHeight + 1)
		}
		img := image.NewPaletted(image.Rect(x, y, x+frameWidth, y+frameHeight), color.Palette{color.Black, color.White})
		img.SetColorIndex(x, y, uint8(i%2))
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 1)
	}
	buf := bytes.NewBuffer(nil)
	require.NoError(t, gif.EncodeAll(buf, g))
	return buf.Bytes()
}

func TestGifGeneratorKeepsSmallAnimatedGIFsAnimated(t *testing.T) {
	data := makeTestGIF(t, 4, 4, 4, 4, 2)

	thumb, err := gifGenerator{}.GenerateThumbnail(bytes.NewReader(data), "image/gif", 2, 2, "scale", true, gifTestContext(int64(len(data)+1024), 32000000))
	require.NoError(t, err)
	require.NotNil(t, thumb)
	defer thumb.Reader.Close()

	assert.True(t, thumb.Animated)
	assert.Equal(t, "image/gif", thumb.ContentType)
}

func TestGifGeneratorFallsBackToStillFrameWhenAnimatedBytesExceedLimit(t *testing.T) {
	data := makeTestGIF(t, 4, 4, 4, 4, 2)

	thumb, err := gifGenerator{}.GenerateThumbnail(bytes.NewReader(data), "image/gif", 2, 2, "scale", true, gifTestContext(1, 32000000))
	require.NoError(t, err)
	require.NotNil(t, thumb)
	defer thumb.Reader.Close()

	assert.False(t, thumb.Animated)
	assert.Equal(t, "image/png", thumb.ContentType)
}

func TestGifGeneratorFallsBackToStillFrameWhenAnimatedFrameCountExceedsLimit(t *testing.T) {
	data := makeTestGIF(t, 2, 2, 2, 2, 257)

	thumb, err := gifGenerator{}.GenerateThumbnail(bytes.NewReader(data), "image/gif", 1, 1, "scale", true, gifTestContext(0, 32000000))
	require.NoError(t, err)
	require.NotNil(t, thumb)
	defer thumb.Reader.Close()

	assert.False(t, thumb.Animated)
	assert.Equal(t, "image/png", thumb.ContentType)
}

func TestGifGeneratorFallsBackToStillFrameWhenLogicalCanvasComplexityExceedsLimit(t *testing.T) {
	data := makeTestGIF(t, 100, 100, 1, 1, 4)

	thumb, err := gifGenerator{}.GenerateThumbnail(bytes.NewReader(data), "image/gif", 10, 10, "scale", true, gifTestContext(int64(len(data)+1024), 30000))
	require.NoError(t, err)
	require.NotNil(t, thumb)
	defer thumb.Reader.Close()

	assert.False(t, thumb.Animated)
	assert.Equal(t, "image/png", thumb.ContentType)
}
