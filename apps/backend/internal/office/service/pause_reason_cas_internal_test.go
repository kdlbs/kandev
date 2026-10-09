package service

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newClearAutoPauseTestRepo opens a minimal in-memory repo: clearAutoPause
// only reaches agent_instances, so this skips the tasks/workflow schema
// newTestService (base_test.go, service_test package) sets up.
func newClearAutoPauseTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

// TestClearAutoPause_AbortsWhenNewerAutoPauseLands pins the retry-abort
// guard added alongside the CAS unpause write (AC-OFFICE-RUNTIME-001.9):
// a clearAutoPause call holding a stale pause_reason snapshot must not
// retry against a newer, unobserved auto-pause reason it finds on reload.
//
// This drives clearAutoPause directly with a hand-built stale snapshot
// instead of racing goroutines against the exported MarkAgentPausedFixed
// entry point, which always re-reads the agent fresh and so cannot
// reproduce the read-then-CAS window from the outside.
func TestClearAutoPause_AbortsWhenNewerAutoPauseLands(t *testing.T) {
	ctx := context.Background()
	repo := newClearAutoPauseTestRepo(t)
	svc := &Service{repo: repo}

	agent := &models.AgentInstance{
		ID:          "agent-cas-abort",
		WorkspaceID: "ws-1",
		Name:        "cas-abort",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := repo.UpdateAgentStatusFields(ctx, agent.ID, "paused", "Auto-paused: reason1"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	// The snapshot a caller (MarkAgentPausedFixed) would have read before a
	// second, unrelated auto-pause lands underneath it.
	stale := &models.AgentInstance{
		ID:          agent.ID,
		WorkspaceID: agent.WorkspaceID,
		Status:      models.AgentStatusPaused,
		PauseReason: "Auto-paused: reason1",
	}
	if err := repo.UpdateAgentStatusFields(ctx, agent.ID, "paused", "Auto-paused: reason2"); err != nil {
		t.Fatalf("re-pause agent: %v", err)
	}

	if err := svc.clearAutoPause(ctx, stale); err == nil {
		t.Fatal("clearAutoPause = nil, want an error aborting the stale clear")
	}

	got, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Status != models.AgentStatusPaused || got.PauseReason != "Auto-paused: reason2" {
		t.Fatalf("status/reason = %q/%q, want paused/%q (newer pause left intact by the abort)",
			got.Status, got.PauseReason, "Auto-paused: reason2")
	}
}

// TestClearAutoPause_SucceedsWhenPauseReasonStillMatches is the control:
// the same stale-snapshot shape but with the observed reason still current
// must clear normally, so the abort branch above is proven to be reason-
// specific rather than a general clearAutoPause regression.
func TestClearAutoPause_SucceedsWhenPauseReasonStillMatches(t *testing.T) {
	ctx := context.Background()
	repo := newClearAutoPauseTestRepo(t)
	svc := &Service{repo: repo}

	agent := &models.AgentInstance{
		ID:          "agent-cas-match",
		WorkspaceID: "ws-1",
		Name:        "cas-match",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := repo.UpdateAgentStatusFields(ctx, agent.ID, "paused", "Auto-paused: reason1"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	snapshot := &models.AgentInstance{
		ID:          agent.ID,
		WorkspaceID: agent.WorkspaceID,
		Status:      models.AgentStatusPaused,
		PauseReason: "Auto-paused: reason1",
	}

	if err := svc.clearAutoPause(ctx, snapshot); err != nil {
		t.Fatalf("clearAutoPause: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Status != models.AgentStatusIdle || got.PauseReason != "" {
		t.Fatalf("status/reason = %q/%q, want idle/\"\" (matching reason clears)",
			got.Status, got.PauseReason)
	}
}
