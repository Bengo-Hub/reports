package reports

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

// tinyWebP is a 1x1 lossless WebP (the well-known browser feature-detection sample).
const tinyWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

// signaturePNG draws a small ink stroke on a transparent canvas, the shape a signature pad saves.
func signaturePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, h/2+(x%7)-3, color.NRGBA{R: 20, G: 30, B: 90, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPrepareSignatureImage(t *testing.T) {
	out, err := PrepareSignatureImage(signaturePNG(t, 300, 100))
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 300 || cfg.Height != 100 {
		t.Fatalf("prepared png %dx%d, %v", cfg.Width, cfg.Height, err)
	}
	if cfg.ColorModel != color.NRGBAModel {
		t.Fatal("prepared png is not 8-bit NRGBA, so its transparency may be lost")
	}

	webp, _ := base64.StdEncoding.DecodeString(tinyWebP)
	out, err = PrepareSignatureImage(webp)
	if err != nil {
		t.Fatalf("webp: %v", err)
	}
	if !bytes.HasPrefix(out, pngMagic) {
		t.Fatal("webp was not converted to png")
	}

	for name, b := range map[string][]byte{
		"empty":     nil,
		"jpeg":      {0xFF, 0xD8, 0xFF, 0xE0, 0, 0},
		"truncated": signaturePNG(t, 50, 20)[:40],
		"oversized": append(append([]byte{}, pngMagic...), make([]byte, MaxSignatureImageBytes)...),
		"too wide":  signaturePNG(t, maxSignatureImageSide+1, 1),
	} {
		if _, err := PrepareSignatureImage(b); !errors.Is(err, ErrSignatureImage) {
			t.Errorf("%s: want ErrSignatureImage, got %v", name, err)
		}
	}
}

// TestSignatureImageEmbedded checks a stored signature lands in the document, and that a bad image
// leaves the wet-ink line instead of failing the report.
func TestSignatureImageEmbedded(t *testing.T) {
	r := &Report{Title: "Acceptance", GeneratedAt: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC), Sections: []Section{{
		Kind: SectionSignature, Title: "Sign-off",
		Signatures: []Signature{
			{Role: "Product owner", Name: "A. Tester", DateLabel: "Date", Image: signaturePNG(t, 600, 200), Note: "Signed with a stored signature"},
			{Role: "Witness", Image: []byte("not an image")},
			{Role: "Client", DateLabel: "Date"},
		},
	}}}
	pdf := layout(r)
	if err := pdf.Error(); err != nil {
		t.Fatalf("layout: %v", err)
	}
	if info := pdf.GetImageInfo("rptsig1"); info == nil {
		t.Fatal("the stored signature was not embedded")
	}
	if info := pdf.GetImageInfo("rptsig2"); info != nil {
		t.Fatal("an unreadable image was embedded")
	}
	b, _, err := Generate(r, FormatPDF)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("generate: %v", err)
	}
	csv, _, err := Generate(r, FormatCSV)
	if err != nil || !bytes.Contains(csv, []byte("Signed with a stored signature")) {
		t.Fatalf("csv lost the signature note: %v\n%s", err, csv)
	}
}

// TestPDFDeterministic guards document hashing: the same Report must render to the same bytes.
func TestPDFDeterministic(t *testing.T) {
	r := sampleReport()
	r.Sections = append(r.Sections, Section{Kind: SectionSignature, Signatures: []Signature{{Role: "Signer", Image: signaturePNG(t, 200, 80)}}})
	a, _, err := Generate(r, FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // a clock-stamped date would differ now
	b, _, err := Generate(r, FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("two renders of one report differ; a stored document hash could never be checked")
	}
}
