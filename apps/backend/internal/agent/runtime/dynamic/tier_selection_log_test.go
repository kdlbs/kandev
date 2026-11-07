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
		WithTierUsageAvailable(true),
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
		WithTierUsageAvailable(true),
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

func TestPreviewUsesAnIndependentRandomPicker(t *testing.T) {
	executionCalls, previewCalls := 0, 0
	engine := NewEngine(
		WithOrderedTiers(true),
		WithTierRandomPick(func(int) int { executionCalls++; return 0 }),
		WithTierPreviewPick(func(int) int { previewCalls++; return 0 }),
	)
	random := usageTierSelectionJSON(t, tierModeRandom)
	profile := Profile{ID: "preview-pick", Version: 1, Candidates: []Candidate{
		{ID: "a", Enabled: true, SelectionJSON: random},
		{ID: "b", Enabled: true, SelectionJSON: joinedTierSelectionJSON(t)},
	}}
	if _, err := engine.PreviewTiers(profile); err != nil {
		t.Fatalf("PreviewTiers: %v", err)
	}
	if executionCalls != 0 {
		t.Fatalf("preview consumed the execution picker %d time(s)", executionCalls)
	}
	if previewCalls == 0 {
		t.Fatal("preview did not use its own picker")
	}
}

func TestOrderedStartIndexDoesNotReopenAnEarlierTier(t *testing.T) {
	tiers := []orderedTier{
		{head: "a", mode: orderedTierMode, direction: orderedSameTier, rows: []Candidate{{ID: "a"}}},
		{head: "b", mode: orderedTierMode, direction: orderedSameTier, rows: []Candidate{{ID: "b"}}},
		{head: "c", mode: orderedTierMode, direction: orderedSameTier, rows: []Candidate{{ID: "c"}}},
	}
	tiers[1].rows = []Candidate{{ID: "x"}}
	if got := orderedStartIndex(tiers, "b", true, []string{"b"}); got != len(tiers) {
		t.Fatalf("deleted current start = %d, want a fail-closed %d", got, len(tiers))
	}
	if got := orderedStartIndex(tiers, "b", true, []string{"a"}); got != 0 {
		t.Fatalf("existing tried start = %d, want 0", got)
	}
	tiers[0].direction = orderedNextTier
	if got := orderedStartIndex(tiers, "a", true, nil); got != 1 {
		t.Fatalf("next-tier start = %d, want 1", got)
	}
}
