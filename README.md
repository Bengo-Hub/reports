# reports

Tenant-branded A4 report documents as PDF, CSV and Excel from one data-driven model, for every
Codevertex Go service.

```go
import "github.com/Bengo-Hub/reports"

r := &reports.Report{
	Title:        "Arrears",
	TenantName:   "Shaba Village Estate",
	PrimaryColor: "#8E2C76",
	GeneratedAt:  time.Now(),
	Currency:     "KES",
	Cards:        []reports.Card{{Label: "Owing", Value: reports.Money("KES", 310000)}},
	Sections: []reports.Section{{
		Kind:    reports.SectionTable,
		Title:   "Largest balances",
		Columns: []reports.Column{{Header: "Account", Weight: 2}, {Header: "Balance", Money: true}},
		Rows:    [][]reports.Cell{{reports.Text("SV-A1"), reports.Text(reports.Money("KES", 12500))}},
	}},
}
format, _ := reports.ParseFormat(req.URL.Query().Get("format")) // pdf (default), csv, xlsx
body, mime, err := reports.Generate(r, format)
```

## Sections

| Kind | Use |
|---|---|
| `SectionTable` | Columns and rows, optional bold total row; the header row repeats on every page |
| `SectionKeyValue` | Label and value rows; `Rule` puts a total on a tinted band |
| `SectionChart` | Columns for a series (negatives draw below zero), or `Horizontal` for many categories or long labels |
| `SectionProse` | Paragraphs; a leading `**` makes one bold |
| `SectionSignature` | Ruled signature blocks, two per row |

Excel gets a Summary sheet plus one sheet per table, with `Money` columns as real numbers.

## Rules the renderer keeps

- Page breaks use the current orientation's height (landscape A4 is 210mm tall). Assuming 297mm
  drew rows past the physical page, where they silently vanished.
- A section heading keeps at least three body rows with it, so a page never ends on a bare heading.
- Text is translated to cp1252 exactly once. `clip` returns UTF-8, and wrapping goes through
  `splitText`, because fpdf's `SplitText` panics on translated bytes above 0x7F.
- Status tones (good, bad) are fixed and never the brand colour.

## Origin

This is the report engine that was copied into pos-api, inventory-api and treasury-api, made one
module so a fix lands once. Moving those services onto it is tracked in each service's backlog.
Fixes made while extracting, which the copies still lack: accented text inside table cells, meta
and headers lost its accents (translated twice), and accented or dashed prose panicked in
`SplitText`.

## Test

```sh
go test ./...
REPORTS_SAMPLE_PDF=/tmp/sample.pdf go test -run TestGeneratePDF ./...   # write a sample to look at
```
