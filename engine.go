package reports

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// Page geometry in mm. Width and height swap between orientations: A4 landscape is 297mm wide and
// only 210mm tall. Page-break checks must use the current orientation's height; checking against
// 297mm on a landscape page draws rows past the physical bottom, where they never render, so rows
// vanish rather than mis-paginate.
const (
	pageWP   = 210.0 // portrait width
	pageWL   = 297.0 // landscape width
	pageHP   = 297.0 // portrait height
	pageHL   = 210.0 // landscape height
	margin   = 13.0
	topY     = 12.0
	bottomMg = 14.0
)

// rgb is an 8-bit colour.
type rgb struct{ r, g, b int }

// palette is every colour a report uses. The brand tones come from the tenant primary colour; the
// ink, grey and muted text tones, white and the two status tones are fixed so text stays legible
// whatever the brand colour.
type palette struct {
	sky       rgb // lightest brand tone (gradient start)
	blue      rgb // the brand colour
	navy      rgb // darkest brand tone (gradient end, headings)
	ink       rgb // body text
	grey      rgb // secondary text
	muted     rgb // tertiary text
	line      rgb // borders and hairlines
	lightBlue rgb // light brand tint (meta keys, totals)
	zebra     rgb // alternate table rows
	white     rgb
	good      rgb // favourable change; never the brand colour
	bad       rgb // negative amounts and unfavourable change
}

// defaultBrand is the indigo used when a tenant has no primary colour.
var defaultBrand = rgb{99, 102, 241}

func newPalette(primaryHex string) palette {
	base := defaultBrand
	if r, g, b, ok := hexToRGB(primaryHex); ok {
		base = rgb{r, g, b}
	}
	return palette{
		sky:       lighten(base, 0.30),
		blue:      base,
		navy:      darken(base, 0.45),
		ink:       rgb{34, 48, 63},
		grey:      rgb{82, 97, 122},
		muted:     rgb{107, 122, 144},
		line:      lighten(base, 0.80),
		lightBlue: lighten(base, 0.90),
		zebra:     lighten(base, 0.95),
		white:     rgb{255, 255, 255},
		good:      rgb{4, 120, 87},
		bad:       rgb{185, 28, 28},
	}
}

func darken(c rgb, f float64) rgb {
	return rgb{clamp(int(float64(c.r) * (1 - f))), clamp(int(float64(c.g) * (1 - f))), clamp(int(float64(c.b) * (1 - f)))}
}

func lighten(c rgb, f float64) rgb {
	return rgb{
		clamp(c.r + int(float64(255-c.r)*f)),
		clamp(c.g + int(float64(255-c.g)*f)),
		clamp(c.b + int(float64(255-c.b)*f)),
	}
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// hexToRGB parses "#RRGGBB" or "RRGGBB".
func hexToRGB(hex string) (int, int, int, bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	r, e1 := strconv.ParseInt(hex[0:2], 16, 0)
	g, e2 := strconv.ParseInt(hex[2:4], 16, 0)
	b, e3 := strconv.ParseInt(hex[4:6], 16, 0)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, 0, 0, false
	}
	return int(r), int(g), int(b), true
}

// painter wraps an fpdf document with the palette and the drawing primitives.
type painter struct {
	pdf      *fpdf.Fpdf
	tr       func(string) string
	pal      palette
	leftX    float64
	rightX   float64
	contentW float64
}

func newPainter(pdf *fpdf.Fpdf, pal palette, pageWidth float64) *painter {
	// Core fonts have 256 glyphs, so text is translated from UTF-8 to cp1252 single bytes, which
	// keeps accented Latin-1 ("Café") and dashes. Invalid UTF-8 is dropped first: a stray byte would
	// decode to U+FFFD, which the translator cannot map and MultiCell can panic on.
	base := pdf.UnicodeTranslatorFromDescriptor("")
	tr := func(s string) string { return base(strings.ToValidUTF8(s, "")) }
	return &painter{pdf: pdf, tr: tr, pal: pal, leftX: margin, rightX: pageWidth - margin, contentW: pageWidth - 2*margin}
}

func (p *painter) setFill(c rgb) { p.pdf.SetFillColor(c.r, c.g, c.b) }
func (p *painter) setText(c rgb) { p.pdf.SetTextColor(c.r, c.g, c.b) }
func (p *painter) setDraw(c rgb) { p.pdf.SetDrawColor(c.r, c.g, c.b) }

// gradient fills a rounded rect with a horizontal gradient from c1 to c2.
func (p *painter) gradient(x, y, w, h, r float64, c1, c2 rgb) {
	p.pdf.ClipRoundedRect(x, y, w, h, r, false)
	p.pdf.LinearGradient(x, y, w, h, c1.r, c1.g, c1.b, c2.r, c2.g, c2.b, 0, 0, 1, 0)
	p.pdf.ClipEnd()
}

// box draws a white rounded rect with a hairline border.
func (p *painter) box(x, y, w, h float64) {
	p.setDraw(p.pal.line)
	p.setFill(p.pal.white)
	p.pdf.SetLineWidth(0.2)
	p.pdf.RoundedRect(x, y, w, h, 1.6, "1234", "D")
}

// fillRect paints a filled rectangle without a border.
func (p *painter) fillRect(x, y, w, h float64, c rgb) {
	p.setFill(c)
	p.pdf.Rect(x, y, w, h, "F")
}

// fillRound paints a filled rounded rect without a border.
func (p *painter) fillRound(x, y, w, h, r float64, c rgb) {
	p.setFill(c)
	p.pdf.RoundedRect(x, y, w, h, r, "1234", "F")
}

// text draws a string with its top at y.
func (p *painter) text(x, y float64, s, font string, sz float64, c rgb) {
	p.pdf.SetFont("Helvetica", font, sz)
	p.setText(c)
	p.pdf.Text(x, y+sz*0.3528*0.82, p.tr(s))
}

// textR draws a string ending at rx.
func (p *painter) textR(rx, y float64, s, font string, sz float64, c rgb) {
	p.pdf.SetFont("Helvetica", font, sz)
	p.setText(c)
	t := p.tr(s)
	p.pdf.Text(rx-p.pdf.GetStringWidth(t), y+sz*0.3528*0.82, t)
}

// textC draws a string centred in [x, x+w].
func (p *painter) textC(x, w, y float64, s, font string, sz float64, c rgb) {
	p.pdf.SetFont("Helvetica", font, sz)
	p.setText(c)
	t := p.tr(s)
	p.pdf.Text(x+(w-p.pdf.GetStringWidth(t))/2, y+sz*0.3528*0.82, t)
}

// hline draws a hairline in the line colour.
func (p *painter) hline(x1, y, x2 float64) {
	p.setDraw(p.pal.line)
	p.pdf.SetLineWidth(0.15)
	p.pdf.Line(x1, y, x2, y)
}

// multiCell wraps text at x with width w and returns the y below the block.
func (p *painter) multiCell(x, y, w, lineH float64, s, font string, sz float64, c rgb) float64 {
	p.pdf.SetFont("Helvetica", font, sz)
	p.setText(c)
	p.pdf.SetXY(x, y)
	p.pdf.MultiCell(w, lineH, p.tr(s), "", "L", false)
	return p.pdf.GetY()
}

// splitText wraps s into lines of width w for measuring. fpdf's SplitText, unlike Cell and
// MultiCell, reads its input as runes, so a translated cp1252 byte above 0x7F decodes as U+FFFD
// and indexes past the 256-entry width table (a panic on "Café" or an em dash). Each translated
// byte is re-encoded as the rune of the same number, which is what the width table expects.
func (p *painter) splitText(s string, w float64) []string {
	t := []byte(p.tr(s))
	r := make([]rune, len(t))
	for i, b := range t {
		r[i] = rune(b)
	}
	return p.pdf.SplitText(string(r), w)
}

// clip shortens s to width w with an ellipsis. It measures the translated text but returns UTF-8,
// so the drawing helpers translate it exactly once; translating twice drops every accented byte
// ("Café" would print "Caf").
func (p *painter) clip(s, font string, sz, w float64) string {
	p.pdf.SetFont("Helvetica", font, sz)
	if p.pdf.GetStringWidth(p.tr(s)) <= w {
		return s
	}
	const ellipsis = "…"
	r := []rune(strings.ToValidUTF8(s, ""))
	for len(r) > 1 && p.pdf.GetStringWidth(p.tr(string(r)+ellipsis)) > w {
		r = r[:len(r)-1]
	}
	return string(r) + ellipsis
}

// textFit draws s shrinking the font from sz towards minSz until the whole string fits w, so a large
// figure in a narrow card shows in full; only at minSz is it clipped.
func (p *painter) textFit(x, y float64, s, font string, sz, minSz, w float64, c rgb) {
	t := p.tr(s)
	size := sz
	for size > minSz {
		p.pdf.SetFont("Helvetica", font, size)
		if p.pdf.GetStringWidth(t) <= w {
			break
		}
		size -= 0.25
	}
	p.pdf.SetFont("Helvetica", font, size)
	if p.pdf.GetStringWidth(t) > w {
		t = p.tr(p.clip(s, font, size, w))
	}
	p.setText(c)
	p.pdf.Text(x, y+size*0.3528*0.82, t)
}

// isNegativeAmount reports whether a formatted amount ("KES -1,200.00", "-3.5%") is below zero.
func isNegativeAmount(s string) bool {
	t := strings.TrimLeft(strings.TrimSpace(s), "ABCDEFGHIJKLMNOPQRSTUVWXYZ ")
	return len(t) > 1 && t[0] == '-' && t[1] >= '0' && t[1] <= '9'
}

// compactNumber shortens an axis or bar value: 1,250,000 is "1.25M", 48,300 is "48.3K".
func compactNumber(v float64) string {
	a, sign := v, ""
	if a < 0 {
		a, sign = -a, "-"
	}
	trim := func(f float64, suffix string) string {
		s := strconv.FormatFloat(f, 'f', 2, 64)
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		return sign + s + suffix
	}
	switch {
	case a >= 1e9:
		return trim(a/1e9, "B")
	case a >= 1e6:
		return trim(a/1e6, "M")
	case a >= 1e3:
		return trim(a/1e3, "K")
	default:
		return trim(a, "")
	}
}

// imgType sniffs the fpdf image type of raw image bytes.
func imgType(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "JPG"
	case len(b) >= 4 && b[0] == 0x47 && b[1] == 0x49 && b[2] == 0x46:
		return "GIF"
	default:
		return "PNG"
	}
}

func upper(s string) string { return strings.ToUpper(s) }

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// Amount formats a number with two decimals and thousands separators: 1234.5 is "1,234.50".
func Amount(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	dec := s[len(s)-3:]
	intPart := s[:len(s)-3]
	neg := strings.HasPrefix(intPart, "-")
	if neg {
		intPart = intPart[1:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String() + dec
	if neg {
		return "-" + out
	}
	return out
}

// Money formats "CODE 1,234.56"; an empty currency gives KES.
func Money(currency string, v float64) string {
	code := currency
	if fields := strings.Fields(currency); len(fields) > 0 {
		code = fields[0]
	}
	if code == "" {
		code = "KES"
	}
	return code + " " + Amount(v)
}

// Quantity renders a number without trailing zeros: 2, 2.5, 12.
func Quantity(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

// Date formats "02 Jan 2006"; a zero time is a dash.
func Date(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("02 Jan 2006")
}

// DateTime formats "02 Jan 2006 15:04"; a zero time is a dash.
func DateTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("02 Jan 2006 15:04")
}
