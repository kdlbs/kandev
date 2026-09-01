package github

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestCIRunGrantServiceRejectsNonAdminForCreateListAndRevoke(t *testing.T) {
	service, _, input := setupCIRunServiceTest(t, false)
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1", Role: authn.RoleMember})
	grantInput := CreateCIRunGrantInput{
		WorkspaceID: "workspace-1", ActorTaskID: input.ActorTaskID, TargetTaskID: input.TargetTaskID,
		WorkflowID: "workflow-1", WorkflowStepID: input.ExpectedWorkflowStepID, RepositoryID: input.RepositoryID,
	}
	if _, err := service.CreateCIRunGrant(ctx, "member-1", grantInput); !isCIRunGrantAdminDenial(err) {
		t.Fatalf("CreateCIRunGrant() error = %v, want admin denial", err)
	}
	if _, err := service.ListCIRunGrants(ctx, "member-1", "workspace-1"); !isCIRunGrantAdminDenial(err) {
		t.Fatalf("ListCIRunGrants() error = %v, want admin denial", err)
	}
	if err := service.RevokeCIRunGrant(ctx, "member-1", "workspace-1", "grant-1"); !isCIRunGrantAdminDenial(err) {
		t.Fatalf("RevokeCIRunGrant() error = %v, want admin denial", err)
	}
}

func isCIRunGrantAdminDenial(err error) bool {
	requestErr, ok := err.(*CIRunRequestError)
	return ok && requestErr.Class == CIRunFailureNotAuthorized
}

func TestCreateCIRunGrantReplacementRollsBackRevocationWhenInsertFails(t *testing.T) {
	service, _, input := setupCIRunServiceTest(t, false)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
	before, err := service.store.GetActiveCIRunGrant(
		ctx, "workspace-1", input.ActorTaskID, input.TargetTaskID,
		"workflow-1", input.ExpectedWorkflowStepID, input.RepositoryID,
	)
	if err != nil || before == nil {
		t.Fatalf("load initial grant: %+v, %v", before, err)
	}
	if _, err := service.store.db.Exec(`CREATE TRIGGER fail_ci_grant_insert
		BEFORE INSERT ON github_ci_run_grants
		BEGIN SELECT RAISE(FAIL, 'forced grant insert failure'); END`); err != nil {
		t.Fatal(err)
	}

	_, err = service.CreateCIRunGrant(ctx, "owner-1", CreateCIRunGrantInput{
		WorkspaceID: "workspace-1", ActorTaskID: input.ActorTaskID, TargetTaskID: input.TargetTaskID,
		WorkflowID: "workflow-1", WorkflowStepID: input.ExpectedWorkflowStepID,
		RepositoryID: input.RepositoryID,
	})
	if err == nil {
		t.Fatal("replacement succeeded despite forced insert failure")
	}
	active, lookupErr := service.store.GetActiveCIRunGrant(
		ctx, "workspace-1", input.ActorTaskID, input.TargetTaskID,
		"workflow-1", input.ExpectedWorkflowStepID, input.RepositoryID,
	)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	if active == nil || active.ID != before.ID || active.Generation != before.Generation {
		t.Fatalf("active grant after failed replacement = %+v, want %+v", active, before)
	}
}

func TestCreateCIRunGrantConcurrentReplacementsRemainMonotonic(t *testing.T) {
	service, _, input := setupCIRunServiceTest(t, false)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
	before, err := service.store.GetActiveCIRunGrant(
		ctx, "workspace-1", input.ActorTaskID, input.TargetTaskID,
		"workflow-1", input.ExpectedWorkflowStepID, input.RepositoryID,
	)
	if err != nil || before == nil {
		t.Fatalf("load initial grant: %+v, %v", before, err)
	}
	const replacements = 8
	start := make(chan struct{})
	errs := make(chan error, replacements)
	var wg sync.WaitGroup
	for index := range replacements {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.CreateCIRunGrant(ctx, fmt.Sprintf("owner-%d", index+2), CreateCIRunGrantInput{
				WorkspaceID: "workspace-1", ActorTaskID: input.ActorTaskID, TargetTaskID: input.TargetTaskID,
				WorkflowID: "workflow-1", WorkflowStepID: input.ExpectedWorkflowStepID,
				RepositoryID: input.RepositoryID,
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent replacement failed: %v", err)
		}
	}

	grants, err := service.store.ListCIRunGrants(ctx, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[int64]bool, len(grants))
	active := 0
	for _, grant := range grants {
		seen[grant.Generation] = true
		if grant.RevokedAt == nil {
			active++
		}
	}
	if len(grants) != replacements+1 || active != 1 {
		t.Fatalf("grants = %d active = %d, want %d and 1", len(grants), active, replacements+1)
	}
	for generation := before.Generation; generation <= before.Generation+replacements; generation++ {
		if !seen[generation] {
			t.Fatalf("missing grant generation %d in %+v", generation, seen)
		}
	}
}

func TestTerminalCIRunAuditFailureRollsBackRequestState(t *testing.T) {
	service, _, input := setupCIRunServiceTest(t, false)
	ctx := context.Background()
	if _, err := service.store.db.Exec(`CREATE TRIGGER fail_terminal_ci_audit
		BEFORE INSERT ON github_ci_run_audit_events
		WHEN NEW.event_type = 'failed'
		BEGIN SELECT RAISE(FAIL, 'forced terminal audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	input.EvidenceKind = CIRunEvidenceCurrentMerge
	_, err := service.RequestFreshCIRun(ctx, input)
	if err == nil {
		t.Fatal("request succeeded despite forced terminal audit failure")
	}
	var status CIRunRequestStatus
	if err := service.store.db.Get(&status, `SELECT status FROM github_ci_run_requests LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if status != CIRunRequestPending {
		t.Fatalf("request status after failed terminal audit = %q, want pending", status)
	}
}
