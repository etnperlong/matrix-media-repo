package i

import (
	"errors"
	"io"
	"os"
	"path"

	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/thumbnailing/m"
)

type jpegxlGenerator struct {
}

func (d jpegxlGenerator) supportedContentTypes() []string {
	return []string{"image/jxl"}
}

func (d jpegxlGenerator) supportsAnimation() bool {
	return false
}

func (d jpegxlGenerator) matches(img io.Reader, contentType string) bool {
	return contentType == "image/jxl"
}

func (d jpegxlGenerator) GetOriginDimensions(b io.Reader, contentType string, ctx rcontext.RequestContext) (bool, int, int, error) {
	dir, err := os.MkdirTemp(os.TempDir(), "mmr-jpegxl-info")
	if err != nil {
		return false, 0, 0, errors.New("jpegxl: error creating temporary directory: " + err.Error())
	}

	tempFile1 := path.Join(dir, "i.jxl")

	defer os.Remove(tempFile1)
	defer os.Remove(dir)

	f, err := os.OpenFile(tempFile1, os.O_RDWR|os.O_CREATE, 0640)
	if err != nil {
		return false, 0, 0, errors.New("jpegxl: error creating temp jpegxl file: " + err.Error())
	}
	if _, err = io.Copy(f, b); err != nil {
		_ = f.Close()
		return false, 0, 0, errors.New("jpegxl: error writing temp jpegxl file: " + err.Error())
	}
	if err = f.Close(); err != nil {
		return false, 0, 0, errors.New("jpegxl: error closing temp jpegxl file: " + err.Error())
	}

	output, err := runExternalToolOutput(jxlinfoTool, tempFile1)
	if err != nil {
		return false, 0, 0, errors.New("jpegxl: error reading jpegxl dimensions with jxlinfo: " + err.Error())
	}

	width, height, err := parseJXLInfoDimensions(output)
	if err != nil {
		return false, 0, 0, errors.New("jpegxl: error parsing jxlinfo output: " + err.Error())
	}

	return true, width, height, nil
}

func (d jpegxlGenerator) GenerateThumbnail(b io.Reader, contentType string, width int, height int, method string, animated bool, ctx rcontext.RequestContext) (*m.Thumbnail, error) {
	dir, err := os.MkdirTemp(os.TempDir(), "mmr-jpegxl")
	if err != nil {
		return nil, errors.New("jpegxl: error creating temporary directory: " + err.Error())
	}

	tempFile1 := path.Join(dir, "i.jxl")
	tempFile2 := path.Join(dir, "o.png")

	defer os.Remove(tempFile1)
	defer os.Remove(tempFile2)
	defer os.Remove(dir)

	f, err := os.OpenFile(tempFile1, os.O_RDWR|os.O_CREATE, 0640)
	if err != nil {
		return nil, errors.New("jpegxl: error creating temp jpegxl file: " + err.Error())
	}
	if _, err = io.Copy(f, b); err != nil {
		_ = f.Close()
		return nil, errors.New("jpegxl: error writing temp jpegxl file: " + err.Error())
	}
	if err = f.Close(); err != nil {
		return nil, errors.New("jpegxl: error closing temp jpegxl file: " + err.Error())
	}

	err = runExternalTool(djxlTool, tempFile1, tempFile2, "--quiet")
	if err != nil {
		return nil, errors.New("jpegxl: error converting jpegxl with djxl: " + err.Error())
	}

	f, err = os.OpenFile(tempFile2, os.O_RDONLY, 0640)
	if err != nil {
		return nil, errors.New("jpegxl: error reading temp png file: " + err.Error())
	}
	defer f.Close()

	return pngGenerator{}.GenerateThumbnail(f, "image/png", width, height, method, false, ctx)
}

func init() {
	generators = append(generators, jpegxlGenerator{})
}
