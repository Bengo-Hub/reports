package reports

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// Format is an output format.
type Format string

const (
	FormatPDF  Format = "pdf"
	FormatCSV  Format = "csv"
	FormatXLSX Format = "xlsx"
)

const (
	MimePDF  = "application/pdf"
	MimeCSV  = "text/csv"
	MimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// Ext is the file extension for the format.
func (f Format) Ext() string { return string(f) }

// Generate renders the report in the format, returning the bytes and their MIME type.
func Generate(r *Report, format Format) ([]byte, string, error) {
	switch format {
	case FormatPDF:
		b, err := renderPDF(r)
		return b, MimePDF, err
	case FormatCSV:
		b, err := renderCSV(r)
		return b, MimeCSV, err
	case FormatXLSX:
		b, err := renderXLSX(r)
		return b, MimeXLSX, err
	default:
		return nil, "", fmt.Errorf("unsupported report format: %q", format)
	}
}

// ParseFormat reads a ?format= value: empty or "pdf" is PDF, "csv", and "xlsx" or "excel".
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "pdf":
		return FormatPDF, nil
	case "csv":
		return FormatCSV, nil
	case "xlsx", "excel":
		return FormatXLSX, nil
	default:
		return "", fmt.Errorf("unsupported format %q: must be pdf, csv or xlsx", s)
	}
}

// renderCSV flattens tables and key/value blocks into one CSV. Chart bars are written as label and
// value rows so no data is lost in the export.
func renderCSV(r *Report) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	_ = w.Write([]string{r.Title})
	if r.Subtitle != "" {
		_ = w.Write([]string{r.Subtitle})
	}
	if !r.PeriodFrom.IsZero() || !r.PeriodTo.IsZero() {
		_ = w.Write([]string{"Period", Date(r.PeriodFrom), Date(r.PeriodTo)})
	}
	for _, m := range r.Meta {
		_ = w.Write([]string{m[0], m[1]})
	}
	_ = w.Write([]string{})

	for _, s := range r.Sections {
		if s.Title != "" {
			_ = w.Write([]string{s.Title})
		}
		switch s.Kind {
		case SectionTable:
			hdr := make([]string, len(s.Columns))
			for i, c := range s.Columns {
				hdr[i] = c.Header
			}
			_ = w.Write(hdr)
			for _, row := range s.Rows {
				_ = w.Write(cellTexts(row))
			}
			if len(s.Total) > 0 {
				_ = w.Write(cellTexts(s.Total))
			}
		case SectionKeyValue:
			for _, kv := range s.Pairs {
				_ = w.Write([]string{kv.Label, kv.Value})
			}
		case SectionChart:
			for _, b := range s.Bars {
				_ = w.Write([]string{b.Label, Amount(b.Value)})
			}
		case SectionProse:
			for _, para := range s.Paragraphs {
				_ = w.Write([]string{strings.TrimPrefix(para, "**")})
			}
		case SectionSignature:
			for _, sig := range s.Signatures {
				_ = w.Write([]string{sig.Role, sig.Name})
			}
		}
		_ = w.Write([]string{})
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("render report csv: %w", err)
	}
	return buf.Bytes(), nil
}

func cellTexts(cells []Cell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.Text
	}
	return out
}
