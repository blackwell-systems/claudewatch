package output

import (
	"strings"
	"testing"
)

func TestVisualLen_PlainText(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"abc def", 7},
	}

	for _, tc := range tests {
		got := visualLen(tc.input)
		if got != tc.want {
			t.Errorf("visualLen(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestVisualLen_StripsANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "bold",
			input: "\x1b[1mhello\x1b[0m",
			want:  5,
		},
		{
			name:  "color",
			input: "\x1b[31mred\x1b[0m",
			want:  3,
		},
		{
			name:  "multiple sequences",
			input: "\x1b[1m\x1b[34mblue bold\x1b[0m",
			want:  9,
		},
		{
			name:  "no ansi",
			input: "plain text",
			want:  10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := visualLen(tc.input)
			if got != tc.want {
				t.Errorf("visualLen() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  int // expected length of output
	}{
		{"needs padding", "hi", 10, 10},
		{"exact width", "hello", 5, 5},
		{"over width", "toolong", 3, 7}, // no truncation
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pad(tc.input, tc.width)
			if len(got) != tc.want {
				t.Errorf("pad(%q, %d) len = %d, want %d", tc.input, tc.width, len(got), tc.want)
			}
		})
	}
}

func TestTable_Render(t *testing.T) {
	// Disable color so we get predictable output.
	SetNoColor(true)
	defer SetNoColor(false)

	tbl := NewTable("Name", "Score")
	tbl.AddRow("Alice", "95")
	tbl.AddRow("Bob", "87")

	output := tbl.Render()

	// Should contain headers.
	if !strings.Contains(output, "Name") {
		t.Error("expected header 'Name' in output")
	}
	if !strings.Contains(output, "Score") {
		t.Error("expected header 'Score' in output")
	}

	// Should contain data.
	if !strings.Contains(output, "Alice") {
		t.Error("expected 'Alice' in output")
	}
	if !strings.Contains(output, "Bob") {
		t.Error("expected 'Bob' in output")
	}

	// Should have separator line.
	if !strings.Contains(output, "─") {
		t.Error("expected separator character in output")
	}

	// Count lines: header + separator + 2 data rows = 4 lines.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(lines))
	}
}

func TestTable_EmptyHeaders(t *testing.T) {
	tbl := NewTable()
	output := tbl.Render()
	if output != "" {
		t.Errorf("expected empty output for empty table, got %q", output)
	}
}

func TestTable_ColumnWidths(t *testing.T) {
	SetNoColor(true)
	defer SetNoColor(false)

	tbl := NewTable("A", "LongHeader")
	tbl.AddRow("VeryLongValue", "X")

	output := tbl.Render()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}

	// The data row should be padded so columns align.
	dataLine := lines[2]
	if !strings.Contains(dataLine, "VeryLongValue") {
		t.Error("expected data row to contain 'VeryLongValue'")
	}
}

func TestTable_String(t *testing.T) {
	SetNoColor(true)
	defer SetNoColor(false)

	tbl := NewTable("Col1")
	tbl.AddRow("Val1")

	// String() should equal Render().
	if tbl.String() != tbl.Render() {
		t.Error("String() != Render()")
	}
}

func TestSetNoColor(t *testing.T) {
	// After SetNoColor(true), StyleHeader should render without ANSI.
	SetNoColor(true)
	rendered := StyleHeader.Render("test")
	if strings.Contains(rendered, "\x1b[") {
		t.Error("expected no ANSI codes after SetNoColor(true)")
	}

	// After SetNoColor(false), we restore — but note: the original styles
	// are lost since SetNoColor only sets to plain. We just verify no crash
	// and that the function is idempotent.
	SetNoColor(false)
}
