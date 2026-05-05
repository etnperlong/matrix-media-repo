package i

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"io"
	"math"

	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/thumbnailing/m"
	"github.com/t2bot/matrix-media-repo/thumbnailing/u"
	"github.com/t2bot/matrix-media-repo/util/readers"
)

type gifGenerator struct {
}

const maxAnimatedGIFFrames = 256

var errAnimatedGIFTooLarge = errors.New("animated gif exceeds max animate size")

func (d gifGenerator) supportedContentTypes() []string {
	return []string{"image/gif"}
}

func (d gifGenerator) supportsAnimation() bool {
	return true
}

func (d gifGenerator) matches(img io.Reader, contentType string) bool {
	return contentType == "image/gif"
}

func (d gifGenerator) GetOriginDimensions(b io.Reader, contentType string, ctx rcontext.RequestContext) (bool, int, int, error) {
	return pngGenerator{}.GetOriginDimensions(b, contentType, ctx)
}

func (d gifGenerator) GenerateThumbnail(b io.Reader, contentType string, width int, height int, method string, animated bool, ctx rcontext.RequestContext) (*m.Thumbnail, error) {
	if animated {
		guarded, fallbackToStatic, err := d.guardAnimatedGIFReader(b, ctx)
		if err != nil {
			return nil, err
		}
		if fallbackToStatic {
			return d.generateStaticThumbnail(guarded, width, height, method, ctx)
		}
		b = guarded
	}

	g, err := gif.DecodeAll(b)
	if err != nil {
		return nil, errors.New("gif: error decoding image: " + err.Error())
	}

	// Prepare a blank frame to use as swap space
	frameImg := image.NewRGBA(image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: g.Config.Width, Y: g.Config.Height}})

	targetStaticFrame := int(math.Floor(math.Min(1, math.Max(0, float64(ctx.Config.Thumbnails.StillFrame))) * float64(len(g.Image))))

	for i, img := range g.Image {
		var disposal byte
		// use disposal method 0 by default
		if g.Disposal == nil {
			disposal = 0
		} else {
			disposal = g.Disposal[i]
		}

		// Copy the frame to a new image and use that
		draw.Draw(frameImg, frameImg.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)

		// Do the thumbnailing on the copied frame
		frameThumb, err := u.MakeThumbnail(frameImg, method, width, height)
		if err != nil {
			return nil, errors.New("gif: error generating thumbnail frame: " + err.Error())
		}
		if frameThumb == nil {
			tmpImg := image.NewRGBA(frameImg.Bounds())
			draw.Draw(tmpImg, tmpImg.Bounds(), frameImg, image.Point{X: 0, Y: 0}, draw.Src)
			frameThumb = tmpImg
		}

		targetImg := image.NewPaletted(frameThumb.Bounds(), img.Palette)
		draw.FloydSteinberg.Draw(targetImg, frameThumb.Bounds(), frameThumb, image.Point{X: 0, Y: 0})

		if !animated && i == targetStaticFrame {
			t, err := pngGenerator{}.GenerateThumbnailOf(targetImg, width, height, method, ctx)
			if err != nil || t != nil {
				return t, err
			}

			// The thumbnailer decided that it shouldn't thumbnail, so encode it ourselves
			buf := bytes.NewBuffer(make([]byte, 0, 128*1024))
			err = u.Encode(ctx, buf, targetImg)
			if err != nil {
				return nil, fmt.Errorf("gif: error encoding static thumbnail: %w", err)
			}
			return &m.Thumbnail{
				Animated:    false,
				ContentType: "image/png",
				Reader:      io.NopCloser(buf),
			}, nil
		}

		// if disposal type is 0 or 1 (preserve previous frame) we can get artifacts from re-scaling.
		// as such, we re-render those frames to disposal type 1 (do not dispose)
		// Importantly, we do not clear the previous frame buffer canvas
		// see https://www.w3.org/Graphics/GIF/spec-gif89a.txt
		// This also applies to frame disposal type 0, https://legacy.imagemagick.org/Usage/anim_basics/#none
		if disposal == 1 || disposal == 0 {
			g.Disposal[i] = 1
		} else {
			draw.Draw(frameImg, frameImg.Bounds(), image.Transparent, image.Point{X: 0, Y: 0}, draw.Src)
		}

		g.Image[i] = targetImg
	}

	// Set the image size to the first frame's size
	g.Config.Width = g.Image[0].Bounds().Max.X
	g.Config.Height = g.Image[0].Bounds().Max.Y

	buf := bytes.NewBuffer(make([]byte, 0, 512*1024))
	err = gif.EncodeAll(buf, g)
	if err != nil {
		return nil, fmt.Errorf("gif: error encoding animated thumbnail: %w", err)
	}

	return &m.Thumbnail{
		ContentType: "image/gif",
		Animated:    true,
		Reader:      io.NopCloser(buf),
	}, nil
}

func (d gifGenerator) guardAnimatedGIFReader(b io.Reader, ctx rcontext.RequestContext) (io.Reader, bool, error) {
	buffered := readers.NewBufferReadsReader(b)
	exceeds, err := d.exceedsAnimationComplexity(buffered, ctx)
	if err != nil {
		if errors.Is(err, errAnimatedGIFTooLarge) {
			ctx.Log.Debug("Animated GIF too large for animated thumbnailing; falling back to still frame")
			return buffered.GetRewoundReader(), true, nil
		}
		ctx.Log.Debug("Animated GIF complexity pre-scan failed; continuing with normal decode: ", err)
		return buffered.GetRewoundReader(), false, nil
	}
	if exceeds {
		ctx.Log.Debug("Animated GIF too complex for animated thumbnailing; falling back to still frame")
		return buffered.GetRewoundReader(), true, nil
	}

	return buffered.GetRewoundReader(), false, nil
}

func (d gifGenerator) exceedsAnimationComplexity(r io.Reader, ctx rcontext.RequestContext) (bool, error) {
	scanner := &gifComplexityScanner{r: r, maxAnimateBytes: ctx.Config.Thumbnails.MaxAnimateSizeBytes}
	header := make([]byte, 13)
	if err := scanner.readFull(header); err != nil {
		return false, err
	}
	if string(header[:3]) != "GIF" {
		return false, errors.New("not a gif")
	}
	logicalWidth := int64(binary.LittleEndian.Uint16(header[6:8]))
	logicalHeight := int64(binary.LittleEndian.Uint16(header[8:10]))
	logicalPixels := logicalWidth * logicalHeight

	maxTotalPixels := int64(ctx.Config.Thumbnails.MaxPixels)
	if maxTotalPixels <= 0 {
		maxTotalPixels = 32000000
	}

	packed := header[10]
	if packed&0x80 != 0 {
		globalColorTableBytes := 3 * (1 << ((packed & 0x07) + 1))
		if err := scanner.skipBytes(int64(globalColorTableBytes)); err != nil {
			return false, err
		}
	}

	frames := 0
	totalPixels := int64(0)
	for {
		blockType := make([]byte, 1)
		if err := scanner.readFull(blockType); err != nil {
			return false, err
		}

		switch blockType[0] {
		case 0x2c: // image descriptor
			descriptor := make([]byte, 9)
			if err := scanner.readFull(descriptor); err != nil {
				return false, err
			}
			frames++
			totalPixels += logicalPixels
			if frames > maxAnimatedGIFFrames || totalPixels > maxTotalPixels {
				return true, nil
			}

			if descriptor[8]&0x80 != 0 {
				localColorTableBytes := 3 * (1 << ((descriptor[8] & 0x07) + 1))
				if err := scanner.skipBytes(int64(localColorTableBytes)); err != nil {
					return false, err
				}
			}
			if err := scanner.skipBytes(1); err != nil { // LZW minimum code size
				return false, err
			}
			if err := scanner.skipSubBlocks(); err != nil {
				return false, err
			}
		case 0x21: // extension
			if err := scanner.skipBytes(1); err != nil { // extension label
				return false, err
			}
			if err := scanner.skipSubBlocks(); err != nil {
				return false, err
			}
		case 0x3b: // trailer
			return false, nil
		default:
			return false, fmt.Errorf("unknown gif block type 0x%x", blockType[0])
		}
	}
}

type gifComplexityScanner struct {
	r               io.Reader
	bytesRead       int64
	maxAnimateBytes int64
}

func (s *gifComplexityScanner) readFull(buf []byte) error {
	_, err := io.ReadFull(s.r, buf)
	s.bytesRead += int64(len(buf))
	if s.maxAnimateBytes > 0 && s.bytesRead > s.maxAnimateBytes {
		return errAnimatedGIFTooLarge
	}
	return err
}

func (s *gifComplexityScanner) skipBytes(n int64) error {
	read, err := io.CopyN(io.Discard, s.r, n)
	s.bytesRead += read
	if s.maxAnimateBytes > 0 && s.bytesRead > s.maxAnimateBytes {
		return errAnimatedGIFTooLarge
	}
	return err
}

func (s *gifComplexityScanner) skipSubBlocks() error {
	for {
		size := make([]byte, 1)
		if err := s.readFull(size); err != nil {
			return err
		}
		if size[0] == 0 {
			return nil
		}
		if err := s.skipBytes(int64(size[0])); err != nil {
			return err
		}
	}
}

func (d gifGenerator) generateStaticThumbnail(b io.Reader, width int, height int, method string, ctx rcontext.RequestContext) (*m.Thumbnail, error) {
	img, err := gif.Decode(b)
	if err != nil {
		return nil, errors.New("gif: error decoding static fallback: " + err.Error())
	}

	t, err := pngGenerator{}.GenerateThumbnailOf(img, width, height, method, ctx)
	if err != nil || t != nil {
		return t, err
	}

	buf := bytes.NewBuffer(make([]byte, 0, 128*1024))
	if err = u.Encode(ctx, buf, img); err != nil {
		return nil, fmt.Errorf("gif: error encoding static fallback: %w", err)
	}
	return &m.Thumbnail{
		Animated:    false,
		ContentType: "image/png",
		Reader:      io.NopCloser(buf),
	}, nil
}

func init() {
	generators = append(generators, gifGenerator{})
}
