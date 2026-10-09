// Package reports renders tenant-branded A4 report documents as PDF, CSV and Excel from one
// data-driven model. A service maps the data its JSON endpoints already compute into a Report
// and calls Generate; a new report is only a new assembly of Sections.
//
// It started as the report engine copied into pos-api, inventory-api and treasury-api; this module
// is the single copy, so a fix lands once for every service that uses it.
package reports

import "time"

// Report is the input model for every report document.
type Report struct {
	Title    string // e.g. "Arrears"
	Subtitle string // caption under the title

	// Branding, resolved by the caller from tenant branding.
	TenantName   string
	OutletName   string // a branch, outlet or property under the tenant
	Address      string // address line(s)
	PrimaryColor string // hex "#RRGGBB"; empty uses the default indigo
	LogoPNG      []byte // logo bytes (PNG, JPG or GIF); empty means no logo
	LogoType     string // "PNG", "JPG", "GIF" or "" to sniff

	// Meta.
	PeriodFrom  time.Time
	PeriodTo    time.Time
	GeneratedAt time.Time
	Currency    string      // e.g. "KES"
	Meta        [][2]string // extra key/value rows for the meta box

	// Layout.
	Landscape bool // wide tables render on A4 landscape

	// Content.
	Cards    []Card    // optional summary tiles across the top
	Sections []Section // the body, rendered in order
	Footer   string    // footer note; empty gives "Generated on Codevertex" and the time

	// ProviderFooterEnabled shows the platform owner's line at the foot of every page. The caller
	// resolves it (platform default on, per-tenant opt-out); this package only renders.
	ProviderFooterEnabled bool
}

// Card is a summary tile: a label, a large value and an optional caption.
type Card struct {
	Label string
	Value string
	Sub   string // a caption starting "+" shows in the good tone, a negative amount in the bad tone
}

// SectionKind selects how a Section renders.
type SectionKind int

const (
	SectionTable     SectionKind = iota // columns and rows, with an optional bold total row
	SectionKeyValue                     // label and value rows (totals)
	SectionChart                        // a bar chart
	SectionProse                        // wrapped paragraphs (agreements, letters, terms)
	SectionSignature                    // ruled signature blocks, two per row
)

// Section is one titled block of the report.
type Section struct {
	Kind  SectionKind
	Title string // heading; empty means none
	Note  string // optional caption under the heading

	// Table.
	Columns []Column
	Rows    [][]Cell
	Total   []Cell // optional bold total row, same arity as Columns

	// KeyValue.
	Pairs []KV

	// Chart. Bars render as columns (a time series, negatives drawn below zero) unless Horizontal
	// is set, which draws one row per bar: better for many categories or long labels, and it
	// paginates like a table.
	Bars       []Bar
	ValueUnit  string // optional unit for the values (e.g. a currency code)
	Horizontal bool

	// Prose: paragraphs in order with a gap between them. A paragraph starting "**" is bold
	// (clause lead-ins).
	Paragraphs []string

	// Signature blocks.
	Signatures []Signature
}

// Signature is one ruled signature block: a rule, a role label, an optional pre-filled name and an
// optional date line.
type Signature struct {
	Role      string // e.g. "For and on behalf of the Estate"
	Name      string // empty leaves the line blank for a wet-ink signature
	DateLabel string // e.g. "Date"; omitted when empty
}

// Column defines a table column.
type Column struct {
	Header string
	Weight float64 // relative width; columns share the content width in proportion
	Align  string  // "L", "R" or "C" (default "L")
	Money  bool    // right-aligned, and written to Excel as a number
}

// Cell is one table cell.
type Cell struct {
	Text string
	Bold bool
}

// KV is a label and value row in a key/value block.
type KV struct {
	Label string
	Value string
	Bold  bool // subtotal or grand total
	Rule  bool // sits on a tinted band (separates a total)
}

// Bar is one bar of a chart.
type Bar struct {
	Label string
	Value float64
}

// Text builds a plain cell.
func Text(s string) Cell { return Cell{Text: s} }

// BoldText builds a bold cell.
func BoldText(s string) Cell { return Cell{Text: s, Bold: true} }
