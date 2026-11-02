package dynamic

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func usageTierSelectionJSON(t *testing.T, mode string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(orderedSelection{Tier: &struct {
		Mode      string `json:"mode"`
		OnFailure string `json:"on_failure"`
	}{Mode: mode, OnFailure: orderedSameTier}})
	if err != nil {
		t.Fatalf("marshal tier selection: %v", err)
	}
	return raw
}

func joinedTierSelectionJSON(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(orderedSelection{JoinPrevious: true})
	if err != nil {
		t.Fatalf("marshal joined selection: %v", err)
	}
	return raw
}

func newInfoObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, observed := observer.New(zapcore.InfoLevel)
	return zap.New(core), observed
}

func usageLogCandidate(now time.Time, id string, remaining, rate float64) Candidate {
	return usageCandidateAt(now, id, CandidateUsage{
		Known: true, RemainingPct: remaining,
		ConsumptionRateKnown: true, ConsumptionPctPerHour: rate,
		ResetAtKnown: true, ResetAt: now.Add(time.Hour),
	})
}

func TestUsageTierSelectionLogsPerCandidateFacts(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	log, observed := newInfoObservedLogger()
	engine := NewEngine(
		WithClock(func() time.Time { return now }),
		WithOrderedTiers(true),
		WithEngineLogger(log),
	)
	profile := Profile{ID: "dynamic-log", Version: 1, Candidates: []Candidate{
		usageLogCandidate(now, "candidate-a", 30, 2),
		usageLogCandidate(now, "candidate-b", 70, 2),
	}}
	profile.Candidates[0].SelectionJSON = usageTierSelectionJSON(t, tierModePace)
	profile.Candidates[1].SelectionJSON = joinedTierSelectionJSON(t)

	decision, err := engine.Select("session-log", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if decision.ExecutionProfileID != "candidate-b" {
		t.Fatalf("selected %q, want the candidate with more runway", decision.ExecutionProfileID)
	}
	entries := observed.FilterMessage(tierSelectionLogMessage).All()
	if len(entries) != 1 {
		t.Fatalf("selection log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["session_id"] != "session-log" || fields["logical_profile_id"] != "dynamic-log" {
		t.Fatalf("identity fields = %#v", fields)
	}
	if fields["generation"] != int64(1) || fields["reason"] != "candidate_order" {
		t.Fatalf("decision fields = %#v", fields)
	}
	if fields["selected_candidate_id"] != "candidate-b" {
		t.Fatalf("selected_candidate_id = %v, want candidate-b", fields["selected_candidate_id"])
	}
	rows, ok := fields["candidates"].([]tierSelectionLogRow)
	if !ok || len(rows) != 2 {
		t.Fatalf("candidates field = %#v, want two rows", fields["candidates"])
	}
	if rows[0].CandidateID != "candidate-b" || rows[0].Mode != tierModePace || !rows[0].Eligible {
		t.Fatalf("first row = %#v", rows[0])
	}
	if !rows[0].UsageKnown || rows[0].RemainingPct == nil || !rows[0].RateKnown || rows[0].ResetAt == nil || rows[0].PaceScore == nil {
		t.Fatalf("first row lost its usage facts: %#v", rows[0])
	}
	if rows[1].CandidateID != "candidate-a" || rows[1].PaceScore == nil || *rows[1].PaceScore >= *rows[0].PaceScore {
		t.Fatalf("second row = %#v, want a lower pace score than the first", rows[1])
	}
}

func TestOrderTierSelectionLogsNothing(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	log, observed := newInfoObservedLogger()
	engine := NewEngine(
		WithClock(func() time.Time { return now }),
		WithOrderedTiers(true),
		WithEngineLogger(log),
	)
	profile := Profile{ID: "dynamic-order", Version: 1, Candidates: []Candidate{
		usageLogCandidate(now, "candidate-a", 30, 2),
		usageLogCandidate(now, "candidate-b", 70, 2),
	}}
	if _, err := engine.Select("session-order", profile, 0, ""); err != nil {
		t.Fatalf("Select: %v", err)
	}
	if entries := observed.FilterMessage(tierSelectionLogMessage).All(); len(entries) != 0 {
		t.Fatalf("order profile emitted %d selection log entries, want 0", len(entries))
	}
}
