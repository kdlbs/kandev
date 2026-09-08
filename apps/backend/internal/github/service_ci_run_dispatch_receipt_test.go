package github

import (
	"context"
	"errors"
	"testing"
)

func TestRequestFreshCIRunTerminatesIneligibleRerunsWithoutReadingWorkflowInputs(t *testing.T) {
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
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureDispatchRefUnavailable {
		t.Fatalf("error = %#v, want dispatch_ref_unavailable", err)
	}
	if client.dispatches != 0 || client.listCalls != 0 {
		t.Fatalf("dispatch fallback was inspected: dispatches=%d list calls=%d", client.dispatches, client.listCalls)
	}
}
