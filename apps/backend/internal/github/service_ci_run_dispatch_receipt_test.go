package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRequestFreshCIRunVerifiesReturnedDispatchRun(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	dispatched := &GitHubActionsRun{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
		CreatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	}
	client.runSequence = []*GitHubActionsRun{client.run, client.run, client.run, dispatched}
	client.mutationMetadata = GitHubRequestMetadata{RunID: dispatched.ID}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RunID != dispatched.ID || receipt.Attempt != 1 {
		t.Fatalf("receipt run = %d attempt %d, want returned dispatch run", receipt.RunID, receipt.Attempt)
	}
	if client.listCalls != 1 {
		t.Fatalf("workflow run list calls = %d, want only the pre-dispatch watermark read", client.listCalls)
	}
}

func TestRequestFreshCIRunRejectsMismatchedReturnedDispatchRun(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	dispatched := &GitHubActionsRun{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: strings.Repeat("b", 40),
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
		CreatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	}
	client.runSequence = []*GitHubActionsRun{client.run, client.run, client.run, dispatched}
	client.mutationMetadata = GitHubRequestMetadata{RunID: dispatched.ID}

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if client.listCalls != 1 || client.dispatches != 1 {
		t.Fatalf("provider calls: list=%d dispatch=%d, want one watermark read and one dispatch",
			client.listCalls, client.dispatches)
	}
}

func TestRequestFreshCIRunDeniesDispatchWithRequiredInputWithoutDefault(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte(`on:
  workflow_dispatch:
    inputs:
      environment:
        required: true
        type: string
`)

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureDispatchDenied {
		t.Fatalf("error = %#v, want workflow_dispatch_denied", err)
	}
	if client.dispatches != 0 {
		t.Fatalf("provider dispatches = %d, want zero for required input without default", client.dispatches)
	}
}
