package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequestFreshCIRunRejectsHeadDriftAfterFinalSourceRunRead(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	drifted := *client.pr
	drifted.HeadSHA = strings.Repeat("b", 40)
	client.runHook = func(call int) {
		if call == 2 {
			client.pr = &drifted
		}
	}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureHeadDrift {
		t.Fatalf("error = %#v, want head_drift", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestFailed ||
		receipt.FailureClass != string(CIRunFailureHeadDrift) {
		t.Fatalf("receipt = %+v, want terminal head_drift", receipt)
	}
	if receipt.ObservedPRHeadSHA != drifted.HeadSHA {
		t.Fatalf("observed PR head = %q, want %q", receipt.ObservedPRHeadSHA, drifted.HeadSHA)
	}
	if client.reruns != 0 {
		t.Fatalf("provider reruns = %d, want zero after final PR head drift", client.reruns)
	}
}
