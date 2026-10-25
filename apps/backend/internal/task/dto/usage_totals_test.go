package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestToTaskUsageTotalsDTO_ZeroUsage_NullTimestamps(t *testing.T) {
	totals := &models.TaskUsageTotals{OutputTokensComplete: true}

	got := ToTaskUsageTotalsDTO(TaskUsageTotalsScopeTask, "task-1", totals)
	if got.Scope != TaskUsageTotalsScopeTask || got.ScopeID != "task-1" {
		t.Fatalf("scope = (%q, %q), want (task, task-1)", got.Scope, got.ScopeID)
	}
	if got.EventCount != 0 || got.TokensTotal != 0 {
		t.Fatalf("got = %+v, want every sum zero", got)
	}
	if !got.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true")
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	firstEventAt, ok := decoded["first_event_at"]
	if !ok {
		t.Fatal("first_event_at must be present in the JSON body, not omitted")
	}
	if firstEventAt != nil {
		t.Errorf("first_event_at = %v, want JSON null", firstEventAt)
	}
	lastEventAt, ok := decoded["last_event_at"]
	if !ok {
		t.Fatal("last_event_at must be present in the JSON body, not omitted")
	}
	if lastEventAt != nil {
		t.Errorf("last_event_at = %v, want JSON null", lastEventAt)
	}
}

func TestToTaskUsageTotalsDTO_SessionScope_CopiesEveryField(t *testing.T) {
	first := time.Date(2026, 8, 23, 4, 0, 0, 0, time.UTC)
	last := first.Add(10 * time.Minute)
	totals := &models.TaskUsageTotals{
		TokensIn:             80,
		TokensCachedRead:     8203943,
		TokensCachedWrite:    264177,
		TokensOut:            44979,
		TokensThought:        0,
		TokensTotal:          8513179,
		CostSubcents:         79118,
		EventCount:           1,
		EstimatedEventCount:  0,
		UnpricedEventCount:   0,
		OutputTokensComplete: true,
		FirstEventAt:         &first,
		LastEventAt:          &last,
	}

	got := ToTaskUsageTotalsDTO(TaskUsageTotalsScopeSession, "sess-1", totals)
	if got.Scope != TaskUsageTotalsScopeSession || got.ScopeID != "sess-1" {
		t.Fatalf("scope = (%q, %q), want (session, sess-1)", got.Scope, got.ScopeID)
	}
	if got.TokensCachedRead != 8203943 || got.TokensTotal != 8513179 || got.CostSubcents != 79118 {
		t.Fatalf("got = %+v, want fields copied verbatim from totals", got)
	}
	if got.FirstEventAt == nil || !got.FirstEventAt.Equal(first) {
		t.Errorf("FirstEventAt = %v, want %v", got.FirstEventAt, first)
	}
	if got.LastEventAt == nil || !got.LastEventAt.Equal(last) {
		t.Errorf("LastEventAt = %v, want %v", got.LastEventAt, last)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["first_event_at"] != "2026-08-23T04:00:00Z" {
		t.Errorf("first_event_at = %v, want 2026-08-23T04:00:00Z", decoded["first_event_at"])
	}
}
