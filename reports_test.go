package reports

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

// sampleReport exercises cards, branding and every section kind.
func sampleReport() *Report {
	return &Report{
		Title:        "Collections",
		Subtitle:     "Monthly summary",
		TenantName:   "Shaba Village Estate",
		Address:      "P.O. Box 1234-00100\nNairobi, Kenya",
		PrimaryColor: "#8E2C76",
		PeriodFrom:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodTo:     time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		GeneratedAt:  time.Date(2026, 10, 9, 10, 30, 0, 0, time.UTC),
		Currency:     "KES",
		Cards: []Card{
			{Label: "Collected", Value: "KES 1,250,000.00", Sub: "+12.4% on August"},
			{Label: "Owing", Value: "KES 310,000.00", Sub: "-3.0% on August"},
		},
		Sections: []Section{
			{Kind: SectionChart, Title: "By month", Bars: []Bar{{Label: "Jul", Value: 42000}, {Label: "Aug", Value: -5100}, {Label: "Sep", Value: 38000}}},
			{Kind: SectionChart, Title: "By charge", Horizontal: true, Bars: []Bar{{Label: "Service charge", Value: 900000}, {Label: "Water", Value: 350000}}},
			{
				Kind: SectionTable, Title: "By block",
				Columns: []Column{{Header: "Block", Weight: 3}, {Header: "Units", Weight: 1, Align: "R"}, {Header: "Collected", Weight: 2, Money: true}},
				Rows:    [][]Cell{{Text("Block A Café"), Text("120"), Text("KES 900,000.00")}, {Text("Block B"), Text("22"), Text("KES 350,000.00")}},
				Total:   []Cell{BoldText("Total"), BoldText("142"), BoldText("KES 1,250,000.00")},
			},
			{Kind: SectionKeyValue, Title: "Totals", Pairs: []KV{{Label: "Billed", Value: "KES 1,560,000.00"}, {Label: "Collected", Value: "KES 1,250,000.00", Bold: true, Rule: true}}},
			{Kind: SectionProse, Title: "Notes", Paragraphs: []string{"**Arrears", "Accounts over 60 days get a demand letter."}},
			{Kind: SectionSignature, Signatures: []Signature{{Role: "Estate manager", DateLabel: "Date"}, {Role: "Treasurer", Name: "J. Doe"}}},
		},
		ProviderFooterEnabled: true,
	}
}

// TestGeneratePDF also writes the sample to $REPORTS_SAMPLE_PDF when set, to look at by eye: a
// valid PDF does not prove nothing was dropped or misplaced.
func TestGeneratePDF(t *testing.T) {
	b, mime, err := Generate(sampleReport(), FormatPDF)
	if err != nil || mime != MimePDF {
		t.Fatalf("generate pdf: %v (mime %q)", err, mime)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 500 {
		t.Fatalf("not a PDF or suspiciously small: %d bytes", len(b))
	}
	if out := os.Getenv("REPORTS_SAMPLE_PDF"); out != "" {
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLandscapePagination guards the dropped-rows bug: page breaks on a landscape page must use
// its 210mm height. With 297mm assumed, 80 rows fit on 3 pages because rows were drawn past the
// physical bottom where they never render; at the true height they need at least 4.
func TestLandscapePagination(t *testing.T) {
	rows := make([][]Cell, 80)
	for i := range rows {
		rows[i] = []Cell{Text(fmt.Sprintf("A%03d", i+1)), Text("KES 1,000.00")}
	}
	r := &Report{Title: "Units", Landscape: true, GeneratedAt: time.Now(), Sections: []Section{{
		Kind: SectionTable, Title: "Units", Columns: []Column{{Header: "Unit"}, {Header: "Balance", Money: true}}, Rows: rows,
	}}}
	pdf := layout(r)
	if err := pdf.Error(); err != nil {
		t.Fatal(err)
	}
	if n := pdf.PageCount(); n < 4 {
		t.Fatalf("80 landscape rows on %d pages; rows are being drawn off the page", n)
	}
	// fpdf converts through points, so sizes come back a hair off.
	if w, h := pdf.GetPageSize(); math.Abs(w-pageWL) > 0.01 || math.Abs(h-pageHL) > 0.01 {
		t.Fatalf("landscape page is %vx%v", w, h)
	}
}

// TestClipKeepsAccents guards the double translation: clip returns UTF-8, so text drawn through it
// is translated once and keeps accented letters.
func TestClipKeepsAccents(t *testing.T) {
	p := newPainter(fpdf.New("P", "mm", "A4", ""), newPalette(""), pageWP)
	if got := p.clip("Café", "", 8, 100); got != "Café" {
		t.Fatalf("clip changed a fitting string: %q", got)
	}
	got := p.clip("Résidence des Jardins de Karen", "", 8, 20)
	if r := []rune(got); r[len(r)-1] != '…' || r[1] != 'é' {
		t.Fatalf("clipped string lost its accent or ellipsis: %q", got)
	}
}

// TestProseWithAccents guards the SplitText panic: fpdf's SplitText reads its input as runes, so
// translated cp1252 bytes above 0x7F decode as U+FFFD and index past its width table.
func TestProseWithAccents(t *testing.T) {
	r := &Report{Title: "Terms", GeneratedAt: time.Now(), Sections: []Section{{
		Kind: SectionProse, Paragraphs: []string{"**Café terms — clause 1", "Owners at Résidence Karen pay the service charge • quarterly × 4."},
	}}}
	if _, _, err := Generate(r, FormatPDF); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCSV(t *testing.T) {
	b, mime, err := Generate(sampleReport(), FormatCSV)
	if err != nil || mime != MimeCSV {
		t.Fatalf("generate csv: %v (mime %q)", err, mime)
	}
	for _, want := range []string{"By block", "Block A Café", "Service charge", "Estate manager"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Fatalf("CSV missing %q:\n%s", want, b)
		}
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{"": FormatPDF, "pdf": FormatPDF, "PDF": FormatPDF, "csv": FormatCSV, "xlsx": FormatXLSX, "Excel": FormatXLSX}
	for in, want := range cases {
		if got, err := ParseFormat(in); err != nil || got != want {
			t.Fatalf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseFormat("docx"); err == nil {
		t.Fatal("expected an error for docx")
	}
}

// TestGenerateXLSX reads the workbook back: a Summary sheet plus one per table, money as numbers.
func TestGenerateXLSX(t *testing.T) {
	rep := &Report{
		Title: "Expenses", Currency: "KES",
		Cards: []Card{{Label: "Total spend", Value: "KES 1,500.30"}},
		Sections: []Section{{
			Kind: SectionTable, Title: "Expenses",
			Columns: []Column{{Header: "Number", Weight: 2}, {Header: "Amount", Weight: 2, Money: true}},
			Rows:    [][]Cell{{Text("EXP-1"), Text("KES 1,000.10")}, {Text("EXP-2"), Text("(500.20)")}},
			Total:   []Cell{BoldText("Total"), BoldText("KES 499.90")},
		}},
	}
	b, mime, err := Generate(rep, FormatXLSX)
	if err != nil || mime != MimeXLSX || len(b) == 0 {
		t.Fatalf("generate xlsx: %v (mime %q, %d bytes)", err, mime, len(b))
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()
	if sheets := f.GetSheetList(); len(sheets) != 2 || sheets[0] != "Summary" || sheets[1] != "Expenses" {
		t.Fatalf("sheets = %v", sheets)
	}
	if v, _ := f.GetCellValue("Expenses", "B2", excelize.Options{RawCellValue: true}); v != "1000.1" {
		t.Errorf("B2 raw = %q, want 1000.1", v)
	}
	if v, _ := f.GetCellValue("Expenses", "B3", excelize.Options{RawCellValue: true}); v != "-500.2" {
		t.Errorf("B3 raw = %q, want -500.2", v)
	}
}

func TestFormatters(t *testing.T) {
	cases := map[string]string{
		Amount(1234.5):       "1,234.50",
		Amount(-1234567.891): "-1,234,567.89",
		Amount(0):            "0.00",
		Money("", 99):        "KES 99.00",
		Money("USD", 1000):   "USD 1,000.00",
		Quantity(2.500):      "2.5",
		Quantity(12):         "12",
		Date(time.Time{}):    "-",
		compactNumber(48300): "48.3K",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if !isNegativeAmount("KES -1,200.00") || isNegativeAmount("KES 1,200.00") {
		t.Error("isNegativeAmount is wrong")
	}
}
