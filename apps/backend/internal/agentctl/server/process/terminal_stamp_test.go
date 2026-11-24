package process

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func newStampManager(t *testing.T) (*Manager, *journal.Journal) {
	t.Helper()
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	deliveryJournal, err := journal.Open(journal.Config{Path: journalPath})
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	m := &Manager{
		cfg: &config.InstanceConfig{
			DurableJournalPath: journalPath, SessionID: "session-1",
			DeliveryStreamID: "stream-1", DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 1,
		},
		logger:          newTestLogger(t),
		updatesCh:       make(chan adapter.AgentEvent, 4),
		deliveryJournal: deliveryJournal,
	}
	t.Cleanup(func() { _ = m.closeDeliveryJournal() })
	return m, deliveryJournal
}

func admitSubmission(t *testing.T, m *Manager, id string) {
	t.Helper()
	sub := journal.Submission{ID: id, SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 1, Hash: "h-" + id, Payload: []byte("prompt")}
	if _, err := m.AdmitDeliverySubmission(context.Background(), sub); err != nil {
		t.Fatalf("admit: %v", err)
	}
}

func committedTerminal(t *testing.T, j *journal.Journal) adapter.AgentEvent {
	t.Helper()
	events, _, err := j.Replay(context.Background(), "stream-1", 0, 10)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	var terminal adapter.AgentEvent
	if err := json.Unmarshal(events[len(events)-1].Payload, &terminal); err != nil {
		t.Fatalf("decode terminal: %v", err)
	}
	return terminal
}

func TestTerminalCommitStampsDetachedFieldsAndCommitTime(t *testing.T) {
	m, j := newStampManager(t)
	before := time.Now().Add(-time.Second)
	if _, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{{Type: adapter.EventTypeComplete}}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	terminal := committedTerminal(t, j)
	if !terminal.DetachedAtCommit || terminal.CommittedAt == nil || terminal.CommittedAt.Before(before) {
		t.Fatalf("terminal = detached %t committed_at %v, want detached with a commit time", terminal.DetachedAtCommit, terminal.CommittedAt)
	}
	if terminal.CancelCause != "" {
		t.Fatalf("cancel cause = %q, want none", terminal.CancelCause)
	}
}

func TestTerminalCommitWhileAttachedIsNotDetached(t *testing.T) {
	m, j := newStampManager(t)
	m.AttachStream("s1")
	if _, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{{Type: adapter.EventTypeComplete}}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if committedTerminal(t, j).DetachedAtCommit {
		t.Fatal("terminal committed while attached stamped detached")
	}
}

func TestTerminalOfBudgetCancelledTurnIsForcedToCancelledWithCause(t *testing.T) {
	m, j := newStampManager(t)
	admitSubmission(t, m, "sub-1")
	decided := time.Now().Add(-time.Minute).Truncate(time.Second)
	m.attach.budget.record = &budgetCancelRecord{submissionID: "sub-1", state: budgetCancelIssued, decidedAt: decided}
	data := map[string]any{"stop_reason": "end_turn"}
	update := adapter.AgentEvent{Type: adapter.EventTypeComplete, DeliverySubmissionID: "sub-1", Data: data}
	if _, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{update}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	terminal := committedTerminal(t, j)
	if terminal.Data["stop_reason"] != "cancelled" || terminal.CancelCause != "offline_budget" {
		t.Fatalf("terminal data %v cause %q, want cancelled + offline_budget", terminal.Data, terminal.CancelCause)
	}
	if terminal.CancelDecidedAt == nil || !terminal.CancelDecidedAt.Equal(decided) {
		t.Fatalf("cancel_decided_at = %v, want %v", terminal.CancelDecidedAt, decided)
	}
	if data["stop_reason"] != "end_turn" {
		t.Fatal("stamp mutated the caller's data map")
	}
	if m.attach.budget.record != nil {
		t.Fatal("record survived its own terminal")
	}
}

func TestErrorTerminalKeepsItsErrorAndGetsOnlyDetachedFields(t *testing.T) {
	m, j := newStampManager(t)
	admitSubmission(t, m, "sub-1")
	m.attach.budget.record = &budgetCancelRecord{submissionID: "sub-1", state: budgetCancelPending}
	update := adapter.AgentEvent{Type: adapter.EventTypeError, Error: "boom", DeliverySubmissionID: "sub-1"}
	if _, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{update}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	terminal := committedTerminal(t, j)
	if terminal.Error != "boom" || terminal.CancelCause != "" || !terminal.DetachedAtCommit {
		t.Fatalf("error terminal = %+v", terminal)
	}
}

func TestNonTerminalEventsAreNotStamped(t *testing.T) {
	m, j := newStampManager(t)
	if _, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{{Type: adapter.EventTypeMessageChunk, Text: "x"}}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	events, _, _ := j.Replay(context.Background(), "stream-1", 0, 10)
	var got adapter.AgentEvent
	_ = json.Unmarshal(events[0].Payload, &got)
	if got.CommittedAt != nil || got.DetachedAtCommit {
		t.Fatalf("message chunk stamped: %+v", got)
	}
}

func TestNoJournalInstanceStampsNothing(t *testing.T) {
	m := &Manager{cfg: &config.InstanceConfig{}, logger: newTestLogger(t)}
	out, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{{Type: adapter.EventTypeComplete}})
	if err != nil || out[0].CommittedAt != nil {
		t.Fatalf("no-journal persist = %+v, err %v; want unstamped", out, err)
	}
}

func TestDispatchSetsInFlightMarkerOnlyAroundTheHarnessCall(t *testing.T) {
	m, _ := newStampManager(t)
	admitSubmission(t, m, "sub-1")
	var during string
	_, err := m.DispatchDeliverySubmission(context.Background(), "sub-1", func(context.Context) error {
		m.attach.mu.Lock()
		during = m.attach.budget.inFlight
		m.attach.mu.Unlock()
		return errors.New("harness failed")
	})
	if err == nil {
		t.Fatal("dispatch error swallowed")
	}
	if during != "sub-1" {
		t.Fatalf("marker during harness call = %q, want sub-1", during)
	}
	if m.attach.budget.inFlight != "" {
		t.Fatal("marker survived a failed dispatch")
	}
}

func TestReadmittedSettledSubmissionNeverSetsMarker(t *testing.T) {
	m, _ := newStampManager(t)
	admitSubmission(t, m, "sub-1")
	if _, err := m.DispatchDeliverySubmission(context.Background(), "sub-1", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if _, err := m.persistDeliveryBatchRaw(context.Background(), []adapter.AgentEvent{{Type: adapter.EventTypeComplete, DeliverySubmissionID: "sub-1"}}); err != nil {
		t.Fatalf("persist terminal: %v", err)
	}
	called := false
	_, _ = m.DispatchDeliverySubmission(context.Background(), "sub-1", func(context.Context) error { called = true; return nil })
	if called || m.attach.budget.inFlight != "" {
		t.Fatalf("settled submission dispatched=%t marker=%q", called, m.attach.budget.inFlight)
	}
}
