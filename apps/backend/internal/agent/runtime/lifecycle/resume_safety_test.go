package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// invariantWriter is a fake ExecutorRunningWriter + executorRunningReader that
// records whether a cleanup DELETED vs REPAIRED the row, so a test can pin the
// resume-safety deletion invariant (#1597 resume-safety invariant).
type invariantWriter struct {
	prior    *models.ExecutorRunning
	deleted  bool
	repaired bool
}

func (w *invariantWriter) UpsertExecutorRunning(_ context.Context, running *models.ExecutorRunning) error {
	w.prior = running
	return nil
}

func (w *invariantWriter) GetExecutorRunningBySessionID(_ context.Context, _ string) (*models.ExecutorRunning, error) {
	if w.prior == nil {
		return nil, models.ErrExecutorRunningNotFound
	}
	return w.prior, nil
}

func (w *invariantWriter) DeleteExecutorRunningBySessionID(_ context.Context, _ string) error {
	w.deleted = true
	w.prior = nil
	return nil
}

func (w *invariantWriter) RepairExecutorRunningDead(_ context.Context, _ string) error {
	w.repaired = true
	if w.prior != nil {
		w.prior.Status = models.ExecutorRunningStatusStopped
		w.prior.LocalPID = 0
	}
	return nil
}

// TestDeleteExecutorRunningRepairsResumableRow is the red-before-green
// characterization test for the on-demand stale-cleanup path
// (CleanupStaleExecutionBySessionID → deleteExecutorRunning). A row that still
// holds a resume_token must be repaired in place, never deleted — otherwise a
// stale-cleanup (or a resume whose relaunch fails) permanently costs the operator
// a resumable conversation (#1597 resume-safety invariant).
func TestDeleteExecutorRunningRepairsResumableRow(t *testing.T) {
	log := newNopLogger(t)
	writer := &invariantWriter{prior: &models.ExecutorRunning{
		SessionID:   "session-1",
		Runtime:     agentruntime.RuntimeStandalone,
		Status:      models.ExecutorRunningStatusRunning,
		ResumeToken: "resume-abc",
		LocalPID:    4242,
	}}
	m := &Manager{logger: log, runningWriter: writer}

	m.deleteExecutorRunning(context.Background(), "session-1")

	if writer.deleted {
		t.Fatal("deleteExecutorRunning deleted a row holding a resume_token; the resume-safety invariant requires repair-in-place")
	}
	if !writer.repaired {
		t.Fatal("deleteExecutorRunning did not repair the resumable row")
	}
	if writer.prior == nil || writer.prior.ResumeToken != "resume-abc" {
		t.Fatal("repair must preserve the resume_token")
	}
	if writer.prior.Status != models.ExecutorRunningStatusStopped || writer.prior.LocalPID != 0 {
		t.Fatalf("repair must mark the row stopped and clear the local liveness handle; got status=%q local_pid=%d",
			writer.prior.Status, writer.prior.LocalPID)
	}
}

// TestDeleteExecutorRunningPrunesNonResumableRow confirms the invariant does not
// over-preserve: a row with no resume_token is still deleted, so genuinely dead
// rows don't accumulate.
func TestDeleteExecutorRunningPrunesNonResumableRow(t *testing.T) {
	log := newNopLogger(t)
	writer := &invariantWriter{prior: &models.ExecutorRunning{
		SessionID: "session-2",
		Runtime:   agentruntime.RuntimeStandalone,
		Status:    models.ExecutorRunningStatusStopped,
	}}
	m := &Manager{logger: log, runningWriter: writer}

	m.deleteExecutorRunning(context.Background(), "session-2")

	if writer.repaired {
		t.Fatal("a row with no resume_token should be deleted, not repaired")
	}
	if !writer.deleted {
		t.Fatal("a row with no resume_token should be deleted")
	}
}
