package claude

import "path/filepath"

// NormalizePath cleans a file path to a canonical form suitable for comparison.
// It resolves ".." components, removes trailing slashes, and normalizes
// separators. Returns an empty string for empty input.
func NormalizePath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}
