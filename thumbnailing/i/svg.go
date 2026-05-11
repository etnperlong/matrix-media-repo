package i

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/thumbnailing/m"
)

type svgRoot struct {
	Width   string `xml:"width,attr"`
	Height  string `xml:"height,attr"`
	ViewBox string `xml:"viewBox,attr"`
}

var svgLengthPattern = regexp.MustCompile(`^\s*([+-]?(?:\d+\.\d+|\d+|\.\d+))(px|in|cm|mm|pt|pc|q)?\s*$`)

type svgGenerator struct {
}

func (d svgGenerator) supportedContentTypes() []string {
	return []string{"image/svg+xml"}
}

func (d svgGenerator) supportsAnimation() bool {
	return false
}

func (d svgGenerator) matches(img io.Reader, contentType string) bool {
	return contentType == "image/svg+xml"
}

func (d svgGenerator) GetOriginDimensions(b io.Reader, contentType string, ctx rcontext.RequestContext) (bool, int, int, error) {
	return getSVGOriginDimensions(b)
}

func (d svgGenerator) GenerateThumbnail(b io.Reader, contentType string, width int, height int, method string, animated bool, ctx rcontext.RequestContext) (*m.Thumbnail, error) {
	dir, err := os.MkdirTemp(os.TempDir(), "mmr-svg")
	if err != nil {
		return nil, errors.New("svg: error creating temporary directory: " + err.Error())
	}

	tempFile1 := path.Join(dir, "i.svg")
	tempFile2 := path.Join(dir, "o.png")

	defer os.Remove(tempFile1)
	defer os.Remove(tempFile2)
	defer os.Remove(dir)

	f, err := os.OpenFile(tempFile1, os.O_RDWR|os.O_CREATE, 0640)
	if err != nil {
		return nil, errors.New("svg: error creating temp svg file: " + err.Error())
	}
	if _, err = io.Copy(f, b); err != nil {
		_ = f.Close()
		return nil, errors.New("svg: error writing temp svg file: " + err.Error())
	}
	if err = f.Close(); err != nil {
		return nil, errors.New("svg: error closing temp svg file: " + err.Error())
	}

	svgFile, err := os.Open(tempFile1)
	if err != nil {
		return nil, errors.New("svg: error reopening temp svg file: " + err.Error())
	}
	dimensional, sourceWidth, sourceHeight, err := getSVGOriginDimensions(svgFile)
	_ = svgFile.Close()
	if err != nil {
		return nil, errors.New("svg: error reading svg dimensions: " + err.Error())
	}

	args := buildRsvgConvertArgs(tempFile1, tempFile2, sourceWidth, sourceHeight)
	if !dimensional {
		args = []string{"-f", "png", "-o", tempFile2, tempFile1}
	}
	err = runExternalTool(rsvgConvertTool, args...)
	if err != nil {
		return nil, errors.New("svg: error converting svg with rsvg-convert: " + err.Error())
	}

	f, err = os.OpenFile(tempFile2, os.O_RDONLY, 0640)
	if err != nil {
		return nil, errors.New("svg: error reading temp png file: " + err.Error())
	}
	defer f.Close()

	return pngGenerator{}.GenerateThumbnail(f, "image/png", width, height, method, false, ctx)
}

func init() {
	generators = append(generators, svgGenerator{})
}

func getSVGOriginDimensions(r io.Reader) (bool, int, int, error) {
	var root svgRoot
	if err := xml.NewDecoder(r).Decode(&root); err != nil {
		return false, 0, 0, err
	}

	width, widthErr := parseSVGLength(root.Width)
	height, heightErr := parseSVGLength(root.Height)
	if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
		return true, int(math.Round(width)), int(math.Round(height)), nil
	}

	viewBoxWidth, viewBoxHeight, err := parseSVGViewBox(root.ViewBox)
	if err == nil && viewBoxWidth > 0 && viewBoxHeight > 0 {
		return true, int(math.Round(viewBoxWidth)), int(math.Round(viewBoxHeight)), nil
	}

	return false, 0, 0, nil
}

func parseSVGLength(value string) (float64, error) {
	matches := svgLengthPattern.FindStringSubmatch(value)
	if len(matches) != 3 {
		return 0, fmt.Errorf("unsupported svg length: %q", value)
	}

	number, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	switch matches[2] {
	case "", "px":
		return number, nil
	case "in":
		return number * 96, nil
	case "cm":
		return number * 96 / 2.54, nil
	case "mm":
		return number * 96 / 25.4, nil
	case "pt":
		return number * 96 / 72, nil
	case "pc":
		return number * 16, nil
	case "q":
		return number * 96 / 101.6, nil
	default:
		return 0, fmt.Errorf("unsupported svg length unit: %q", matches[2])
	}
}

func parseSVGViewBox(value string) (float64, float64, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(parts) != 4 {
		return 0, 0, fmt.Errorf("unsupported viewBox: %q", value)
	}

	width, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, 0, err
	}
	height, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return 0, 0, err
	}

	return width, height, nil
}
