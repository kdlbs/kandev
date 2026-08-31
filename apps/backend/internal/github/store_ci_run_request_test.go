package github

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreScopedCIRunSchema(t *testing.T) {
	store := newTestStore(t)
	want := map[string][]string{
		"github_ci_run_grants": {
			"workspace_id", "actor_task_id", "target_task_id", "workflow_id",
			"workflow_step_id", "repository_id", "revoked_at",
		},
		"github_ci_run_requests": {
			"grant_id", "actor_task_id", "actor_session_id", "target_task_id",
			"expected_head_sha", "source_run_id", "expected_source_attempt",
			"idempotency_hash", "provider_call_started_at", "failure_class",
			"provider_workflow_name", "provider_workflow_path",
			"execution_owner", "execution_lease_expires_at", "provider_retry_after",
			"provider_call_revision", "provider_run_watermark",
		},
		"github_ci_run_audit_events": {
			"request_id", "event_type", "failure_class", "details_json",
		},
	}
	for table, columns := range want {
		got, err := store.tableColumns(table)
		if err != nil {
			t.Fatalf("tableColumns(%s): %v", table, err)
		}
		for _, column := range columns {
			if _, ok := got[column]; !ok {
				t.Errorf("%s.%s is missing", table, column)
			}
		}
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("schema replay: %v", err)
	}
}

func TestStoreCIRunExecutionLeaseExpiresBeforeProviderStart(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	request, _, err := store.ClaimCIRunRequest(ctx, testCIRunRequest(grant, now))
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := store.AcquireCIRunExecution(ctx, request, "worker-1", now, 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	acquired, err = store.AcquireCIRunExecution(ctx, request, "worker-2", now.Add(29*time.Second), 30*time.Second)
	if err != nil || acquired {
		t.Fatalf("early takeover = %v, %v", acquired, err)
	}
	acquired, err = store.AcquireCIRunExecution(ctx, request, "worker-2", now.Add(30*time.Second), 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("expired takeover = %v, %v", acquired, err)
	}
	request.Operation = CIRunOperationRerunFailedJobs
	request.ProviderWorkflowID = 77
	request.ExecutionOwner = "worker-1"
	if err := store.MarkCIRunProviderCallStarted(ctx, request, now.Add(31*time.Second)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale owner provider start error = %v, want sql.ErrNoRows", err)
	}
	request.ExecutionOwner = "worker-2"
	if err := store.MarkCIRunProviderCallStarted(ctx, request, now.Add(31*time.Second)); err != nil {
		t.Fatalf("current owner provider start: %v", err)
	}
}

func TestStoreCIRunTerminalWriteRejectsStaleLeaseOwner(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	request, _, err := store.ClaimCIRunRequest(ctx, testCIRunRequest(grant, now))
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireCIRunExecution(ctx, request, "worker-1", now, time.Second); err != nil || !acquired {
		t.Fatalf("worker-1 acquire = %v, %v", acquired, err)
	}
	stale := *request
	if acquired, err := store.AcquireCIRunExecution(ctx, request, "worker-2", now.Add(time.Second), time.Minute); err != nil || !acquired {
		t.Fatalf("worker-2 takeover = %v, %v", acquired, err)
	}
	stale.Status = CIRunRequestFailed
	stale.FailureClass = string(CIRunFailureProviderUnavailable)
	stale.UpdatedAt = now.Add(2 * time.Second)
	if err := store.CompleteCIRunRequest(ctx, &stale); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale terminal write error = %v, want sql.ErrNoRows", err)
	}
	loaded, err := store.GetCIRunRequest(ctx, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != CIRunRequestPending || loaded.ExecutionOwner != "worker-2" {
		t.Fatalf("stale owner overwrote request: %+v", loaded)
	}
}

func TestStoreCIRunAmbiguousWriteCannotDowngradeSuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	request, _, err := store.ClaimCIRunRequest(ctx, testCIRunRequest(grant, now))
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireCIRunExecution(ctx, request, "worker-1", now, time.Minute); err != nil || !acquired {
		t.Fatalf("acquire = %v, %v", acquired, err)
	}
	request.Operation = CIRunOperationRerunFailedJobs
	request.ProviderWorkflowID = 77
	startedAt := now.Add(time.Second)
	if err := store.MarkCIRunProviderCallStarted(ctx, request, startedAt); err != nil {
		t.Fatal(err)
	}
	request.ProviderCallStartedAt = &startedAt
	ambiguous := *request
	request.Status = CIRunRequestSucceeded
	request.ProviderRunID = request.SourceRunID
	request.ProviderAttempt = request.ExpectedSourceAttempt + 1
	request.UpdatedAt = now.Add(2 * time.Second)
	if err := store.CompleteCIRunRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	ambiguous.Status = CIRunRequestReconciling
	ambiguous.FailureClass = string(CIRunFailureProviderCallAmbiguous)
	ambiguous.UpdatedAt = now.Add(3 * time.Second)
	if err := store.CompleteCIRunRequest(ctx, &ambiguous); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("late ambiguous write error = %v, want sql.ErrNoRows", err)
	}
	loaded, err := store.GetCIRunRequest(ctx, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != CIRunRequestSucceeded || loaded.ProviderRunID != request.SourceRunID {
		t.Fatalf("success was downgraded: %+v", loaded)
	}
}

func TestStoreCIRunProviderRevisionRejectsEarlierAttempt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 123_456_789, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	request, _, err := store.ClaimCIRunRequest(ctx, testCIRunRequest(grant, now))
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireCIRunExecution(ctx, request, "worker-1", now, time.Minute); err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	request.Operation = CIRunOperationRerunFailedJobs
	request.ProviderWorkflowID = 77
	if err := store.MarkCIRunProviderCallStarted(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	earlierAttempt := *request
	if err := store.DeferCIRunForRateLimit(ctx, request, now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireCIRunExecution(
		ctx, request, "worker-2", now.Add(2*time.Minute), time.Minute,
	); err != nil || !acquired {
		t.Fatalf("retry acquire = %v, %v", acquired, err)
	}
	if err := store.MarkCIRunProviderCallStarted(ctx, request, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	earlierAttempt.Status = CIRunRequestReconciling
	earlierAttempt.FailureClass = string(CIRunFailureProviderCallAmbiguous)
	earlierAttempt.UpdatedAt = now.Add(3 * time.Minute)
	if err := store.CompleteCIRunRequest(ctx, &earlierAttempt); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("earlier provider attempt error = %v, want sql.ErrNoRows", err)
	}
	loaded, err := store.GetCIRunRequest(ctx, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProviderCallRevision != 2 || loaded.Status != CIRunRequestReconciling ||
		loaded.FailureClass != "" {
		t.Fatalf("earlier provider attempt overwrote retry: %+v", loaded)
	}
}

func TestStoreClaimCIRunRequestIsIdempotentAndConcurrent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatalf("upsert grant: %v", err)
	}
	req := testCIRunRequest(grant, now)

	const callers = 12
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, _, err := store.ClaimCIRunRequest(ctx, req)
			if err != nil {
				errs <- err
				return
			}
			ids <- claimed.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Errorf("claim: %v", err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Errorf("claim ID = %q, want %q", id, first)
		}
	}
	var count int
	if err := store.db.Get(&count, `SELECT COUNT(*) FROM github_ci_run_requests`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("request rows = %d, want 1", count)
	}

	semanticConflict := *req
	semanticConflict.ID = "request-other"
	semanticConflict.IdempotencyHash = strings.Repeat("b", 64)
	if _, _, err := store.ClaimCIRunRequest(ctx, &semanticConflict); !errors.Is(err, ErrCIRunSemanticConflict) {
		t.Fatalf("semantic duplicate error = %v", err)
	}
	idempotencyConflict := *req
	idempotencyConflict.ID = "request-reused-key"
	idempotencyConflict.PRNumber = 43
	if _, _, err := store.ClaimCIRunRequest(ctx, &idempotencyConflict); !errors.Is(err, ErrCIRunIdempotencyConflict) {
		t.Fatalf("idempotency reuse error = %v", err)
	}
}

func TestStoreCIRunRequestProviderStartAndAuditAreRedacted(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	req, _, err := store.ClaimCIRunRequest(ctx, testCIRunRequest(grant, now))
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireCIRunExecution(ctx, req, "worker-1", now, 30*time.Second); err != nil || !acquired {
		t.Fatalf("acquire provider worker = %v, %v", acquired, err)
	}
	req.Operation = CIRunOperationRerunFailedJobs
	req.ProviderWorkflowID = 77
	req.ProviderHeadRepo = "kdlbs/kandev"
	req.ProviderHeadRef = "feature/x"
	req.ProviderHeadSHA = req.ExpectedHeadSHA
	if err := store.MarkCIRunProviderCallStarted(ctx, req, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetCIRunRequest(ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProviderCallStartedAt == nil {
		t.Fatal("provider call start was not persisted")
	}
	dispatchStartedAt := now.Add(2 * time.Second)
	if err := store.PrepareCIRunDispatchFallback(ctx, req, dispatchStartedAt); err != nil {
		t.Fatal(err)
	}
	if acquired, err := store.AcquireCIRunExecution(
		ctx, req, "worker-2", dispatchStartedAt, 30*time.Second,
	); err != nil || !acquired {
		t.Fatalf("acquire dispatch worker = %v, %v", acquired, err)
	}
	if err := store.MarkCIRunProviderCallStarted(ctx, req, dispatchStartedAt); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.GetCIRunRequest(ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProviderCallStartedAt == nil || !loaded.ProviderCallStartedAt.Equal(dispatchStartedAt) {
		t.Fatalf("dispatch provider call start = %v, want %v", loaded.ProviderCallStartedAt, dispatchStartedAt)
	}
	if err := store.AppendCIRunAuditEvent(ctx, &CIRunAuditEvent{
		ID: "audit-1", RequestID: req.ID, EventType: "provider_started",
		FailureClass: "", DetailsJSON: `{"operation":"rerun_failed_jobs"}`, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	var details string
	if err := store.db.Get(&details, `SELECT details_json FROM github_ci_run_audit_events WHERE id = ?`, "audit-1"); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"token", "authorization", "private_key"} {
		if strings.Contains(strings.ToLower(details), secret) {
			t.Fatalf("audit details contain forbidden key %q: %s", secret, details)
		}
	}
	if _, err := store.GetCIRunRequest(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing request error = %v", err)
	}
}

func TestDeleteWorkspaceSettingsRemovesCIRunAuthorityAndAudit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	request, _, err := store.ClaimCIRunRequest(ctx, testCIRunRequest(grant, now))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendCIRunAuditEvent(ctx, &CIRunAuditEvent{
		ID: "audit-delete", RequestID: request.ID, EventType: "created",
		DetailsJSON: `{}`, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWorkspaceSettings(ctx, grant.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"github_ci_run_grants", "github_ci_run_requests", "github_ci_run_audit_events",
	} {
		var count int
		if err := store.db.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s rows = %d, want 0", table, count)
		}
	}
}

func testCIRunGrant(now time.Time) *CIRunGrant {
	return &CIRunGrant{
		ID: "grant-1", WorkspaceID: "workspace-1", ActorTaskID: "coordinator-1",
		TargetTaskID: "target-1", WorkflowID: "workflow-1", WorkflowStepID: "ci-fixup",
		RepositoryID: "repository-1", CreatedByUserID: "admin-1", CreatedAt: now, UpdatedAt: now,
	}
}

func testCIRunRequest(grant *CIRunGrant, now time.Time) *CIRunRequest {
	return &CIRunRequest{
		ID: "request-1", GrantID: grant.ID, WorkspaceID: grant.WorkspaceID,
		ActorTaskID: grant.ActorTaskID, ActorSessionID: "session-1", TargetTaskID: grant.TargetTaskID,
		WorkflowID: grant.WorkflowID, WorkflowStepID: grant.WorkflowStepID, RepositoryID: grant.RepositoryID,
		PRNumber: 42, ExpectedHeadSHA: strings.Repeat("a", 40), SourceRunID: 100,
		ExpectedSourceAttempt: 1, EvidenceKind: CIRunEvidencePRHead,
		IdempotencyHash: strings.Repeat("a", 64), Status: CIRunRequestPending,
		CreatedAt: now, UpdatedAt: now,
	}
}
