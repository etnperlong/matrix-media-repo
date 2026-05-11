package i

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
)

func TestBuildRsvgConvertArgsKeepsSmallSourceSize(t *testing.T) {
	assert.Equal(t, []string{
		"-f", "png",
		"-o", "/tmp/output.png",
		"/tmp/input.svg",
	}, buildRsvgConvertArgs("/tmp/input.svg", "/tmp/output.png", 100, 50))
}

func TestBuildRsvgConvertArgsClampsLargeSourceSize(t *testing.T) {
	assert.Equal(t, []string{
		"-f", "png",
		"-w", "4096",
		"-h", "2048",
		"-o", "/tmp/output.png",
		"/tmp/input.svg",
	}, buildRsvgConvertArgs("/tmp/input.svg", "/tmp/output.png", 8000, 4000))
}

func TestParseJXLInfoDimensions(t *testing.T) {
	width, height, err := parseJXLInfoDimensions([]byte("JPEG XL image, 3x2, lossy, 8-bit RGB\nColor space: RGB, D65, sRGB primaries, sRGB transfer function, rendering intent: Relative\n"))
	require.NoError(t, err)
	assert.Equal(t, 3, width)
	assert.Equal(t, 2, height)
}

func TestParseJXLInfoDimensionsRejectsUnexpectedOutput(t *testing.T) {
	_, _, err := parseJXLInfoDimensions([]byte("not a jxlinfo response"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected jxlinfo output")
}

func TestSvgGeneratorReturnsHelpfulErrorWhenToolMissing(t *testing.T) {
	originalLookPath := commandLookPath
	originalCombinedOutput := commandCombinedOutput
	t.Cleanup(func() {
		commandLookPath = originalLookPath
		commandCombinedOutput = originalCombinedOutput
	})

	commandLookPath = func(name string) (string, error) {
		return "", exec.ErrNotFound
	}

	_, err := svgGenerator{}.GenerateThumbnail(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`), "image/svg+xml", 1, 1, "scale", false, rcontext.InitialNoConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rsvg-convert not found")
	assert.Contains(t, err.Error(), "slim/full container image")
}

func TestSvgGeneratorGetOriginDimensionsUsesViewBox(t *testing.T) {
	ok, width, height, err := svgGenerator{}.GetOriginDimensions(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 240"></svg>`), "image/svg+xml", rcontext.InitialNoConfig())
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 320, width)
	assert.Equal(t, 240, height)
}

func TestJpegxlGeneratorGetOriginDimensionsUsesJxlinfo(t *testing.T) {
	originalLookPath := commandLookPath
	originalCombinedOutput := commandCombinedOutput
	t.Cleanup(func() {
		commandLookPath = originalLookPath
		commandCombinedOutput = originalCombinedOutput
	})

	commandLookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	var called bool
	commandCombinedOutput = func(name string, args ...string) ([]byte, error) {
		called = true
		require.Equal(t, "jxlinfo", name)
		require.Len(t, args, 1)
		assert.True(t, strings.HasSuffix(args[0], ".jxl"))
		return []byte("JPEG XL image, 7x5, lossy, 8-bit RGB\n"), nil
	}

	ok, width, height, err := jpegxlGenerator{}.GetOriginDimensions(bytes.NewReader([]byte("test-jxl-data")), "image/jxl", rcontext.InitialNoConfig())
	require.NoError(t, err)
	assert.True(t, called)
	assert.True(t, ok)
	assert.Equal(t, 7, width)
	assert.Equal(t, 5, height)
}
