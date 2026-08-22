package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// AC-29: the automations YAML export reads automations, triggers, and
// repository links within a single transaction opened on the store's reader
// handle, so the export observes one consistent snapshot even though the
// same read spans several other stores' tables too. BeginReadTx opens that
// transaction; ListAutomationsForExportTx is ListAutomations' counterpart
// that reads within it instead of opening its own.

func TestBeginReadTx_ReadsCommittedData(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "Daily Review", Enabled: true, MaxConcurrentRuns: 1}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	tx, err := store.BeginReadTx(ctx)
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	automations, err := store.ListAutomationsForExportTx(ctx, tx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(automations))
	}
	if automations[0].ID != a.ID {
		t.Errorf("ID = %q, want %q", automations[0].ID, a.ID)
	}
}

func TestListAutomationsForExportTx_HydratesTriggersAndRepositoryIDs(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Daily Review",
		Enabled:           true,
		MaxConcurrentRuns: 1,
		RepositoryIDs:     []string{"repo-a", "repo-b"},
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	trigger := &AutomationTrigger{
		AutomationID: a.ID,
		Type:         "scheduled",
		Enabled:      true,
		Config:       json.RawMessage(`{"cron_expression":"0 9 * * *"}`),
	}
	if err := store.CreateTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}

	tx, err := store.BeginReadTx(ctx)
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	automations, err := store.ListAutomationsForExportTx(ctx, tx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(automations))
	}
	got := automations[0]
	if len(got.Triggers) != 1 || got.Triggers[0].ID != trigger.ID {
		t.Errorf("Triggers = %+v, want [%s]", got.Triggers, trigger.ID)
	}
	if len(got.RepositoryIDs) != 2 || got.RepositoryIDs[0] != "repo-a" || got.RepositoryIDs[1] != "repo-b" {
		t.Errorf("RepositoryIDs = %v, want [repo-a repo-b]", got.RepositoryIDs)
	}
}

// AC-8: repositories are ordered by automation_repositories.position
// ascending, tiebroken by automation_repositories.repository_id ascending.
// The schema's only uniqueness constraint is UNIQUE(automation_id,
// repository_id) — nothing prevents two different repositories on the same
// automation from sharing a position — so a real tie is reachable data, not
// a hypothetical. Insert the tied rows directly (insertAutomationRepositories
// always assigns sequential positions, so it cannot produce a tie) in
// repository_id-descending insertion order, deliberately opposite the
// expected output order, so a query lacking the tiebreak would only pass by
// accident of SQLite's unspecified tie order.
func TestListAutomationsForExportTx_AC8_RepositoryPositionTieBreaksByRepositoryID(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "Daily Review", Enabled: true, MaxConcurrentRuns: 1}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	now := time.Now().UTC()
	for _, repoID := range []string{"repo-z", "repo-m", "repo-a"} {
		_, err := store.db.ExecContext(ctx, `
			INSERT INTO automation_repositories (id, automation_id, repository_id, position, created_at)
			VALUES (?, ?, ?, 0, ?)`, "arow-"+repoID, a.ID, repoID, now)
		if err != nil {
			t.Fatalf("seed tied automation_repositories row %s: %v", repoID, err)
		}
	}

	tx, err := store.BeginReadTx(ctx)
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	automations, err := store.ListAutomationsForExportTx(ctx, tx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(automations))
	}
	want := []string{"repo-a", "repo-m", "repo-z"}
	got := automations[0].RepositoryIDs
	if len(got) != len(want) {
		t.Fatalf("RepositoryIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RepositoryIDs = %v, want %v (repository_id-ascending tiebreak on position tie)", got, want)
			break
		}
	}
}

func TestListAutomationsForExportTx_EmptyWorkspace(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	tx, err := store.BeginReadTx(ctx)
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	automations, err := store.ListAutomationsForExportTx(ctx, tx, "ws-empty")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx: %v", err)
	}
	if len(automations) != 0 {
		t.Errorf("expected 0 automations, got %d", len(automations))
	}
}
