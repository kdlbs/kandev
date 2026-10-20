package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// Office answers run-owned permission requests; the task permission path must
// not write task messages, resolve, or invoke the unattended handler for them
// (AC-OFFICE-TASKLESS-001.8).
func TestHandlePermissionRequest_IgnoresRunOwned(t *testing.T) {
	f := newUnattendedFixture(t, rejectAndAllow)
	creator := f.svc.messageCreator.(*mockMessageCreator)
	unattendedCalls := 0
	f.svc.SetUnattendedPermissionHandler(func(context.Context, string, string, string, string) {
		unattendedCalls++
	})

	f.svc.handlePermissionRequest(context.Background(), watcher.PermissionRequestData{
		OwnerKind:            "run",
		RequestID:            "request-1",
		PendingID:            "pending-1",
		AutoApprovedOptionID: "allow-once",
		AutoApprovalPending:  true,
	})

	if creator.permissionMessageWrites != 0 || len(f.delivered) != 0 || len(f.audits) != 0 || unattendedCalls != 0 {
		t.Fatalf("run-owned request reached task handling: writes=%d delivered=%v audits=%d unattended=%d",
			creator.permissionMessageWrites, f.delivered, len(f.audits), unattendedCalls)
	}
}
