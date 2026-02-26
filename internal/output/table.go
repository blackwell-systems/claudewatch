package output

import (
	"fmt"
	"regexp"
	"strings"
)

// ansiRegex matches ANSI escape sequences used for terminal styling.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visualLen returns the display width of a string, excluding ANSI escape codes.
func visualLen(s string) int {
	return len(ansiRegex.ReplaceAllString(s, ""))
}

// Table is a simple styled table renderer.
type Table struct {
	headers []string
	rows    [][]string
	widths  []int
}

// NewTable creates a new table with the given column headers.
func NewTable(headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = visualLen(h)
	}
	return &Table{
		headers: headers,
		widths:  widths,
	}
}

// AddRow adds a row of values to the table. The number of values should
// match the number of headers.
func (t *Table) AddRow(values ...string) {
	row := make([]string, len(t.headers))
	for i := range t.headers {
		if i < len(values) {
			row[i] = values[i]
		}
		if vl := visualLen(row[i]); vl > t.widths[i] {
			t.widths[i] = vl
		}
	}
	t.rows = append(t.rows, row)
}

// Render returns the formatted table as a string.
func (t *Table) Render() string {
	if len(t.headers) == 0 {
		return ""
	}

	var sb strings.Builder

	// Header row.
	for i, h := range t.headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(StyleHeader.Render(pad(h, t.widths[i])))
	}
	sb.WriteString("\n")

	// Separator.
	for i, w := range t.widths {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(StyleMuted.Render(strings.Repeat("─", w)))
	}
	sb.WriteString("\n")

	// Data rows.
	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(pad(cell, t.widths[i]))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// String implements fmt.Stringer.
func (t *Table) String() string {
	return t.Render()
}

// Print writes the table to stdout.
func (t *Table) Print() {
	fmt.Print(t.Render())
}

// pad right-pads a string to the given visual width, accounting for ANSI escapes.
func pad(s string, width int) string {
	vl := visualLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}
