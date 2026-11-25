package backendapp

import (
	"fmt"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A task that left the observed step surfaces as a changed-approval conflict,
// distinct from "nothing awaiting approval".
func TestStepApprovalStatusErrorMapsALostStepRaceToAborted(t *testing.T) {
	err := stepApprovalStatusError(fmt.Errorf("move: %w", taskservice.ErrStepApprovalChanged))
	if status.Code(err) != codes.Aborted {
		t.Fatalf("code = %v, want Aborted", status.Code(err))
	}
	if status.Code(stepApprovalStatusError(taskservice.ErrStepNotAwaitingApproval)) != codes.FailedPrecondition {
		t.Fatal("not-awaiting must stay FailedPrecondition")
	}
}
