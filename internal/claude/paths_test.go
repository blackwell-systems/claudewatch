package claude

import (
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty string", "", ""},
		{"simple path", "/usr/local/bin", "/usr/local/bin"},
		{"trailing slash", "/usr/local/bin/", "/usr/local/bin"},
		{"double slash", "/usr//local//bin", "/usr/local/bin"},
		{"dot-dot components", "/usr/local/../bin", "/usr/bin"},
		{"dot components", "/usr/./local/./bin", "/usr/local/bin"},
		{"relative path", "foo/bar", "foo/bar"},
		{"relative with dot-dot", "foo/../bar", "bar"},
		{"root", "/", "/"},
		{"just dot", ".", "."},
		{"dot-dot only", "..", ".."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePath(tc.input)
			if got != tc.expect {
				t.Errorf("NormalizePath(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}
