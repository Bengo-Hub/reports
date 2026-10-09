package reports

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// renderXLSX writes the report as a workbook: a Summary sheet (title, period, meta, cards,
// key/value blocks, chart data) and one sheet per table so each can be filtered and summed. Money
// columns are real numbers with a two-decimal format, so totals and pivots work.
func renderXLSX(r *Report) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	headerFill := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(r.PrimaryColor)), "#")
	if _, _, _, ok := hexToRGB(headerFill); !ok {
		headerFill = "4F46E5"
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	title, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	header, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{headerFill}},
	})
	moneyFmt := "#,##0.00"
	money, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt})
	moneyBold, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt, Font: &excelize.Font{Bold: true}})

	summary := "Summary"
	if err := f.SetSheetName("Sheet1", summary); err != nil {
		return nil, fmt.Errorf("xlsx summary sheet: %w", err)
	}
	row := 1
	put := func(sheet string, col, row int, v any, style int) {
		ref, _ := excelize.CoordinatesToCellName(col, row)
		_ = f.SetCellValue(sheet, ref, v)
		if style != 0 {
			_ = f.SetCellStyle(sheet, ref, ref, style)
		}
	}

	put(summary, 1, row, r.Title, title)
	row++
	if r.Subtitle != "" {
		put(summary, 1, row, r.Subtitle, 0)
		row++
	}
	if r.TenantName != "" {
		put(summary, 1, row, r.TenantName, bold)
		row++
	}
	if !r.PeriodFrom.IsZero() || !r.PeriodTo.IsZero() {
		put(summary, 1, row, "Period", bold)
		put(summary, 2, row, Date(r.PeriodFrom)+" to "+Date(r.PeriodTo), 0)
		row++
	}
	for _, m := range r.Meta {
		put(summary, 1, row, m[0], bold)
		put(summary, 2, row, m[1], 0)
		row++
	}
	row++
	for _, c := range r.Cards {
		put(summary, 1, row, c.Label, bold)
		putValue(f, summary, 2, row, c.Value, money)
		if c.Sub != "" {
			put(summary, 3, row, c.Sub, 0)
		}
		row++
	}

	used := map[string]bool{summary: true}
	for i, s := range r.Sections {
		switch s.Kind {
		case SectionTable:
			name := sheetName(s.Title, i, used)
			if _, err := f.NewSheet(name); err != nil {
				return nil, fmt.Errorf("xlsx sheet %q: %w", name, err)
			}
			for c, col := range s.Columns {
				put(name, c+1, 1, col.Header, header)
			}
			for ri, cells := range s.Rows {
				for c, cell := range cells {
					if c < len(s.Columns) && s.Columns[c].Money {
						putValue(f, name, c+1, ri+2, cell.Text, money)
					} else {
						put(name, c+1, ri+2, cell.Text, 0)
					}
				}
			}
			if len(s.Total) > 0 {
				tr := len(s.Rows) + 2
				for c, cell := range s.Total {
					if c < len(s.Columns) && s.Columns[c].Money {
						putValue(f, name, c+1, tr, cell.Text, moneyBold)
					} else {
						put(name, c+1, tr, cell.Text, bold)
					}
				}
			}
			for c, col := range s.Columns {
				colName, _ := excelize.ColumnNumberToName(c + 1)
				_ = f.SetColWidth(name, colName, colName, min(12+col.Weight*4, 60))
			}
			_ = f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		case SectionKeyValue:
			if s.Title != "" {
				put(summary, 1, row, s.Title, title)
				row++
			}
			for _, kv := range s.Pairs {
				style, labelStyle := money, 0
				if kv.Bold {
					style, labelStyle = moneyBold, bold
				}
				put(summary, 1, row, kv.Label, labelStyle)
				putValue(f, summary, 2, row, kv.Value, style)
				row++
			}
			row++
		case SectionChart:
			if s.Title != "" {
				put(summary, 1, row, s.Title, title)
				row++
			}
			for _, b := range s.Bars {
				put(summary, 1, row, b.Label, 0)
				put(summary, 2, row, b.Value, money)
				row++
			}
			row++
		case SectionProse:
			for _, para := range s.Paragraphs {
				put(summary, 1, row, strings.TrimPrefix(para, "**"), 0)
				row++
			}
		}
	}
	_ = f.SetColWidth(summary, "A", "A", 34)
	_ = f.SetColWidth(summary, "B", "C", 24)
	f.SetActiveSheet(0)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("render report xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// putValue writes an amount ("KES 1,234.50", "-500", "(1,000)") as a number when it parses,
// otherwise as text, so Excel can sum it.
func putValue(f *excelize.File, sheet string, col, row int, text string, style int) {
	ref, _ := excelize.CoordinatesToCellName(col, row)
	if n, ok := parseAmount(text); ok {
		_ = f.SetCellValue(sheet, ref, n)
		_ = f.SetCellStyle(sheet, ref, ref, style)
		return
	}
	_ = f.SetCellValue(sheet, ref, text)
}

// parseAmount strips a currency code, thousands separators and spaces; "(1,000)" is negative.
func parseAmount(text string) (float64, bool) {
	s := strings.TrimSpace(text)
	if s == "" {
		return 0, false
	}
	neg := strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")
	s = strings.Trim(s, "()")
	s = strings.TrimSpace(strings.TrimLeft(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"))
	s = strings.ReplaceAll(strings.ReplaceAll(s, ",", ""), " ", "")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

// sheetName makes a unique Excel sheet name from a title: at most 31 characters, none of []:*?/\.
func sheetName(title string, idx int, used map[string]bool) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" {
		name = fmt.Sprintf("Table %d", idx+1)
	}
	if r := []rune(name); len(r) > 28 {
		name = string(r[:28])
	}
	base, n := name, 2
	for used[name] {
		name = fmt.Sprintf("%s %d", base, n)
		n++
	}
	used[name] = true
	return name
}
