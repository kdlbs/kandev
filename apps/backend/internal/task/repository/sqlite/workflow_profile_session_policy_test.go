package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkflowProfileSessionPolicyColumnExists(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)

	rows, err := repo.db.Query("PRAGMA table_info(workflows)")
	if err != nil {
		t.Fatalf("inspect workflows schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var found bool
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan workflows schema: %v", err)
		}
		if name == "profile_session_policy" {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate workflows schema: %v", err)
	}
	if !found {
		t.Fatal("workflows.profile_session_policy column is missing")
	}
}

func TestWorkflowProfileSessionPolicyRoundTripAndDefaults(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)
	ctx := t.Context()

	for _, policy := range []models.WorkflowProfileSessionPolicy{
		models.WorkflowProfileSessionPolicyComplete,
		models.WorkflowProfileSessionPolicyParkReuse,
		models.WorkflowProfileSessionPolicyParkNew,
	} {
		workflow := &models.Workflow{
			ID:                   "workflow-" + string(policy),
			WorkspaceID:          "ws-policy",
			Name:                 string(policy),
			ProfileSessionPolicy: policy,
		}
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("create %q: %v", policy, err)
		}
		got, err := repo.GetWorkflow(ctx, workflow.ID)
		if err != nil {
			t.Fatalf("get %q: %v", policy, err)
		}
		if got.ProfileSessionPolicy != policy {
			t.Fatalf("get %q policy = %q, want %q", policy, got.ProfileSessionPolicy, policy)
		}
	}

	unknown := &models.Workflow{
		ID:                   "workflow-unknown",
		WorkspaceID:          "ws-policy",
		Name:                 "unknown",
		ProfileSessionPolicy: "unsupported",
	}
	if err := repo.CreateWorkflow(ctx, unknown); err != nil {
		t.Fatalf("create unknown: %v", err)
	}
	got, err := repo.GetWorkflow(ctx, unknown.ID)
	if err != nil {
		t.Fatalf("get unknown: %v", err)
	}
	if got.ProfileSessionPolicy != models.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("unknown policy = %q, want complete", got.ProfileSessionPolicy)
	}

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		"UPDATE workflows SET profile_session_policy = ? WHERE id = ?",
	), "corrupt", unknown.ID); err != nil {
		t.Fatalf("seed invalid stored policy: %v", err)
	}
	got, err = repo.GetWorkflow(ctx, unknown.ID)
	if err != nil {
		t.Fatalf("get invalid stored policy: %v", err)
	}
	if got.ProfileSessionPolicy != models.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("invalid stored policy = %q, want complete", got.ProfileSessionPolicy)
	}

	got.ProfileSessionPolicy = models.WorkflowProfileSessionPolicyParkReuse
	if err := repo.UpdateWorkflow(ctx, got); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	listed, err := repo.ListWorkflows(ctx, "ws-policy", false)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	for _, workflow := range listed {
		if workflow.ID == unknown.ID && workflow.ProfileSessionPolicy != models.WorkflowProfileSessionPolicyParkReuse {
			t.Fatalf("listed policy = %q, want %q", workflow.ProfileSessionPolicy, models.WorkflowProfileSessionPolicyParkReuse)
		}
	}
}

func TestWorkflowProfileSessionPolicyMigrationReplays(t *testing.T) {
	repo := newRepoForWorkflowStyleTests(t)
	if err := repo.initSchema(); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	if err := repo.initSchema(); err != nil {
		t.Fatalf("replay schema twice: %v", err)
	}

	workflow := &models.Workflow{ID: "workflow-replay", WorkspaceID: "ws-replay", Name: "Replay"}
	if err := repo.CreateWorkflow(t.Context(), workflow); err != nil {
		t.Fatalf("create after replay: %v", err)
	}
	if workflow.ProfileSessionPolicy != models.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("created policy = %q, want %q", workflow.ProfileSessionPolicy, models.WorkflowProfileSessionPolicyComplete)
	}
}
