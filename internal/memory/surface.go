package memory

import (
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// MemorySurfaceResult contains tasks matching the user's message and the keywords that triggered matches.
type MemorySurfaceResult struct {
	MatchedTasks []*store.TaskMemory
	Keywords     []string
}

// SurfaceRelevantMemory extracts keywords from userMessage and queries task history.
// Returns matching tasks and the keywords that triggered matches.
// Uses simple substring matching: splits userMessage on whitespace, filters stop words,
// matches remaining tokens against TaskIdentifier fields case-insensitively.
func SurfaceRelevantMemory(userMessage string, projectName string, memStore *store.WorkingMemoryStore) (*MemorySurfaceResult, error) {
	// Extract keywords from user message.
	keywords := extractKeywords(userMessage)
	if len(keywords) == 0 {
		return &MemorySurfaceResult{
			MatchedTasks: []*store.TaskMemory{},
			Keywords:     []string{},
		}, nil
	}

	// Query task history for each keyword and deduplicate.
	matchedMap := make(map[string]*store.TaskMemory)
	for _, keyword := range keywords {
		tasks, err := memStore.GetTaskHistory(keyword)
		if err != nil {
			continue
		}
		for _, task := range tasks {
			matchedMap[task.TaskIdentifier] = task
		}
	}

	// Convert map to slice.
	var matched []*store.TaskMemory
	for _, task := range matchedMap {
		matched = append(matched, task)
	}

	// Sort by LastUpdated descending (most recent first).
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].LastUpdated.After(matched[j].LastUpdated)
	})

	return &MemorySurfaceResult{
		MatchedTasks: matched,
		Keywords:     keywords,
	}, nil
}

// extractKeywords splits message on whitespace, filters stop words, and enforces minimum length.
func extractKeywords(message string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
		"to": true, "of": true, "in": true, "for": true, "on": true, "with": true,
		"as": true, "by": true, "at": true, "from": true, "this": true, "that": true,
		"it": true, "can": true, "will": true, "we": true, "i": true, "you": true,
		"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "shall": true, "not": true, "no": true, "yes": true, "if": true,
		"then": true, "else": true, "when": true, "where": true, "who": true, "what": true,
		"why": true, "how": true, "which": true, "there": true, "here": true, "into": true,
		"out": true, "up": true, "down": true, "over": true, "under": true, "again": true,
		"further": true, "than": true, "about": true, "against": true, "between": true,
		"through": true, "during": true, "before": true, "after": true, "above": true,
		"below": true, "so": true, "because": true, "while": true, "until": true,
		"its": true, "our": true, "their": true, "my": true, "your": true, "his": true,
		"her": true, "all": true, "some": true, "any": true, "each": true, "every": true,
		"both": true, "few": true, "more": true, "most": true, "other": true, "such": true,
		"only": true, "own": true, "same": true, "too": true, "very": true,
	}

	words := strings.Fields(strings.ToLower(message))
	var keywords []string
	seen := make(map[string]bool)

	for _, word := range words {
		// Strip punctuation from edges.
		cleaned := strings.Trim(word, ",.!?;:()[]{}\"'`")

		// Filter: minimum length 3, not a stop word, not already seen.
		if len(cleaned) >= 3 && !stopWords[cleaned] && !seen[cleaned] {
			keywords = append(keywords, cleaned)
			seen[cleaned] = true
		}
	}

	return keywords
}
