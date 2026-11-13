package reports

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"golang.org/x/image/webp"
)

// MaxSignatureImageBytes caps a signature image handed to the renderer. It matches the private
// signature upload limit in httpware/objectstore (1 MB), so a file that was accepted at upload
// is never refused here.
const MaxSignatureImageBytes = 1 << 20

// maxSignatureImageSide bounds the decoded width and height, so a small file that declares a huge
// canvas cannot make the renderer allocate gigabytes.
const maxSignatureImageSide = 4000

// ErrSignatureImage is returned by PrepareSignatureImage for an image the renderer will not embed.
var ErrSignatureImage = errors.New("reports: unusable signature image")

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	riffMagic = []byte("RIFF")
	webpMagic = []byte("WEBP")
)

// PrepareSignatureImage checks a stored signature (PNG or WebP, at most MaxSignatureImageBytes and
// maxSignatureImageSide pixels a side) and returns it as an 8-bit PNG with its transparency kept,
// the one form the PDF writer embeds reliably (it refuses 16-bit and interlaced PNG and has no
// WebP support). Services can call it when a signature is chosen, to refuse a bad file early; the
// renderer calls it again and leaves the line blank when it fails.
func PrepareSignatureImage(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: empty", ErrSignatureImage)
	}
	if len(b) > MaxSignatureImageBytes {
		return nil, fmt.Errorf("%w: %d bytes is over the %d byte limit", ErrSignatureImage, len(b), MaxSignatureImageBytes)
	}
	var (
		cfg    image.Config
		decode func([]byte) (image.Image, error)
		err    error
	)
	switch {
	case bytes.HasPrefix(b, pngMagic):
		cfg, err = png.DecodeConfig(bytes.NewReader(b))
		decode = func(b []byte) (image.Image, error) { return png.Decode(bytes.NewReader(b)) }
	case len(b) >= 12 && bytes.Equal(b[:4], riffMagic) && bytes.Equal(b[8:12], webpMagic):
		cfg, err = webp.DecodeConfig(bytes.NewReader(b))
		decode = func(b []byte) (image.Image, error) { return webp.Decode(bytes.NewReader(b)) }
	default:
		return nil, fmt.Errorf("%w: only PNG and WebP are accepted", ErrSignatureImage)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSignatureImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSignatureImageSide || cfg.Height > maxSignatureImageSide {
		return nil, fmt.Errorf("%w: %dx%d pixels is outside 1 to %d a side", ErrSignatureImage, cfg.Width, cfg.Height, maxSignatureImageSide)
	}
	img, err := decode(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSignatureImage, err)
	}
	// Redraw onto 8-bit NRGBA: drops 16-bit depth, palettes and interlacing, keeps alpha.
	bounds := img.Bounds()
	flat := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(flat, flat.Bounds(), img, bounds.Min, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, flat); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSignatureImage, err)
	}
	return out.Bytes(), nil
}
