package github

import (
	"context"
	"errors"
	"testing"
)

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
