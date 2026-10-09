package reports

import "math"

// Chart geometry in mm. fpdf has no chart helper, so gridlines, bars and labels are laid out by
// hand in the brand palette.
const (
	chartH      = 62.0 // column chart plot height
	chartLabelH = 14.0 // room under the axis for category labels
	hRowH       = 7.4  // one row of a horizontal chart
	hLabelW     = 46.0 // label gutter of a horizontal chart
	hValueW     = 24.0 // room past the bar end for its value
)

// drawChart draws columns, or one row per bar when the section asks for Horizontal.
func (rc *reportCtx) drawChart(s Section) {
	if len(s.Bars) == 0 {
		return
	}
	if s.Horizontal {
		rc.drawHorizontalChart(s)
		return
	}
	rc.drawColumnChart(s)
}

// drawColumnChart draws a column per bar on an axis from zero, extended below zero when a bar is
// negative (a loss month draws downward in the negative tone instead of vanishing). Gridlines sit
// on round steps; values show only when the slot is wide enough, and labels thin out to every k-th
// bar on long series so they never collide. drawSection has already reserved the whole chart with
// its heading, so there is no page check here.
func (rc *reportCtx) drawColumnChart(s Section) {
	p := rc.p

	var maxV, minV float64
	for _, b := range s.Bars {
		maxV = math.Max(maxV, b.Value)
		minV = math.Min(minV, b.Value)
	}
	step := niceCeil((maxV - minV) / 4)
	niceMax := math.Ceil(maxV/step) * step
	niceMin := math.Floor(minV/step) * step
	if niceMax == niceMin {
		niceMax = niceMin + step
	}
	span := niceMax - niceMin
	ticks := int(math.Round(span / step))

	plotX := p.leftX + 16.0 // gutter for axis labels
	plotW := p.contentW - 18.0
	bottom := rc.y + chartH
	yOf := func(v float64) float64 { return bottom - chartH*(v-niceMin)/span }
	zero := yOf(0)

	for i := 0; i <= ticks; i++ {
		val := niceMin + step*float64(i)
		gy := yOf(val)
		p.setDraw(p.pal.line)
		p.pdf.SetLineWidth(0.12)
		p.pdf.Line(plotX, gy, plotX+plotW, gy)
		p.textR(plotX-2.0, gy-1.3, compactNumber(val), "", 6.3, p.pal.muted)
	}

	n := len(s.Bars)
	slot := plotW / float64(n)
	barW := min(slot*0.6, 22)
	showValues := slot >= 11
	every := 1
	for slot*float64(every) < 12 {
		every++
	}
	for i, b := range s.Bars {
		cx := plotX + slot*float64(i) + slot/2
		bx := cx - barW/2
		vy := yOf(b.Value)
		if b.Value >= 0 {
			if h := zero - vy; h > 0.3 {
				p.gradient(bx, vy, barW, h, 0.8, p.pal.sky, p.pal.blue)
			}
			if showValues && b.Value != 0 {
				p.textC(cx-slot/2, slot, vy-3.6, compactNumber(b.Value), "B", 6.4, p.pal.navy)
			}
		} else {
			if h := vy - zero; h > 0.3 {
				p.fillRound(bx, zero, barW, h, 0.8, p.pal.bad)
			}
			if showValues {
				p.textC(cx-slot/2, slot, vy+1.0, compactNumber(b.Value), "B", 6.4, p.pal.bad)
			}
		}
		if i%every == 0 {
			p.textC(cx-slot*float64(every)/2, slot*float64(every), bottom+2.0,
				p.clip(b.Label, "", 6.6, slot*float64(every)-1), "", 6.6, p.pal.grey)
		}
	}

	p.setDraw(p.pal.navy)
	p.pdf.SetLineWidth(0.3)
	p.pdf.Line(plotX, zero, plotX+plotW, zero)

	rc.y = bottom + chartLabelH
}

// drawHorizontalChart draws one row per bar: label left, bar to the right, value at its end. Many
// categories just make it taller, paginating like a table. Negative values draw as an empty bar
// with the value in the negative tone.
func (rc *reportCtx) drawHorizontalChart(s Section) {
	p := rc.p

	var maxV float64
	for _, b := range s.Bars {
		maxV = math.Max(maxV, b.Value)
	}
	niceMax := niceCeil(maxV)
	plotX := p.leftX + hLabelW
	plotW := max(p.contentW-hLabelW-hValueW, 20)

	for _, b := range s.Bars {
		rc.ensure(hRowH)
		y := rc.y
		rc.drawCellText(b.Label, p.leftX, plotX, y+2.2, "B", 7.2, p.pal.grey, "R")
		bw := max(plotW*(b.Value/niceMax), 0)
		barH := hRowH * 0.55
		if bw > 0.3 {
			p.gradient(plotX, y+(hRowH-barH)/2, bw, barH, 0.6, p.pal.sky, p.pal.blue)
		}
		valCol := p.pal.navy
		if b.Value < 0 {
			valCol = p.pal.bad
		}
		rc.drawCellText(compactNumber(b.Value), plotX+bw, p.rightX, y+2.2, "B", 7.2, valCol, "L")
		rc.y += hRowH
	}
	rc.y += 4.0
}

// niceCeil rounds v up to a tidy axis value: 1, 2, 2.5 or 5 times a power of ten.
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := 1.0
	for v/mag >= 10 {
		mag *= 10
	}
	for v/mag < 1 {
		mag /= 10
	}
	switch n := v / mag; {
	case n <= 1:
		return mag
	case n <= 2:
		return 2 * mag
	case n <= 2.5:
		return 2.5 * mag
	case n <= 5:
		return 5 * mag
	default:
		return 10 * mag
	}
}
