package reports

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// renderPDF builds the branded A4 report PDF.
func renderPDF(r *Report) ([]byte, error) {
	pdf := layout(r)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render report pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// layout draws the report onto a new document; split from renderPDF so tests can inspect pages.
func layout(r *Report) *fpdf.Fpdf {
	orient := "P"
	pw, ph := pageWP, pageHP
	if r.Landscape {
		orient = "L"
		pw, ph = pageWL, pageHL
	}
	pdf := fpdf.New(orient, "mm", "A4", "")
	pdf.SetCompression(true)
	pdf.SetMargins(margin, topY, margin)
	pdf.SetAutoPageBreak(false, bottomMg) // pages break by hand: drawing uses absolute coordinates
	pdf.AliasNbPages("")                  // "{nb}" becomes the page count

	p := newPainter(pdf, newPalette(r.PrimaryColor), pw)
	rc := &reportCtx{p: p, r: r, pageH: ph}
	// Every page has the footer; pages after the first a slim running header, so a loose page
	// still says whose report it is.
	pdf.SetFooterFunc(rc.drawFooter)
	pdf.SetHeaderFunc(func() {
		if pdf.PageNo() > 1 {
			rc.drawRunningHeader()
		}
	})
	pdf.AddPage()

	rc.drawHeader()
	rc.y = rc.drawMeta(rc.y + 5.0)
	if len(r.Cards) > 0 {
		rc.y = rc.drawCards(rc.y + 5.0)
	}
	for _, s := range r.Sections {
		rc.drawSection(s)
	}
	return pdf
}

// reportCtx carries the painter, the report and the vertical cursor. pageH is this report's page
// height (297mm portrait, 210mm landscape).
type reportCtx struct {
	p     *painter
	r     *Report
	y     float64
	pageH float64
}

// runningHeaderH is the height of the continuation-page header.
const runningHeaderH = 9.0

// bottomLimit is the lowest y content may reach on the current page.
func (rc *reportCtx) bottomLimit() float64 { return rc.pageH - bottomMg - 6.0 }

// newPage starts a page and puts the cursor below the running header.
func (rc *reportCtx) newPage() {
	rc.p.pdf.AddPage()
	rc.y = topY + runningHeaderH
}

// ensure starts a new page unless h mm fit below the cursor.
func (rc *reportCtx) ensure(h float64) {
	if rc.y+h > rc.bottomLimit() {
		rc.newPage()
	}
}

// drawRunningHeader: tenant name left, report title right, brand hairline beneath.
func (rc *reportCtx) drawRunningHeader() {
	p, r := rc.p, rc.r
	y := topY - 2.0
	p.text(p.leftX, y, p.clip(r.TenantName, "B", 8.5, p.contentW*0.5), "B", 8.5, p.pal.navy)
	p.textR(p.rightX, y, p.clip(r.Title, "", 8, p.contentW*0.45), "", 8, p.pal.grey)
	p.gradient(p.leftX, y+5.2, p.contentW, 0.6, 0.3, p.pal.sky, p.pal.navy)
}

// drawHeader: logo left, title and subtitle right, gradient rule beneath.
func (rc *reportCtx) drawHeader() {
	p, r := rc.p, rc.r
	hy := topY - 1.0
	logoH := 20.0
	if len(r.LogoPNG) > 0 {
		lt := r.LogoType
		if lt == "" {
			lt = imgType(r.LogoPNG)
		}
		if info := p.pdf.RegisterImageOptionsReader("rptlogo", fpdf.ImageOptions{ImageType: lt}, bytes.NewReader(r.LogoPNG)); info != nil && info.Height() > 0 {
			logoW := logoH * info.Width() / info.Height()
			p.pdf.ImageOptions("rptlogo", p.leftX, hy, logoW, logoH, false, fpdf.ImageOptions{ImageType: lt}, 0, "")
		} else {
			p.pdf.ClearError() // an unreadable logo never fails the report
		}
	}
	p.textR(p.rightX, hy+0.5, upper(r.Title), "B", 20, p.pal.navy)
	if r.Subtitle != "" {
		p.textR(p.rightX, hy+9.0, upper(r.Subtitle), "", 8, p.pal.blue)
	}
	ruleY := topY + logoH + 1.0
	p.gradient(p.leftX, ruleY, p.contentW, 1.4, 0.7, p.pal.sky, p.pal.navy)
	rc.y = ruleY + 2.0
}

// drawMeta: tenant identity left, meta box right. Returns the y below the taller of the two.
func (rc *reportCtx) drawMeta(y float64) float64 {
	p, r := rc.p, rc.r

	companyW := p.contentW*0.5 - 6.0
	p.text(p.leftX, y, ifEmpty(r.TenantName, "-"), "B", 14, p.pal.navy)
	ly := y + 6.2
	if r.OutletName != "" && r.OutletName != r.TenantName {
		p.text(p.leftX, ly, r.OutletName, "B", 9.5, p.pal.blue)
		ly += 5.0
	}
	if r.Address != "" {
		ly = p.multiCell(p.leftX, ly, companyW, 4.2, r.Address, "", 9, p.pal.grey) + 0.6
	}

	rows := rc.metaRows()
	mbW := p.contentW * 0.44
	mbX := p.rightX - mbW
	keyW := mbW * 0.44
	const rowH = 6.4
	ry := y
	for i, kv := range rows {
		p.fillRect(mbX, ry, keyW, rowH, p.pal.lightBlue)
		if i > 0 {
			p.hline(mbX, ry, mbX+mbW)
		}
		p.text(mbX+2.5, ry+2.1, upper(kv[0]), "B", 7.2, p.pal.blue)
		p.text(mbX+keyW+2.5, ry+2.1, p.clip(kv[1], "B", 8, mbW-keyW-4.5), "B", 8, p.pal.navy)
		ry += rowH
	}
	p.box(mbX, y, mbW, ry-y)

	if ry > ly {
		return ry
	}
	return ly
}

// metaRows: period, generated time, currency, then the caller's rows.
func (rc *reportCtx) metaRows() [][2]string {
	r := rc.r
	rows := [][2]string{}
	if !r.PeriodFrom.IsZero() || !r.PeriodTo.IsZero() {
		rows = append(rows, [2]string{"Period", Date(r.PeriodFrom) + " - " + Date(r.PeriodTo)})
	}
	rows = append(rows, [2]string{"Generated", DateTime(r.GeneratedAt)})
	if r.Currency != "" {
		rows = append(rows, [2]string{"Currency", r.Currency})
	}
	for _, kv := range r.Meta {
		// The outlet already shows under the tenant name; an Outlet row is for CSV and Excel.
		if kv[0] == "Outlet" && kv[1] == r.OutletName {
			continue
		}
		rows = append(rows, kv)
	}
	return rows
}

// drawCards renders the summary tiles in one row.
func (rc *reportCtx) drawCards(y float64) float64 {
	p := rc.p
	n := len(rc.r.Cards)
	if n == 0 {
		return y
	}
	gap := 4.0
	cw := (p.contentW - gap*float64(n-1)) / float64(n)
	ch := 19.0
	rc.y = y
	rc.ensure(ch + 4)
	y = rc.y
	x := p.leftX
	for _, c := range rc.r.Cards {
		p.fillRound(x, y, cw, ch, 1.8, p.pal.zebra)
		p.gradient(x, y, cw, 1.1, 0.5, p.pal.sky, p.pal.blue)
		p.text(x+3.5, y+3.6, upper(c.Label), "B", 6.8, p.pal.muted)
		valCol := p.pal.navy
		if isNegativeAmount(c.Value) {
			valCol = p.pal.bad
		}
		p.textFit(x+3.5, y+8.2, c.Value, "B", 13, 8.0, cw-7, valCol)
		if c.Sub != "" {
			subCol := p.pal.grey
			switch {
			case strings.HasPrefix(c.Sub, "+"):
				subCol = p.pal.good
			case isNegativeAmount(c.Sub):
				subCol = p.pal.bad
			}
			p.text(x+3.5, y+14.4, p.clip(c.Sub, "B", 7.2, cw-7), "B", 7.2, subCol)
		}
		x += cw + gap
	}
	return y + ch
}

// minOrphanRows body rows stay with a section heading, so a page never ends on a bare heading.
const minOrphanRows = 3

// drawSection reserves the heading plus a minimum slice of the body as one check before drawing
// anything. Checking the heading and the body separately let a heading and table header land at a
// page bottom with every row on the next page.
func (rc *reportCtx) drawSection(s Section) {
	rc.y += 6.0
	headingH := 0.0
	if s.Title != "" {
		headingH = 8.0
		if s.Note != "" {
			headingH += 5.0
		}
	}
	rc.ensure(headingH + rc.sectionMinBodyH(s))
	if s.Title != "" {
		rc.p.fillRound(rc.p.leftX, rc.y-0.2, 1.3, 4.6, 0.6, rc.p.pal.blue)
		rc.p.text(rc.p.leftX+3.4, rc.y, upper(s.Title), "B", 10, rc.p.pal.navy)
		rc.p.hline(rc.p.leftX, rc.y+6.0, rc.p.rightX)
		rc.y += 8.0
		if s.Note != "" {
			rc.p.text(rc.p.leftX, rc.y, s.Note, "I", 8.5, rc.p.pal.grey)
			rc.y += 5.0
		}
	}
	switch s.Kind {
	case SectionTable:
		rc.drawTable(s)
	case SectionKeyValue:
		rc.drawKeyValue(s)
	case SectionChart:
		rc.drawChart(s)
	case SectionProse:
		rc.drawProse(s)
	case SectionSignature:
		rc.drawSignatures(s)
	}
}

// sectionMinBodyH is the body height that must stay with the heading: a table header and a few
// rows, the first few pairs, a whole column chart (never split), two lines of prose, or one
// signature row.
func (rc *reportCtx) sectionMinBodyH(s Section) float64 {
	switch s.Kind {
	case SectionTable:
		n := min(minOrphanRows, len(s.Rows))
		return tableHeadH + tableRowH*float64(n)
	case SectionKeyValue:
		n := min(minOrphanRows, len(s.Pairs))
		return kvRowH*float64(n) + 4
	case SectionChart:
		if s.Horizontal {
			return hRowH * float64(min(minOrphanRows, len(s.Bars)))
		}
		return chartH + chartLabelH + 8.0
	case SectionProse:
		return 2 * proseLineH
	case SectionSignature:
		return sigBlockH
	default:
		return 0
	}
}

// Row geometry in mm.
const (
	tableHeadH = 7.0
	tableRowH  = 6.2
	kvRowH     = 6.0
	proseLineH = 4.6  // wrapped body line height
	proseGap   = 2.6  // gap between paragraphs
	sigBlockH  = 20.0 // one signature row
)

// drawProse measures each paragraph first, so one that would straddle the page bottom starts on a
// fresh page; with automatic breaks off, an unmeasured MultiCell would run off the page.
func (rc *reportCtx) drawProse(s Section) {
	p := rc.p
	for _, para := range s.Paragraphs {
		if para == "" {
			rc.y += proseGap
			continue
		}
		font, body := "", para
		if strings.HasPrefix(para, "**") {
			font, body = "B", strings.TrimPrefix(para, "**")
		}
		p.pdf.SetFont("Helvetica", font, 9.2)
		lines := p.splitText(body, p.contentW)
		if len(lines) == 0 {
			lines = []string{""}
		}
		h := float64(len(lines)) * proseLineH
		// Break first when the paragraph does not fit but would fit on an empty page; a longer one
		// flows, or it would loop.
		if rc.y+h > rc.bottomLimit() && h < rc.pageH-topY-bottomMg-6.0 {
			rc.newPage()
		}
		col := p.pal.ink
		if font == "B" {
			col = p.pal.navy
		}
		rc.y = p.multiCell(p.leftX, rc.y, p.contentW, proseLineH, body, font, 9.2, col) + proseGap
	}
}

// drawSignatures renders ruled signature blocks two per row.
func (rc *reportCtx) drawSignatures(s Section) {
	p := rc.p
	if len(s.Signatures) == 0 {
		return
	}
	sigW := (p.contentW - 12.0) / 2
	for i := 0; i < len(s.Signatures); i += 2 {
		rc.ensure(sigBlockH)
		y := rc.y + 8.0 // room above the rule for a wet-ink signature
		for j := 0; j < 2 && i+j < len(s.Signatures); j++ {
			sig := s.Signatures[i+j]
			x := p.leftX + float64(j)*(sigW+12.0)
			p.setDraw(p.pal.navy)
			p.pdf.SetLineWidth(0.4)
			p.pdf.Line(x, y, x+sigW, y)
			p.text(x, y+1.4, sig.Role, "", 8.2, p.pal.grey)
			if sig.Name != "" {
				p.text(x, y+5.6, sig.Name, "B", 9.2, p.pal.navy)
			}
			if sig.DateLabel != "" {
				dy := y + 5.6
				if sig.Name != "" {
					dy += 4.6
				}
				p.setDraw(p.pal.line)
				p.pdf.SetLineWidth(0.25)
				p.pdf.Line(x, dy+3.4, x+sigW*0.7, dy+3.4)
				p.text(x, dy, sig.DateLabel+":", "", 8.2, p.pal.muted)
			}
		}
		rc.y = y + sigBlockH - 8.0 + 6.0
	}
}

// drawTable renders a zebra-striped table with an optional bold total row, repeating the header
// row on every page it spans.
func (rc *reportCtx) drawTable(s Section) {
	p := rc.p
	if len(s.Columns) == 0 {
		return
	}
	var total float64
	for _, c := range s.Columns {
		total += weight(c)
	}
	xs := make([]float64, len(s.Columns)+1)
	xs[0] = p.leftX
	for i, c := range s.Columns {
		xs[i+1] = xs[i] + p.contentW*weight(c)/total
	}

	// No ensure() in the header: the first one has room from drawSection's check, and every later
	// one follows a new page. Checking here would re-orphan the heading.
	drawHeaderRow := func() {
		p.fillRound(p.leftX, rc.y, p.contentW, tableHeadH, 1.2, p.pal.navy)
		for i, c := range s.Columns {
			rc.drawCellText(upper(c.Header), xs[i], xs[i+1], rc.y+2.0, "B", 7, p.pal.white, colAlign(c))
		}
		rc.y += tableHeadH
	}
	drawHeaderRow()

	for idx, row := range s.Rows {
		if rc.y+tableRowH > rc.bottomLimit() {
			rc.newPage()
			drawHeaderRow()
		}
		if idx%2 == 1 {
			p.fillRect(p.leftX, rc.y, p.contentW, tableRowH, p.pal.zebra)
		}
		for i, c := range s.Columns {
			if i >= len(row) {
				break
			}
			font := ""
			if row[i].Bold {
				font = "B"
			}
			rc.drawCellText(row[i].Text, xs[i], xs[i+1], rc.y+1.6, font, 8, amountColor(p, c, row[i].Text), colAlign(c))
		}
		rc.y += tableRowH
	}
	p.hline(p.leftX, rc.y, p.rightX)

	if len(s.Total) > 0 {
		if rc.y+tableRowH+1 > rc.bottomLimit() {
			rc.newPage()
			drawHeaderRow()
		}
		rc.y += 0.8
		p.fillRound(p.leftX, rc.y, p.contentW, tableRowH+0.6, 1.0, p.pal.lightBlue)
		for i, c := range s.Columns {
			if i >= len(s.Total) {
				break
			}
			col := p.pal.navy
			if colAlign(c) == "R" && isNegativeAmount(s.Total[i].Text) {
				col = p.pal.bad
			}
			rc.drawCellText(s.Total[i].Text, xs[i], xs[i+1], rc.y+1.9, "B", 8.4, col, colAlign(c))
		}
		rc.y += tableRowH + 0.6
	}
}

func weight(c Column) float64 {
	if c.Weight <= 0 {
		return 1
	}
	return c.Weight
}

// drawCellText draws a cell's text inside [x0, x1] with 2mm padding.
func (rc *reportCtx) drawCellText(s string, x0, x1, y float64, font string, sz float64, c rgb, align string) {
	p := rc.p
	pad := 2.0
	w := max(x1-x0-2*pad, 2)
	t := p.clip(s, font, sz, w)
	switch align {
	case "R":
		p.textR(x1-pad, y, t, font, sz, c)
	case "C":
		p.textC(x0+pad, w, y, t, font, sz, c)
	default:
		p.text(x0+pad, y, t, font, sz, c)
	}
}

// amountColor: the negative tone for a below-zero figure in a right-aligned column, else ink.
func amountColor(p *painter, c Column, text string) rgb {
	if colAlign(c) == "R" && isNegativeAmount(text) {
		return p.pal.bad
	}
	return p.pal.ink
}

func colAlign(c Column) string {
	if c.Align != "" {
		return c.Align
	}
	if c.Money {
		return "R"
	}
	return "L"
}

// drawKeyValue renders label and value rows; a Rule row sits on a tinted band. No whole-block
// ensure(): the heading already has room for the first rows, and paging the block would strand it.
func (rc *reportCtx) drawKeyValue(s Section) {
	p := rc.p
	boxW := min(p.contentW*0.62, 120)
	y := rc.y
	for i, kv := range s.Pairs {
		if y+kvRowH > rc.bottomLimit() {
			rc.newPage()
			y = rc.y
		}
		switch {
		case kv.Rule:
			p.fillRound(p.leftX, y, boxW, kvRowH, 1.0, p.pal.lightBlue)
		case i%2 == 1:
			p.fillRect(p.leftX, y, boxW, kvRowH, p.pal.zebra)
		}
		font := ""
		col := p.pal.ink
		if kv.Bold {
			font, col = "B", p.pal.navy
		}
		valCol := col
		if isNegativeAmount(kv.Value) {
			valCol = p.pal.bad
		}
		rc.drawCellText(kv.Label, p.leftX+1, p.leftX+boxW*0.58, y+1.6, font, 9, col, "L")
		rc.drawCellText(kv.Value, p.leftX+boxW*0.55, p.leftX+boxW-1, y+1.6, font, 9, valCol, "R")
		y += kvRowH
	}
	p.hline(p.leftX, y, p.leftX+boxW)
	rc.y = y
}

// drawFooter: the note and page count, then the platform owner's line when enabled.
func (rc *reportCtx) drawFooter() {
	p := rc.p
	note := rc.r.Footer
	if note == "" {
		note = "Generated on Codevertex · " + DateTime(rc.r.GeneratedAt)
	}
	fy := rc.pageH - bottomMg + 1.0
	p.hline(p.leftX, fy, p.rightX)
	p.text(p.leftX, fy+1.5, note, "", 7, p.pal.muted)
	right := fmt.Sprintf("Page %d of {nb}", p.pdf.PageNo())
	if rc.r.TenantName != "" {
		right = rc.r.TenantName + "  |  " + right
	}
	p.textR(p.rightX, fy+1.5, right, "", 7, p.pal.muted)
	if rc.r.ProviderFooterEnabled {
		w := p.rightX - p.leftX
		p.textC(p.leftX, w, fy+5.0, providerFooterLead, "B", 6.6, p.pal.muted)
		p.textC(p.leftX, w, fy+8.2, providerFooterContact, "", 6.2, p.pal.muted)
	}
}
