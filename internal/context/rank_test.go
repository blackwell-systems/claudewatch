package context

import (
	"testing"
	"time"
)

func TestRankAndSort_RecencyBoost(t *testing.T) {
	now := time.Now()
	oldTime := now.AddDate(-1, 0, 0) // 1 year ago
	recentTime := now.AddDate(0, 0, -1) // 1 day ago

	items := []ContextItem{
		{
			Title:     "Old item",
			Snippet:   "old content",
			Timestamp: oldTime,
			Score:     0.5,
		},
		{
			Title:     "Recent item",
			Snippet:   "recent content",
			Timestamp: recentTime,
			Score:     0.5, // same base score
		},
	}

	RankAndSort(items)

	// Recent item should have slightly higher final score due to recency boost
	// Old item: 365 days ago, recency_factor = min(1.0, 365/365) = 1.0
	// final_score = 0.5 * (1.0 + 0.2 * 1.0) = 0.5 * 1.2 = 0.6
	//
	// Recent item: 1 day ago, recency_factor = min(1.0, 1/365) ≈ 0.0027
	// final_score = 0.5 * (1.0 + 0.2 * 0.0027) ≈ 0.50027

	if items[0].Title != "Old item" {
		t.Errorf("Expected old item first (higher recency boost), got %s", items[0].Title)
	}

	// Verify that the old item's score increased more
	if items[0].Score <= 0.5 {
		t.Errorf("Expected old item score to increase from 0.5, got %f", items[0].Score)
	}

	// The recent item should have increased slightly
	if items[1].Score <= 0.5 {
		t.Errorf("Expected recent item score to increase from 0.5, got %f", items[1].Score)
	}

	// Old item should have higher final score (more decay)
	if items[0].Score <= items[1].Score {
		t.Errorf("Expected old item (%f) to have higher score than recent item (%f) due to recency factor", items[0].Score, items[1].Score)
	}
}

func TestRankAndSort_StableSortForEqualScores(t *testing.T) {
	now := time.Now()

	items := []ContextItem{
		{Title: "First", Snippet: "a", Timestamp: now, Score: 0.5},
		{Title: "Second", Snippet: "b", Timestamp: now, Score: 0.5},
		{Title: "Third", Snippet: "c", Timestamp: now, Score: 0.5},
	}

	RankAndSort(items)

	// With same timestamp and same score, stable sort should preserve original order
	if items[0].Title != "First" || items[1].Title != "Second" || items[2].Title != "Third" {
		t.Errorf("Expected stable sort to preserve order, got %s, %s, %s", items[0].Title, items[1].Title, items[2].Title)
	}
}

func TestRankAndSort_DescendingScoreOrder(t *testing.T) {
	now := time.Now()

	items := []ContextItem{
		{Title: "Low", Snippet: "a", Timestamp: now, Score: 0.3},
		{Title: "High", Snippet: "b", Timestamp: now, Score: 0.9},
		{Title: "Medium", Snippet: "c", Timestamp: now, Score: 0.6},
	}

	RankAndSort(items)

	// Should be sorted high to low
	if items[0].Title != "High" || items[1].Title != "Medium" || items[2].Title != "Low" {
		t.Errorf("Expected High, Medium, Low order, got %s, %s, %s", items[0].Title, items[1].Title, items[2].Title)
	}
}

func TestDeduplicateItems_RemovesExactDuplicates(t *testing.T) {
	items := []ContextItem{
		{
			Source:  SourceMemory,
			Title:   "Memory item",
			Snippet: "  EXACT CONTENT  ", // will be normalized
			Score:   0.8,
		},
		{
			Source:  SourceTranscript,
			Title:   "Transcript item",
			Snippet: "exact content", // same after normalization
			Score:   0.7,
		},
	}

	result := DeduplicateItems(items)

	if len(result) != 1 {
		t.Fatalf("Expected 1 item after dedup, got %d", len(result))
	}

	// Memory has higher priority than transcript
	if result[0].Source != SourceMemory {
		t.Errorf("Expected memory source (higher priority), got %s", result[0].Source)
	}
}

func TestDeduplicateItems_SourcePriority(t *testing.T) {
	items := []ContextItem{
		{Source: SourceTranscript, Snippet: "content", Score: 0.9}, // lowest priority
		{Source: SourceTaskHistory, Snippet: "content", Score: 0.8},
		{Source: SourceMemory, Snippet: "content", Score: 0.7},
		{Source: SourceCommit, Snippet: "content", Score: 0.6}, // highest priority
	}

	result := DeduplicateItems(items)

	if len(result) != 1 {
		t.Fatalf("Expected 1 item after dedup, got %d", len(result))
	}

	// Commit has highest priority
	if result[0].Source != SourceCommit {
		t.Errorf("Expected commit source (highest priority), got %s", result[0].Source)
	}

	// Should keep the score from the highest priority source
	if result[0].Score != 0.6 {
		t.Errorf("Expected score 0.6 from commit item, got %f", result[0].Score)
	}
}

func TestDeduplicateItems_PreservesDifferentContent(t *testing.T) {
	items := []ContextItem{
		{Source: SourceMemory, Snippet: "content A", Score: 0.8},
		{Source: SourceMemory, Snippet: "content B", Score: 0.7},
		{Source: SourceMemory, Snippet: "content C", Score: 0.6},
	}

	result := DeduplicateItems(items)

	if len(result) != 3 {
		t.Fatalf("Expected 3 items (different content), got %d", len(result))
	}
}

func TestDeduplicateItems_EmptyInput(t *testing.T) {
	items := []ContextItem{}
	result := DeduplicateItems(items)

	if len(result) != 0 {
		t.Errorf("Expected empty result for empty input, got %d items", len(result))
	}
}

func TestComputeContentHash_Normalization(t *testing.T) {
	// These should all produce the same hash
	testCases := []string{
		"Test Content",
		"test content",
		"  test content  ",
		"TEST CONTENT",
		"\n  Test Content\t",
	}

	var hashes []string
	for _, tc := range testCases {
		hashes = append(hashes, computeContentHash(tc))
	}

	// All hashes should be identical
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("Expected all hashes to match, but hash[%d] (%s) != hash[0] (%s)", i, hashes[i], hashes[0])
		}
	}
}

func TestSourcePriority(t *testing.T) {
	priorities := map[SourceType]int{
		SourceCommit:      4,
		SourceMemory:      3,
		SourceTaskHistory: 2,
		SourceTranscript:  1,
	}

	for source, expected := range priorities {
		got := sourcePriority(source)
		if got != expected {
			t.Errorf("Expected priority %d for %s, got %d", expected, source, got)
		}
	}

	// Test that commit > memory > task_history > transcript
	if sourcePriority(SourceCommit) <= sourcePriority(SourceMemory) {
		t.Error("Commit should have higher priority than Memory")
	}
	if sourcePriority(SourceMemory) <= sourcePriority(SourceTaskHistory) {
		t.Error("Memory should have higher priority than TaskHistory")
	}
	if sourcePriority(SourceTaskHistory) <= sourcePriority(SourceTranscript) {
		t.Error("TaskHistory should have higher priority than Transcript")
	}
}
