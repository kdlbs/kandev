package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateFreshCIRunInputRejectsNonHexHeadSHA(t *testing.T) {
	input := RequestFreshCIRunInput{
		ActorTaskID: "coordinator-1", ActorSessionID: "session-1", TargetTaskID: "target-1",
		RepositoryID: "repository-1", PRNumber: 42, ExpectedHeadSHA: strings.Repeat("z", 40),
		ExpectedWorkflowStepID: "ci-fixup", SourceRunID: 100, ExpectedSourceAttempt: 1,
		EvidenceKind: CIRunEvidencePRHead, IdempotencyKey: "consumer-42-attempt-1",
	}

	if failure := validateFreshCIRunInput(input); failure != CIRunFailureTaskMismatch {
		t.Fatalf("validateFreshCIRunInput() = %q, want %q", failure, CIRunFailureTaskMismatch)
	}
}

func TestCIRunEvidenceVerdictPendingForPendingRequests(t *testing.T) {
	for _, status := range []CIRunRequestStatus{CIRunRequestPending, CIRunRequestReconciling} {
		request := &CIRunRequest{Status: status, EvidenceKind: CIRunEvidencePRHead}
		if got := evidenceVerdict(request); got != ciRunEvidenceVerdictPending {
			t.Fatalf("status %s verdict = %q, want pending", status, got)
		}
	}
}

func TestRequestFreshCIRunClassifiesInitialPRRateLimitAsRetryable(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	reset := service.ciRunClock()().UTC().Add(10 * time.Minute)
	client.prErrSequence = []error{&GitHubAPIError{
		StatusCode: 429, Endpoint: "/repos/kdlbs/kandev/pulls/42",
		Body: "rate limit exceeded", RetryAfter: &reset,
	}}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderRateLimited {
		t.Fatalf("error = %#v, want provider_rate_limited", err)
	}
	if receipt == nil || receipt.RequestID == "" || receipt.ProviderRetryAfter == nil ||
		!receipt.ProviderRetryAfter.Equal(reset) {
		t.Fatalf("receipt = %+v, want request identity and reset %s", receipt, reset)
	}
	loaded, loadErr := service.store.GetCIRunRequest(context.Background(), receipt.RequestID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.Status != CIRunRequestPending || loaded.ProviderRetryAfter == nil {
		t.Fatalf("stored request = %+v, want pending retry", loaded)
	}
}

func TestRequestFreshCIRunClassifiesFinalPRRateLimitAsRetryable(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	reset := service.ciRunClock()().UTC().Add(10 * time.Minute)
	client.prErrSequence = []error{nil, &GitHubAPIError{
		StatusCode: 429, Endpoint: "/repos/kdlbs/kandev/pulls/42",
		Body: "rate limit exceeded", RetryAfter: &reset,
	}}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderRateLimited {
		t.Fatalf("error = %#v, want provider_rate_limited", err)
	}
	if receipt == nil || receipt.ProviderRetryAfter == nil || !receipt.ProviderRetryAfter.Equal(reset) {
		t.Fatalf("receipt = %+v, want reset %s", receipt, reset)
	}
	if client.reruns != 0 || client.dispatches != 0 {
		t.Fatalf("provider mutated after final read rate limit: rerun=%d dispatch=%d",
			client.reruns, client.dispatches)
	}
}

func TestRequestFreshCIRunDefersPostMutationRateLimitWithoutResending(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	reset := service.ciRunClock()().UTC().Add(10 * time.Minute)
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
	client.listErrSequence = []error{&GitHubAPIError{
		StatusCode: 429, Endpoint: "/repos/kdlbs/kandev/actions/workflows/77/runs",
		Body: "rate limit exceeded", RetryAfter: &reset,
	}}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderRateLimited {
		t.Fatalf("error = %#v, want provider_rate_limited", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || receipt.ProviderRetryAfter == nil ||
		!receipt.ProviderRetryAfter.Equal(reset) || client.reruns != 1 {
		t.Fatalf("receipt = %+v, reruns = %d", receipt, client.reruns)
	}
	service.ciRunNow = func() time.Time { return reset.Add(time.Second) }
	receipt, err = service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || client.reruns != 1 || client.listCalls != 2 {
		t.Fatalf("receipt = %+v, reruns = %d, list calls = %d", receipt, client.reruns, client.listCalls)
	}
}

func TestRequestFreshCIRunDeniesDispatchWithoutServerDerivedRef(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	client.pr.HeadBranch = ""
	client.run.HeadBranch = ""

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureDispatchRefUnavailable {
		t.Fatalf("error = %#v, want dispatch_ref_unavailable", err)
	}
	if client.dispatches != 0 {
		t.Fatal("workflow was dispatched without a server-derived ref")
	}
}

func TestCIRunInstallationFailureDistinguishesConfigurationAndPermission(t *testing.T) {
	if got := ciRunInstallationFailure(ErrGitHubNotConfigured); got != CIRunFailureInstallationRequired {
		t.Fatalf("not configured = %q, want installation_required", got)
	}
	if got := ciRunInstallationFailure(ErrGitHubCapabilityDenied); got != CIRunFailureInstallationPermission {
		t.Fatalf("capability denied = %q, want installation_permission_missing", got)
	}
}
