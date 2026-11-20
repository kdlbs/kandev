package statussummary

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestQuotaWaitSummaryFromRouteState(t *testing.T) {
	deadline := time.Date(2026, 11, 9, 12, 30, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	policyJSON, err := json.Marshal(dynamicruntime.PolicyState{
		FailureCode: routingerr.CodeQuotaLimited,
		Deadline:    &deadline,
	})
	if err != nil {
		t.Fatal(err)
	}
	zero := time.Time{}
	zeroJSON, err := json.Marshal(dynamicruntime.PolicyState{
		FailureCode: routingerr.CodeQuotaLimited,
		Deadline:    &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	nilDeadlineJSON, err := json.Marshal(dynamicruntime.PolicyState{FailureCode: routingerr.CodeQuotaLimited})
	if err != nil {
		t.Fatal(err)
	}

	base := dynamicruntime.RouteState{
		SessionID:       "session-1",
		Status:          "retry_wait",
		PolicyStateJSON: string(policyJSON),
	}
	for _, test := range []struct {
		name  string
		state *dynamicruntime.RouteState
		want  bool
	}{
		{name: "quota retry wait", state: &base, want: true},
		{name: "quota reset wait", state: routeStateWithStatus(base, "waiting_for_reset"), want: true},
		{name: "nil state"},
		{name: "missing session", state: routeStateWithSession(base, "")},
		{name: "non-wait status", state: routeStateWithStatus(base, "action_required")},
		{name: "non-quota failure", state: routeStateWithPolicy(base, `{"failure_code":"provider_unavailable","deadline":"2026-11-09T10:30:00Z"}`)},
		{name: "missing deadline", state: routeStateWithPolicy(base, string(nilDeadlineJSON))},
		{name: "zero deadline", state: routeStateWithPolicy(base, string(zeroJSON))},
		{name: "invalid policy json", state: routeStateWithPolicy(base, "{")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := QuotaWaitSummaryFromRouteState(test.state)
			if (got != nil) != test.want {
				t.Fatalf("summary = %+v, want present %t", got, test.want)
			}
			if got != nil {
				if got.SessionID != base.SessionID || got.Reason != QuotaWaitReasonQuotaLimited {
					t.Fatalf("summary identity = %+v", got)
				}
				if want := deadline.UTC(); !got.Deadline.Equal(want) {
					t.Fatalf("deadline = %s, want UTC %s", got.Deadline, want)
				}
			}
		})
	}
}

func routeStateWithStatus(base dynamicruntime.RouteState, status string) *dynamicruntime.RouteState {
	base.Status = status
	return &base
}

func routeStateWithSession(base dynamicruntime.RouteState, sessionID string) *dynamicruntime.RouteState {
	base.SessionID = sessionID
	return &base
}

func routeStateWithPolicy(base dynamicruntime.RouteState, policyJSON string) *dynamicruntime.RouteState {
	base.PolicyStateJSON = policyJSON
	return &base
}

func TestProjectorContinuesSessionUpdatesWhenQuotaWaitReadFails(t *testing.T) {
	quotaWait := &QuotaWaitSummary{
		SessionID: "session-1",
		Reason:    QuotaWaitReasonQuotaLimited,
		Deadline:  time.Date(2026, 11, 9, 12, 30, 0, 0, time.UTC),
	}
	tests := []struct {
		name    string
		initial *TaskStatusSummary
		want    *QuotaWaitSummary
	}{
		{name: "cold projection"},
		{
			name: "restored projection",
			initial: &TaskStatusSummary{
				Revision:       3,
				PrimarySession: &PrimarySessionSummary{ID: "session-1", State: "RUNNING"},
				QuotaWait:      quotaWait,
			},
			want: quotaWait,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var reads atomic.Int64
			projector, store, updates, closeBus := newQuotaWaitProjector(t, test.initial, func(context.Context, string) (*QuotaWaitSummary, error) {
				reads.Add(1)
				return nil, errors.New("route store temporarily unavailable")
			})
			defer closeBus()

			err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
				"task_id":      "task-1",
				"workspace_id": "workspace-1",
				"session_id":   "session-1",
				"new_state":    "WAITING_FOR_INPUT",
				"is_primary":   true,
			}))
			if err != nil {
				t.Fatalf("HandleEvent: %v", err)
			}

			got := store.summary("task-1")
			if got == nil || got.PrimarySession == nil || got.PrimarySession.State != "WAITING_FOR_INPUT" {
				t.Fatalf("stored session transition = %+v, want WAITING_FOR_INPUT", got)
			}
			if !equalQuotaWait(got.QuotaWait, test.want) {
				t.Fatalf("stored quota wait = %+v, want %+v", got.QuotaWait, test.want)
			}
			if updates.Load() != 1 {
				t.Fatalf("published summary updates = %d, want 1", updates.Load())
			}
			if reads.Load() < 1 {
				t.Fatalf("quota wait reads = %d, want at least 1", reads.Load())
			}
		})
	}
}

func TestProjectorClearsQuotaWaitWhenSourceConfirmsNoWait(t *testing.T) {
	initial := &TaskStatusSummary{
		Revision:       3,
		PrimarySession: &PrimarySessionSummary{ID: "session-1", State: "RUNNING"},
		QuotaWait: &QuotaWaitSummary{
			SessionID: "session-1",
			Reason:    QuotaWaitReasonQuotaLimited,
			Deadline:  time.Date(2026, 11, 9, 12, 30, 0, 0, time.UTC),
		},
	}
	projector, store, updates, closeBus := newQuotaWaitProjector(t, initial, func(context.Context, string) (*QuotaWaitSummary, error) {
		return nil, nil
	})
	defer closeBus()

	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
		"task_id":      "task-1",
		"workspace_id": "workspace-1",
		"session_id":   "session-1",
		"new_state":    "WAITING_FOR_INPUT",
		"is_primary":   true,
	})); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if got := store.summary("task-1"); got == nil || got.QuotaWait != nil {
		t.Fatalf("stored quota wait = %+v, want cleared", got)
	}
	if updates.Load() != 1 {
		t.Fatalf("published summary updates = %d, want 1", updates.Load())
	}
}

func newQuotaWaitProjector(
	t *testing.T,
	initial *TaskStatusSummary,
	loader QuotaWaitLoader,
) (*Projector, *projectorTestStore, *atomic.Int64, func()) {
	t.Helper()
	store := newProjectorTestStore()
	if initial != nil {
		store.rows["task-1"] = &StoredTaskStatusSummary{
			TaskID:      "task-1",
			WorkspaceID: "workspace-1",
			Summary:     *cloneSummary(initial),
		}
	}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	updates := new(atomic.Int64)
	if _, err := eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(context.Context, *bus.Event) error {
		updates.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadQuotaWait: loader,
		Now: func() time.Time {
			return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
		},
	})
	return projector, store, updates, func() {
		projector.Close()
		eventBus.Close()
	}
}
