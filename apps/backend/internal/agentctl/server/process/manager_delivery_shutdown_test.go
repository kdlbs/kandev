package process

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestManagerJournalClosesWhenTeardownDrainExpires(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:            t.TempDir(),
		DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
	}, newTestLogger(t))
	deliveryJournal, err := mgr.DeliveryJournal()
	if err != nil {
		t.Fatalf("DeliveryJournal() error = %v", err)
	}

	release, err := mgr.admitStart()
	if err != nil {
		t.Fatalf("admitStart() error = %v", err)
	}
	t.Cleanup(release)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.StopForTeardown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("StopForTeardown() error = %v, want %v", err, context.Canceled)
	}
	if _, _, err := deliveryJournal.Replay(context.Background(), "session", 0, 10); !errors.Is(err, journal.ErrJournalClosed) {
		t.Fatalf("late Replay() error = %v, want %v", err, journal.ErrJournalClosed)
	}
	if _, err := mgr.DeliveryJournal(); !errors.Is(err, journal.ErrJournalClosed) {
		t.Fatalf("DeliveryJournal() after teardown error = %v, want %v", err, journal.ErrJournalClosed)
	}

	release()
	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("StopForTeardown() retry error = %v", err)
	}
}

func TestPersistEventAfterJournalCloseFailsClosed(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt")}, newTestLogger(t))
	if err := mgr.closeDeliveryJournal(); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.persistDeliveryEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "late"}); !errors.Is(err, journal.ErrJournalClosed) {
		t.Fatalf("late event error = %v, want closed journal", err)
	}
}
