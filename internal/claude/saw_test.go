package claude

import (
	"testing"
	"time"
)

func TestParseSAWTag_Valid(t *testing.T) {
	wave, agent, ok := ParseSAWTag("[SAW:wave1:agent-A] desc")
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if wave != 1 {
		t.Errorf("expected wave=1, got %d", wave)
	}
	if agent != "A" {
		t.Errorf("expected agent=A, got %q", agent)
	}
}

func TestParseSAWTag_ValidWave2AgentB(t *testing.T) {
	wave, agent, ok := ParseSAWTag("[SAW:wave2:agent-B] x")
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if wave != 2 {
		t.Errorf("expected wave=2, got %d", wave)
	}
	if agent != "B" {
		t.Errorf("expected agent=B, got %q", agent)
	}
}

func TestParseSAWTag_NoTag(t *testing.T) {
	wave, agent, ok := ParseSAWTag("")
	if ok {
		t.Fatal("expected ok=false, got true")
	}
	if wave != 0 {
		t.Errorf("expected wave=0, got %d", wave)
	}
	if agent != "" {
		t.Errorf("expected agent='', got %q", agent)
	}
}

func TestParseSAWTag_MalformedMissingBracket(t *testing.T) {
	_, _, ok := ParseSAWTag("SAW:wave1:agent-A desc")
	if ok {
		t.Fatal("expected ok=false for missing bracket, got true")
	}
}

func TestParseSAWTag_MalformedBadWave(t *testing.T) {
	_, _, ok := ParseSAWTag("[SAW:waveX:agent-A] desc")
	if ok {
		t.Fatal("expected ok=false for bad wave number, got true")
	}
}

func TestParseSAWTag_MalformedEmptyAgent(t *testing.T) {
	_, _, ok := ParseSAWTag("[SAW:wave1:agent-] desc")
	if ok {
		t.Fatal("expected ok=false for empty agent, got true")
	}
}

func TestComputeSAWWaves_Empty(t *testing.T) {
	result := ComputeSAWWaves(nil)
	if result == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got len=%d", len(result))
	}
}

func TestComputeSAWWaves_SingleWave(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	spans := []AgentSpan{
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave1:agent-A] task a", LaunchedAt: t0, CompletedAt: t1, Success: true},
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave1:agent-B] task b", LaunchedAt: t0, CompletedAt: t1, Success: true},
	}
	sessions := ComputeSAWWaves(spans)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	sess := sessions[0]
	if sess.SessionID != "s1" {
		t.Errorf("expected SessionID=s1, got %q", sess.SessionID)
	}
	if sess.ProjectHash != "ph1" {
		t.Errorf("expected ProjectHash=ph1, got %q", sess.ProjectHash)
	}
	if len(sess.Waves) != 1 {
		t.Fatalf("expected 1 wave, got %d", len(sess.Waves))
	}
	if len(sess.Waves[0].Agents) != 2 {
		t.Fatalf("expected 2 agents in wave, got %d", len(sess.Waves[0].Agents))
	}
	if sess.TotalAgents != 2 {
		t.Errorf("expected TotalAgents=2, got %d", sess.TotalAgents)
	}
	// Verify agents sorted by letter
	if sess.Waves[0].Agents[0].Agent != "A" {
		t.Errorf("expected first agent=A, got %q", sess.Waves[0].Agents[0].Agent)
	}
	if sess.Waves[0].Agents[1].Agent != "B" {
		t.Errorf("expected second agent=B, got %q", sess.Waves[0].Agents[1].Agent)
	}
	// Verify wave timing
	if !sess.Waves[0].StartedAt.Equal(t0) {
		t.Errorf("expected StartedAt=%v, got %v", t0, sess.Waves[0].StartedAt)
	}
	if !sess.Waves[0].EndedAt.Equal(t1) {
		t.Errorf("expected EndedAt=%v, got %v", t1, sess.Waves[0].EndedAt)
	}
}

func TestComputeSAWWaves_MultiWave(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 10, 10, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 1, 10, 15, 0, 0, time.UTC)
	spans := []AgentSpan{
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave1:agent-A] wave1 task a", LaunchedAt: t0, CompletedAt: t1, Success: true},
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave1:agent-B] wave1 task b", LaunchedAt: t0, CompletedAt: t1, Success: true},
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave2:agent-A] wave2 task a", LaunchedAt: t2, CompletedAt: t3, Success: true},
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave2:agent-B] wave2 task b", LaunchedAt: t2, CompletedAt: t3, Success: false},
	}
	sessions := ComputeSAWWaves(spans)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	sess := sessions[0]
	if len(sess.Waves) != 2 {
		t.Fatalf("expected 2 waves, got %d", len(sess.Waves))
	}
	if sess.TotalAgents != 4 {
		t.Errorf("expected TotalAgents=4, got %d", sess.TotalAgents)
	}
	// Waves sorted by wave number
	if sess.Waves[0].Wave != 1 {
		t.Errorf("expected first wave=1, got %d", sess.Waves[0].Wave)
	}
	if sess.Waves[1].Wave != 2 {
		t.Errorf("expected second wave=2, got %d", sess.Waves[1].Wave)
	}
	// Verify statuses
	w2agents := sess.Waves[1].Agents
	// agent-B in wave2 failed
	var agentBStatus string
	for _, a := range w2agents {
		if a.Agent == "B" {
			agentBStatus = a.Status
		}
	}
	if agentBStatus != "failed" {
		t.Errorf("expected agent-B in wave2 to have status=failed, got %q", agentBStatus)
	}
}

func TestComputeSAWWaves_SkipsUntagged(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	spans := []AgentSpan{
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave1:agent-A] task a", LaunchedAt: t0, CompletedAt: t1, Success: true},
		{SessionID: "s1", ProjectHash: "ph1", Description: "untagged task", LaunchedAt: t0, CompletedAt: t1, Success: true},
		{SessionID: "s1", ProjectHash: "ph1", Description: "another untagged", LaunchedAt: t0, CompletedAt: t1, Success: false},
	}
	sessions := ComputeSAWWaves(spans)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions[0].Waves) != 1 {
		t.Fatalf("expected 1 wave, got %d", len(sessions[0].Waves))
	}
	if len(sessions[0].Waves[0].Agents) != 1 {
		t.Errorf("expected 1 agent (untagged skipped), got %d", len(sessions[0].Waves[0].Agents))
	}
	if sessions[0].TotalAgents != 1 {
		t.Errorf("expected TotalAgents=1, got %d", sessions[0].TotalAgents)
	}
}

func TestComputeSAWWaves_MultiSession(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	spans := []AgentSpan{
		{SessionID: "s1", ProjectHash: "ph1", Description: "[SAW:wave1:agent-A] session1 task", LaunchedAt: t0, CompletedAt: t1, Success: true},
		{SessionID: "s2", ProjectHash: "ph2", Description: "[SAW:wave1:agent-A] session2 task", LaunchedAt: t0, CompletedAt: t1, Success: true},
	}
	sessions := ComputeSAWWaves(spans)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	// Sessions sorted by SessionID
	if sessions[0].SessionID != "s1" {
		t.Errorf("expected first session=s1, got %q", sessions[0].SessionID)
	}
	if sessions[1].SessionID != "s2" {
		t.Errorf("expected second session=s2, got %q", sessions[1].SessionID)
	}
	if sessions[0].ProjectHash != "ph1" {
		t.Errorf("expected s1 ProjectHash=ph1, got %q", sessions[0].ProjectHash)
	}
	if sessions[1].ProjectHash != "ph2" {
		t.Errorf("expected s2 ProjectHash=ph2, got %q", sessions[1].ProjectHash)
	}
	if sessions[0].TotalAgents != 1 {
		t.Errorf("expected s1 TotalAgents=1, got %d", sessions[0].TotalAgents)
	}
	if sessions[1].TotalAgents != 1 {
		t.Errorf("expected s2 TotalAgents=1, got %d", sessions[1].TotalAgents)
	}
}
