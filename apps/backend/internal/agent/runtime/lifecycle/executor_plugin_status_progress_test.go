package lifecycle

import (
	"testing"
	"time"
)

// A row left in cleanup_progress owes cleanup exactly as cleanup_pending does.
func TestStatusReportsSavedCleanupProgressAsCleanupPending(t *testing.T) {
	status := pluginExecutorStatusFromInventory(pluginExecutorInventory{Phase: pluginExecutorPhaseCleanupProgress}, time.Now())
	if status.State != pluginExecutorPhaseCleanupPending || status.Reason != pluginExecutorPhaseCleanupPending {
		t.Fatalf("status = %+v, want cleanup_pending", status)
	}
}
