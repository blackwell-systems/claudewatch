package context

import (
	"math"
	"sort"
	"time"
)

// RankAndSort applies recency decay and sorts items by final score descending.
// Modifies items[].Score in place and sorts the slice.
func RankAndSort(items []ContextItem) {
	if len(items) == 0 {
		return
	}

	now := time.Now()

	// Apply recency decay to each item's score
	for i := range items {
		ageDays := now.Sub(items[i].Timestamp).Hours() / 24.0
		recencyFactor := math.Min(1.0, ageDays/365.0)
		items[i].Score = items[i].Score * (1.0 + 0.2*recencyFactor)
	}

	// Sort by final score descending (highest first)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})
}
