package i

import (
	"errors"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type externalTool struct {
	name        string
	installHint string
}

var (
	commandLookPath       = exec.LookPath
	commandCombinedOutput = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}

	rsvgConvertTool = externalTool{
		name:        "rsvg-convert",
		installHint: "use the slim/full container image or install rsvg-convert",
	}
	djxlTool = externalTool{
		name:        "djxl",
		installHint: "use the slim/full container image or install libjxl-tools",
	}
	jxlinfoTool = externalTool{
		name:        "jxlinfo",
		installHint: "use the slim/full container image or install libjxl-tools",
	}
	ffmpegTool = externalTool{
		name:        "ffmpeg",
		installHint: "use the full container image or install ffmpeg",
	}

	jxlInfoDimensionsPattern = regexp.MustCompile(`(?m)^JPEG XL image,\s*(\d+)x(\d+)\b`)
)

func buildRsvgConvertArgs(inputFile string, outputFile string, sourceWidth int, sourceHeight int) []string {
	args := []string{"-f", "png"}
	width, height := fitWithinBox(sourceWidth, sourceHeight, 4096, 4096)
	if width > 0 && height > 0 && (width != sourceWidth || height != sourceHeight) {
		args = append(args, "-w", strconv.Itoa(width), "-h", strconv.Itoa(height))
	}
	args = append(args, "-o", outputFile, inputFile)
	return args
}

func fitWithinBox(width int, height int, maxWidth int, maxHeight int) (int, int) {
	if width <= 0 || height <= 0 {
		return width, height
	}
	if width <= maxWidth && height <= maxHeight {
		return width, height
	}

	scale := math.Min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	scaledWidth := int(math.Floor(float64(width) * scale))
	scaledHeight := int(math.Floor(float64(height) * scale))
	if scaledWidth < 1 {
		scaledWidth = 1
	}
	if scaledHeight < 1 {
		scaledHeight = 1
	}
	return scaledWidth, scaledHeight
}

func parseJXLInfoDimensions(output []byte) (int, int, error) {
	matches := jxlInfoDimensionsPattern.FindSubmatch(output)
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("unexpected jxlinfo output: %s", strings.TrimSpace(string(output)))
	}

	width, err := strconv.Atoi(string(matches[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid jxlinfo width: %w", err)
	}
	height, err := strconv.Atoi(string(matches[2]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid jxlinfo height: %w", err)
	}

	return width, height, nil
}

func runExternalTool(tool externalTool, args ...string) error {
	_, err := runExternalToolOutput(tool, args...)
	return err
}

func runExternalToolOutput(tool externalTool, args ...string) ([]byte, error) {
	if _, err := commandLookPath(tool.name); err != nil {
		return nil, wrapExternalToolLookupError(tool, err)
	}

	output, err := commandCombinedOutput(tool.name, args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return output, fmt.Errorf("%s failed: %w: %s", tool.name, err, detail)
		}
		return output, fmt.Errorf("%s failed: %w", tool.name, err)
	}

	return output, nil
}

func wrapExternalToolLookupError(tool externalTool, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s not found; %s", tool.name, tool.installHint)
	}
	return fmt.Errorf("error locating %s: %w", tool.name, err)
}
