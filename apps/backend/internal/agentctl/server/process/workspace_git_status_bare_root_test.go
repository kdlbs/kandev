package process

import (
	"context"
	"testing"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
func TestGitStatusDetailsWaitBareRootReturnsRepositoryUnavailable(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached", true: "fresh"}[fresh], func(t *testing.T) {
			tracker := newStatusConcurrencyTracker(t)

			status, err := tracker.GetGitStatusWithDetails(context.Background(), fresh)
			if err != nil {
				t.Fatalf("GetGitStatusWithDetails() error = %v, want nil", err)
			}
			if status.StatusState != gitStatusStateUnavailable ||
				status.ErrorCode != gitStatusErrorNoRepository ||
				status.DetailState != gitStatusDetailUnavailable {
				t.Fatalf("GetGitStatusWithDetails() = %+v, want unavailable repository status", status)
			}
		})
	}
}
