package mcp

import (
	"encoding/json"
	"testing"
)

// TestGetSessionFriction_RequiresSessionID verifies that an empty session_id returns an error.
func TestGetSessionFriction_RequiresSessionID(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	_, err := s.handleGetSessionFriction(json.RawMessage(`{"session_id":""}`))
	if err == nil {
		t.Fatal("expected error for empty session_id, got nil")
	}
	if err.Error() != "session_id is required" {
		t.Errorf("expected 'session_id is required', got %q", err.Error())
	}
}

// TestGetSessionFriction_NotFound verifies that an unknown session_id returns an empty result and no error.
func TestGetSessionFriction_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	result, err := s.handleGetSessionFriction(json.RawMessage(`{"session_id":"nonexistent-session"}`))
	if err != nil {
		t.Fatalf("expected no error for unknown session_id, got: %v", err)
	}

	r, ok := result.(SessionFrictionResult)
	if !ok {
		t.Fatalf("expected SessionFrictionResult, got %T", result)
	}

	if r.SessionID != "nonexistent-session" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "nonexistent-session")
	}
	if r.TotalFriction != 0 {
		t.Errorf("TotalFriction = %d, want 0", r.TotalFriction)
	}
	if r.FrictionCounts == nil {
		t.Error("FrictionCounts must not be nil")
	}
	if len(r.FrictionCounts) != 0 {
		t.Errorf("FrictionCounts must be empty, got %v", r.FrictionCounts)
	}
	if r.TopFrictionType != "" {
		t.Errorf("TopFrictionType = %q, want empty string", r.TopFrictionType)
	}
}

// TestGetSessionFriction_NoFriction verifies that a session with empty FrictionCounts returns TotalFriction=0.
func TestGetSessionFriction_NoFriction(t *testing.T) {
	dir := t.TempDir()
	writeFacet(t, dir, "no-friction-sess", map[string]int{})

	s := newTestServer(dir, 0)

	result, err := s.handleGetSessionFriction(json.RawMessage(`{"session_id":"no-friction-sess"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionFrictionResult)
	if !ok {
		t.Fatalf("expected SessionFrictionResult, got %T", result)
	}

	if r.SessionID != "no-friction-sess" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "no-friction-sess")
	}
	if r.TotalFriction != 0 {
		t.Errorf("TotalFriction = %d, want 0", r.TotalFriction)
	}
	if r.TopFrictionType != "" {
		t.Errorf("TopFrictionType = %q, want empty string", r.TopFrictionType)
	}
}

// TestGetSessionFriction_WithFriction verifies correct TotalFriction and TopFrictionType with friction data.
func TestGetSessionFriction_WithFriction(t *testing.T) {
	dir := t.TempDir()
	writeFacet(t, dir, "friction-sess", map[string]int{
		"wrong_approach": 3,
		"off_track":      2,
	})

	s := newTestServer(dir, 0)

	result, err := s.handleGetSessionFriction(json.RawMessage(`{"session_id":"friction-sess"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionFrictionResult)
	if !ok {
		t.Fatalf("expected SessionFrictionResult, got %T", result)
	}

	if r.SessionID != "friction-sess" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "friction-sess")
	}
	if r.TotalFriction != 5 {
		t.Errorf("TotalFriction = %d, want 5", r.TotalFriction)
	}
	if r.TopFrictionType != "wrong_approach" {
		t.Errorf("TopFrictionType = %q, want %q", r.TopFrictionType, "wrong_approach")
	}
	if r.FrictionCounts == nil {
		t.Error("FrictionCounts must not be nil")
	}
}

// TestGetSessionFriction_TopFrictionType verifies that the highest-count friction type is selected.
func TestGetSessionFriction_TopFrictionType(t *testing.T) {
	dir := t.TempDir()
	writeFacet(t, dir, "top-friction-sess", map[string]int{
		"wrong_approach":   1,
		"miscommunication": 10,
		"off_track":        3,
	})

	s := newTestServer(dir, 0)

	result, err := s.handleGetSessionFriction(json.RawMessage(`{"session_id":"top-friction-sess"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionFrictionResult)
	if !ok {
		t.Fatalf("expected SessionFrictionResult, got %T", result)
	}

	if r.TopFrictionType != "miscommunication" {
		t.Errorf("TopFrictionType = %q, want %q", r.TopFrictionType, "miscommunication")
	}
	if r.TotalFriction != 14 {
		t.Errorf("TotalFriction = %d, want 14", r.TotalFriction)
	}
}

// TestGetSessionFriction_TieBreak verifies that equal counts result in lexicographically first key.
func TestGetSessionFriction_TieBreak(t *testing.T) {
	dir := t.TempDir()
	writeFacet(t, dir, "tiebreak-sess", map[string]int{
		"zebra": 5,
		"apple": 5,
		"mango": 5,
	})

	s := newTestServer(dir, 0)

	result, err := s.handleGetSessionFriction(json.RawMessage(`{"session_id":"tiebreak-sess"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionFrictionResult)
	if !ok {
		t.Fatalf("expected SessionFrictionResult, got %T", result)
	}

	// "apple" is lexicographically first among "apple", "mango", "zebra".
	if r.TopFrictionType != "apple" {
		t.Errorf("TopFrictionType = %q, want %q (alphabetically first in tie)", r.TopFrictionType, "apple")
	}
	if r.TotalFriction != 15 {
		t.Errorf("TotalFriction = %d, want 15", r.TotalFriction)
	}
}
