package context

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// DeduplicateItems removes duplicate items based on content hash.
// Within each hash group, keeps the item with highest source priority.
// Priority order: commit > memory > task_history > transcript.
func DeduplicateItems(items []ContextItem) []ContextItem {
	if len(items) == 0 {
		return items
	}

	// First pass: compute content hashes if not already set
	for i := range items {
		if items[i].ContentHash == "" {
			items[i].ContentHash = computeContentHash(items[i].Snippet)
		}
	}

	// Group by content hash
	hashGroups := make(map[string][]ContextItem)
	for _, item := range items {
		hashGroups[item.ContentHash] = append(hashGroups[item.ContentHash], item)
	}

	// Keep highest priority item from each group
	var result []ContextItem
	for _, group := range hashGroups {
		best := group[0]
		for _, item := range group[1:] {
			if sourcePriority(item.Source) > sourcePriority(best.Source) {
				best = item
			}
		}
		result = append(result, best)
	}

	return result
}

// computeContentHash computes SHA-256 hash of normalized content.
// Normalization: lowercase, trim whitespace.
func computeContentHash(content string) string {
	normalized := strings.TrimSpace(strings.ToLower(content))
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}

// sourcePriority returns priority value for source type.
// Higher value = higher priority.
// Priority order: commit > memory > task_history > transcript.
func sourcePriority(s SourceType) int {
	switch s {
	case SourceCommit:
		return 4
	case SourceMemory:
		return 3
	case SourceTaskHistory:
		return 2
	case SourceTranscript:
		return 1
	default:
		return 0
	}
}
