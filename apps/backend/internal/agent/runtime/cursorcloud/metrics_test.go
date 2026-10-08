package cursorcloud

import (
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCursorCloudMetricsUseClosedLabelsAndIncrementOnceAtClassification(t *testing.T) {
	read := func(metric, key string) int64 {
		t.Helper()
		value := expvar.Get(metric)
		m, ok := value.(*expvar.Map)
		if !ok {
			t.Fatalf("expvar %q is %T, want map", metric, value)
		}
		value = m.Get(key)
		if value == nil {
			return 0
		}
		counter, ok := value.(*expvar.Int)
		if !ok {
			t.Fatalf("counter %q/%q is %T, want int", metric, key, value)
		}
		return counter.Value()
	}

	key := "operation=create;outcome=unknown"
	before := read("cursor_cloud_dispatch_total", key)
	recordDispatch(models.ManagedAgentOperationCreate, "unknown")
	recordDispatch(models.ManagedAgentOperationCreate, "unknown")
	if got := read("cursor_cloud_dispatch_total", key); got != before+2 {
		t.Fatalf("dispatch counter=%d, want two independent classifications", got-before)
	}
	badKey := "operation=unbounded;outcome=unknown"
	recordDispatch(models.ManagedAgentOperationKind("unbounded"), "unknown")
	if got := read("cursor_cloud_dispatch_total", badKey); got != 0 {
		t.Fatalf("unexpected unbounded dispatch label count=%d", got)
	}

	unknownKey := "operation=followup;reason=crash_recovery"
	unknownBefore := read("cursor_cloud_submission_unknown_total", unknownKey)
	recordSubmissionUnknown(models.ManagedAgentOperationFollowup, "crash_recovery")
	if got := read("cursor_cloud_submission_unknown_total", unknownKey); got != unknownBefore+1 {
		t.Fatalf("unknown counter delta=%d, want one transition", got-unknownBefore)
	}

	reconnectKey := "reason=disconnect;outcome=history_expired"
	reconnectBefore := read("cursor_cloud_reconnect_total", reconnectKey)
	recordReconnect("disconnect", "history_expired")
	if got := read("cursor_cloud_reconnect_total", reconnectKey); got != reconnectBefore+1 {
		t.Fatalf("reconnect counter delta=%d, want one observation", got-reconnectBefore)
	}
	recordReconnect("session-123", "connected")
	if got := read("cursor_cloud_reconnect_total", "reason=session-123;outcome=connected"); got != 0 {
		t.Fatalf("unexpected reconnect identity label count=%d", got)
	}

	cancelKey := "outcome=pending"
	cancelBefore := read("cursor_cloud_cancel_total", cancelKey)
	recordCancel("pending")
	if got := read("cursor_cloud_cancel_total", cancelKey); got != cancelBefore+1 {
		t.Fatalf("cancel counter delta=%d, want one classified attempt", got-cancelBefore)
	}
}
