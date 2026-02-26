package claude

import (
	"path/filepath"
)

// ParseAllFacets reads all JSON files from ~/.claude/usage-data/facets/
// and returns parsed SessionFacet entries.
func ParseAllFacets(claudeHome string) ([]SessionFacet, error) {
	dir := filepath.Join(claudeHome, "usage-data", "facets")
	return parseJSONDir[SessionFacet](dir)
}
