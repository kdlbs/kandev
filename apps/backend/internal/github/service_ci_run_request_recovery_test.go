package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestFreshCIRunRejectsGrantRevokedAtRerunStart(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.runHook = func(call int) {
		if call == 2 {
			_, _ = service.store.db.Exec(`UPDATE github_ci_run_grants SET revoked_at = CURRENT_TIMESTAMP`)
		}
	}
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureNotAuthorized {
		t.Fatalf("error = %#v, want not_authorized", err)
	}
	if client.reruns != 0 {
		t.Fatalf("provider reruns = %d, want zero after grant revocation", client.reruns)
	}
}

func TestRequestFreshCIRunRejectsLaneChangeAtDispatchStart(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	client.listHook = func() {
		client.listHook = nil
		_, _ = service.store.db.Exec(`UPDATE tasks SET workflow_step_id = 'review' WHERE id = 'target-1'`)
	}

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureWorkflowStepMismatch {
		t.Fatalf("error = %#v, want workflow_step_mismatch", err)
	}
	if client.dispatches != 0 {
		t.Fatalf("provider dispatches = %d, want zero after lane change", client.dispatches)
	}
}

func TestRequestFreshCIRunRetriesRateLimitedMutationAfterReset(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	now := service.ciRunClock()().UTC()
	reset := now.Add(time.Minute)
	client.rerunErr = &CIRunProviderError{
		Class: CIRunFailureProviderRateLimited, StatusCode: 429,
		Retryable: true, RetryAfter: &reset,
	}

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderRateLimited {
		t.Fatalf("first error = %#v, want provider_rate_limited", err)
	}
	_, err = service.RequestFreshCIRun(context.Background(), input)
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderRateLimited || client.reruns != 1 {
		t.Fatalf("early retry error = %#v, provider reruns = %d", err, client.reruns)
	}

	service.ciRunNow = func() time.Time { return reset.Add(time.Second) }
	client.rerunErr = nil
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || client.reruns != 2 {
		t.Fatalf("receipt = %+v, provider reruns = %d", receipt, client.reruns)
	}
}

func TestRequestFreshCIRunReconcilesAmbiguousMutationWithoutResending(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = classifyCIRunProviderError(
		&GitHubAPIError{StatusCode: 503, Endpoint: "/rerun"}, true, true,
	)
	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("first error = %#v", err)
	}
	client.runs = []GitHubActionsRun{{
		ID: input.SourceRunID, Attempt: 2, WorkflowID: 77, Event: "pull_request",
		HeadSHA: input.ExpectedHeadSHA, HeadBranch: "feature/x",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
	}}
	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || receipt.Attempt != 2 ||
		receipt.WorkflowName != "E2E" || receipt.WorkflowPath != reviewedDispatchWorkflow {
		t.Fatalf("receipt = %+v", receipt)
	}
	if client.reruns != 1 {
		t.Fatalf("provider reruns = %d, want one ambiguous send only", client.reruns)
	}
}

func TestRequestFreshCIRunKeepsRateLimitedReconciliationClosedToResend(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	reset := service.ciRunClock()().UTC().Add(time.Minute)
	client.listErr = &CIRunProviderError{
		Class: CIRunFailureProviderRateLimited, StatusCode: 429,
		Retryable: true, RetryAfter: &reset,
	}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderRateLimited {
		t.Fatalf("error = %#v, want provider_rate_limited", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || client.reruns != 1 {
		t.Fatalf("receipt = %+v, provider reruns = %d", receipt, client.reruns)
	}
	persisted, err := service.store.GetCIRunRequest(context.Background(), receipt.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ProviderCallStartedAt == nil || persisted.Status != CIRunRequestReconciling {
		t.Fatalf("rate limit reopened sent mutation: %+v", persisted)
	}

	client.listErr = nil
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
	service.ciRunNow = func() time.Time { return reset.Add(time.Second) }
	receipt, err = service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || client.reruns != 1 {
		t.Fatalf("receipt = %+v, provider reruns = %d", receipt, client.reruns)
	}
}

func TestRequestFreshCIRunTakesOverExpiredPreProviderLease(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	now := service.ciRunClock()().UTC()
	binding, err := service.loadCIRunBinding(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := service.store.ClaimCIRunRequest(
		context.Background(), newCIRunRequest(binding, input, now),
	)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := service.store.AcquireCIRunExecution(
		context.Background(), request, "crashed-worker", now, 30*time.Second,
	)
	if err != nil || !acquired {
		t.Fatalf("seed crashed lease = %v, %v", acquired, err)
	}
	service.ciRunNow = func() time.Time { return now.Add(31 * time.Second) }
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || client.reruns != 1 {
		t.Fatalf("receipt = %+v, provider reruns = %d", receipt, client.reruns)
	}
}

func TestRequestFreshCIRunDoesNotReplaySucceededReceiptForDifferentHead(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
	first, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	stale := input
	stale.ExpectedHeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	stale.IdempotencyKey = "different-head"
	receipt, err := service.RequestFreshCIRun(context.Background(), stale)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureHeadDrift {
		t.Fatalf("error = %#v, want head_drift", err)
	}
	if receipt == nil || receipt.RequestID == first.RequestID || receipt.Status != CIRunRequestFailed {
		t.Fatalf("receipt = %+v, want a distinct failed request from %+v", receipt, first)
	}
	if client.reruns != 1 {
		t.Fatalf("provider reruns = %d, want only the original request", client.reruns)
	}
}
