package process

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func pressureTestManager(t *testing.T) (*Manager, *journal.Journal) {
	t.Helper()
	m := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session", DurableJournalPath: filepath.Join(t.TempDir(), "original")}, newTestLogger(t))
	if err := m.deliveryJournal.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "small"), MaxEventBytes: 2048, MaxStreamBytes: 4096, MaxJournalBytes: 16384, ReserveBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	m.deliveryJournal = j
	m.status.Store(StatusRunning)
	t.Cleanup(func() {
		m.CloseAdmission()
		if err := m.closeDeliveryJournal(); err != nil {
			t.Error(err)
		}
	})
	return m, j
}

func fillPressureJournal(t *testing.T, j *journal.Journal) uint64 {
	t.Helper()
	var last uint64
	for {
		e, err := j.Append(context.Background(), journal.Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session", Type: adapter.EventTypeMessageChunk, Payload: make([]byte, 400)})
		if errors.Is(err, journal.ErrStreamFull) {
			return last
		}
		if err != nil {
			t.Fatal(err)
		}
		last = e.Sequence
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.5
func TestFullJournalRemainsReadableAndPendingWriteResumesAfterACK(t *testing.T) {
	m, j := pressureTestManager(t)
	last := fillPressureJournal(t, j)
	done := make(chan error, 1)
	go func() {
		_, err := m.persistDeliveryEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: strings.Repeat("x", 400)})
		done <- err
	}()
	waitForDeliveryGuard(t, m)
	encoded, err := json.Marshal(m.DeliveryHealth())
	if err != nil {
		t.Fatal(err)
	}
	var health map[string]any
	if err := json.Unmarshal(encoded, &health); err != nil {
		t.Fatal(err)
	}
	if health["producer_paused"] != true {
		t.Fatalf("actual waiting producer not reported: %s", encoded)
	}
	select {
	case err := <-done:
		t.Fatalf("write returned before ACK: %v", err)
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := m.DeliveryJournal(); err != nil {
		t.Fatalf("full journal inaccessible: %v", err)
	}
	if _, _, err := j.Replay(ctx, "session", 0, 100); err != nil {
		t.Fatal(err)
	}
	if m.Status() != StatusRunning {
		t.Fatal("delivery pressure changed process liveness")
	}
	if err := j.Acknowledge(ctx, "session", last); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("pending write did not resume")
	}
	if m.DeliveryHealth().ProducerPaused {
		t.Fatal("producer remained paused after committed output resumed")
	}
	events, _, err := j.Replay(ctx, "session", last, 10)
	if err != nil || len(events) != 1 || events[0].Sequence != last+1 {
		t.Fatalf("retained pending event = %+v, %v", events, err)
	}
}

func TestFullJournalWriterClosesWithoutWaitingForACK(t *testing.T) {
	m, j := pressureTestManager(t)
	fillPressureJournal(t, j)
	writer := newDeliveryEventWriter(m)
	done := make(chan error, 1)
	go func() {
		_, err := writer.persist(context.Background(), adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: strings.Repeat("x", 400)})
		done <- err
	}()
	waitForDeliveryGuard(t, m)
	writer.close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed writer accepted event")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("writer waited for ACK during teardown")
	}
	if _, err := m.DeliveryJournal(); err != nil {
		t.Fatal("writer close invalidated retained journal")
	}
}

func TestLatePressureEventCannotCancelSuccessor(t *testing.T) {
	m, j := pressureTestManager(t)
	fillPressureJournal(t, j)
	m.deliveryActiveID = "successor"
	if err := m.refreshDeliveryPressure(context.Background(), j, "predecessor"); err != nil {
		t.Fatal(err)
	}
	if m.DeliveryHealth().CancellationPending {
		t.Fatal("late event attempted successor cancellation")
	}
	if !m.deliveryBlocked {
		t.Fatal("pressure did not fence admission")
	}
}

func waitForDeliveryGuard(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		m.deliveryActiveMu.RLock()
		guarded := m.deliveryHealth.Capacity.Guarded
		m.deliveryActiveMu.RUnlock()
		if guarded {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("writer did not reach capacity guard")
		case <-tick.C:
		}
	}
}

type pressureCancelAdapter struct {
	*stubAdapter
	cancelled   chan struct{}
	cancelError error
}

func (a *pressureCancelAdapter) Cancel(context.Context) error {
	a.cancelled <- struct{}{}
	return a.cancelError
}

func TestDeliveryPressureCancelsOnlyMatchingOwnerAndKeepsFailedCancelBlocked(t *testing.T) {
	m, j := pressureTestManager(t)
	adpt := &pressureCancelAdapter{stubAdapter: &stubAdapter{}, cancelled: make(chan struct{}, 1), cancelError: errors.New("cancel unconfirmed")}
	m.adapter = adpt
	ctx := context.Background()
	if _, err := m.AdmitDeliverySubmission(ctx, journal.Submission{ID: "active", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, Hash: "hash", Payload: []byte("prompt")}); err != nil {
		t.Fatal(err)
	}
	last := fillPressureJournal(t, j)
	m.deliveryActiveID = "active"
	if err := m.refreshDeliveryPressure(ctx, j, "predecessor"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-adpt.cancelled:
		t.Fatal("late event cancelled active owner")
	default:
	}
	if err := m.refreshDeliveryPressure(ctx, j, "active"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-adpt.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("current owner was not cancelled")
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for m.DeliveryHealth().CancellationPending {
		select {
		case <-deadline.C:
			t.Fatal("cancel result did not settle")
		case <-tick.C:
		}
	}
	if err := j.Acknowledge(ctx, "session", last); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AdmitDeliverySubmission(ctx, journal.Submission{ID: "next", Hash: "next"}); !errors.Is(err, errDeliveryPressure) {
		t.Fatalf("unconfirmed cancellation admitted prompt: %v", err)
	}
	if m.Status() != StatusRunning {
		t.Fatal("failed cancellation reported process death")
	}
	retained, err := j.GetSubmission(ctx, "active")
	if err != nil {
		t.Fatal(err)
	}
	if retained.TerminalEventRetained {
		t.Fatal("failed cancellation fabricated a terminal")
	}
}

func TestOversizedOutputFailsWithoutPoisoningJournal(t *testing.T) {
	m, j := pressureTestManager(t)
	_, err := m.persistDeliveryEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: strings.Repeat("x", 4096)})
	if !errors.Is(err, journal.ErrStreamFull) {
		t.Fatalf("oversized event error = %v", err)
	}
	if got, err := m.DeliveryJournal(); err != nil || got != j {
		t.Fatalf("oversized event poisoned journal: %v", err)
	}
	if m.Status() != StatusRunning {
		t.Fatal("oversized output reported process death")
	}
}
