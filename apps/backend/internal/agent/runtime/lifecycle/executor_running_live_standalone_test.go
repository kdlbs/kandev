package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// listingWriter is a fake ExecutorRunningWriter that also implements the
// optional executorRunningLister capability, so a test can pin
// Manager.ListLiveStandaloneExecutorsRunning's delegation without a real DB.
type listingWriter struct {
	invariantWriter
	rows []*models.ExecutorRunning
	err  error
}

func (w *listingWriter) ListExecutorsRunningLiveStandalone(context.Context) ([]*models.ExecutorRunning, error) {
	return w.rows, w.err
}

// TestListLiveStandaloneExecutorsRunningDelegatesToWriter pins that the
// manager forwards to the writer's optional lister capability when present.
// This inventory is read at startup step 3, before any control-server
// contact, to build the recovery guard/correlation set (discovery H).
func TestListLiveStandaloneExecutorsRunningDelegatesToWriter(t *testing.T) {
	mgr := newTestManager(t)
	want := []*models.ExecutorRunning{{SessionID: "session-1"}}
	mgr.SetExecutorRunningWriter(&listingWriter{rows: want})

	got, err := mgr.ListLiveStandaloneExecutorsRunning(context.Background())
	if err != nil {
		t.Fatalf("ListLiveStandaloneExecutorsRunning: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "session-1" {
		t.Fatalf("got = %#v, want %#v", got, want)
	}
}

// TestListLiveStandaloneExecutorsRunningWithoutListerReturnsEmpty pins the
// best-effort fallback: a writer that doesn't support listing (e.g. a test
// double) yields no candidates rather than an error, matching the rest of
// this file's optional-interface pattern.
func TestListLiveStandaloneExecutorsRunningWithoutListerReturnsEmpty(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(&invariantWriter{})

	got, err := mgr.ListLiveStandaloneExecutorsRunning(context.Background())
	if err != nil {
		t.Fatalf("ListLiveStandaloneExecutorsRunning: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %#v, want empty", got)
	}
}
