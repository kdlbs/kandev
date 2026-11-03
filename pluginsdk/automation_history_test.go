package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type automationHistoryTestHost struct {
	UnimplementedHostData
	reader AutomationHistoryReader
}

func (h *automationHistoryTestHost) GetState(context.Context, string, string, string) (map[string]any, bool, error) {
	return nil, false, nil
}
func (h *automationHistoryTestHost) SetState(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (h *automationHistoryTestHost) DeleteState(context.Context, string, string, string) error {
	return nil
}
func (h *automationHistoryTestHost) ListState(context.Context, string, string) ([]StateEntry, error) {
	return nil, nil
}
func (h *automationHistoryTestHost) GetConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}
func (h *automationHistoryTestHost) RevealSecret(context.Context, string) (string, error) {
	return "", nil
}
func (h *automationHistoryTestHost) GetSecret(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (h *automationHistoryTestHost) SetSecret(context.Context, string, string) error { return nil }
func (h *automationHistoryTestHost) DeleteSecret(context.Context, string) error      { return nil }
func (h *automationHistoryTestHost) EmitEvent(context.Context, string, map[string]any) error {
	return nil
}

func (h *automationHistoryTestHost) AutomationHistory() AutomationHistoryReader { return h.reader }

type recordingAutomationHistoryReader struct {
	definitionQuery AutomationDefinitionQuery
	runQuery        AutomationRunQuery
	getQuery        AutomationRunGetQuery
}

func (r *recordingAutomationHistoryReader) ListAutomations(_ context.Context, q AutomationDefinitionQuery) (AutomationDefinitionPage, error) {
	r.definitionQuery = q
	timezone := "Europe/Berlin"
	return AutomationDefinitionPage{
		Items: []AutomationDefinitionObservation{{
			ID: "automation-1", WorkspaceID: q.WorkspaceID, Name: "nightly", Enabled: true,
			CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
			ResourceVersion: "definition-version", ObservedAt: "2026-10-03T00:00:00Z",
			Triggers: []AutomationTriggerObservation{{
				ID: "trigger-1", Kind: "scheduled", Enabled: true,
				CronExpression: "0 9 * * *", Timezone: &timezone, ScheduleAvailable: true,
			}},
		}},
		PageInfo: ExactReadPageInfo{HasMore: false, SnapshotVersion: "snapshot-1", ObservedAt: "2026-10-03T00:00:00Z"},
	}, nil
}

func (r *recordingAutomationHistoryReader) ListRuns(_ context.Context, q AutomationRunQuery) (AutomationRunPage, error) {
	r.runQuery = q
	attempt := int64(2)
	scheduledAt := "2026-10-01T00:05:00Z"
	return AutomationRunPage{
		Items: []AutomationRunObservation{{
			ID: "run-1", WorkspaceID: q.WorkspaceID, AutomationID: "automation-1", AutomationName: "nightly",
			TriggerID: "trigger-1", TriggerKind: "scheduled", TaskID: "task-1", SessionID: "session-1",
			TurnID: "turn-1", ManagedConversationID: "conversation-1",
			SourceStatus: "task_created", Status: "archived", RetryState: "scheduled", DeliveryStatus: "accepted",
			RetryGroupID: "group-1", RetryParentRunID: "run-parent", AttemptNumber: &attempt,
			CreatedAt: "2026-10-01T00:00:00Z", RetryScheduledAt: &scheduledAt,
			Summary: "final answer", ErrorExcerpt: "safe error", ResourceVersion: "run-version",
			ObservedAt: "2026-10-03T00:00:00Z",
		}},
		PageInfo: ExactReadPageInfo{HasMore: false, SnapshotVersion: "snapshot-run"},
	}, nil
}

func (r *recordingAutomationHistoryReader) GetRun(_ context.Context, q AutomationRunGetQuery) (AutomationRunObservation, HostReadReceipt, error) {
	r.getQuery = q
	return AutomationRunObservation{
			ID: q.RunID, WorkspaceID: q.WorkspaceID, AutomationID: "automation-1",
			SourceStatus: "task_created", Status: "cancelled", CreatedAt: "2026-10-01T00:00:00Z",
			ResourceVersion: "fresh-version", ObservedAt: "2026-10-04T00:00:00Z",
		}, HostReadReceipt{
			InstallationID: "installation-1", WorkspaceID: q.WorkspaceID,
			CapabilityRevision: 7, RequestID: q.RequestID, SnapshotVersion: "receipt-version",
		}, nil
}

func TestAutomationHistoryRoundTrip(t *testing.T) {
	reader := &recordingAutomationHistoryReader{}
	host := dialHostOverBufconn(t, &automationHistoryTestHost{reader: reader})

	automationHistory, ok := HostAutomationHistory(host)
	require.True(t, ok)
	from := "2026-10-01T00:00:00Z"
	before := "2026-10-05T00:00:00Z"

	definitions, err := automationHistory.ListAutomations(context.Background(), AutomationDefinitionQuery{
		RequestID: "definitions-1", WorkspaceID: "workspace-1",
		Filter: AutomationDefinitionFilter{AutomationID: "automation-1", CreatedFrom: &from, CreatedBefore: &before},
		Page:   ExactReadPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Len(t, definitions.Items, 1)
	require.Equal(t, "automation-1", definitions.Items[0].ID)
	require.Equal(t, "definition-version", definitions.Items[0].ResourceVersion)
	require.True(t, definitions.Items[0].Triggers[0].ScheduleAvailable)
	require.NotNil(t, definitions.Items[0].Triggers[0].Timezone)
	require.Equal(t, "Europe/Berlin", *definitions.Items[0].Triggers[0].Timezone)
	require.Equal(t, &from, reader.definitionQuery.Filter.CreatedFrom)
	require.Equal(t, &before, reader.definitionQuery.Filter.CreatedBefore)
	require.Equal(t, "definitions-1", reader.definitionQuery.RequestID)
	require.Equal(t, int32(10), reader.definitionQuery.Page.Limit)

	runs, err := automationHistory.ListRuns(context.Background(), AutomationRunQuery{
		RequestID: "runs-1", WorkspaceID: "workspace-1",
		Filter: AutomationRunFilter{AutomationID: "automation-1"}, Page: ExactReadPage{Limit: 1},
	})
	require.NoError(t, err)
	require.Len(t, runs.Items, 1)
	run := runs.Items[0]
	require.Equal(t, "run-1", run.ID)
	require.Equal(t, "task_created", run.SourceStatus)
	require.Equal(t, "archived", run.Status)
	require.Equal(t, "scheduled", run.RetryState)
	require.Equal(t, "accepted", run.DeliveryStatus)
	require.NotNil(t, run.AttemptNumber)
	require.Equal(t, int64(2), *run.AttemptNumber)
	require.NotNil(t, run.RetryScheduledAt)
	require.Equal(t, "2026-10-01T00:05:00Z", *run.RetryScheduledAt)
	require.Nil(t, run.RetryClaimedAt)
	require.Equal(t, "final answer", run.Summary)
	require.Equal(t, "runs-1", reader.runQuery.RequestID)

	refreshed, receipt, err := automationHistory.GetRun(context.Background(), AutomationRunGetQuery{
		RequestID: "get-1", WorkspaceID: "workspace-1", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Equal(t, "cancelled", refreshed.Status)
	require.Equal(t, "fresh-version", refreshed.ResourceVersion)
	require.Equal(t, "installation-1", receipt.InstallationID)
	require.Equal(t, uint64(7), receipt.CapabilityRevision)
	require.Equal(t, "get-1", receipt.RequestID)
	require.Equal(t, "get-1", reader.getQuery.RequestID)
}

type legacyAutomationHost struct{ automationHistoryTestHost }

func (h *legacyAutomationHost) AutomationHistory() AutomationHistoryReader { return nil }

func TestAutomationHistoryOptionalHost(t *testing.T) {
	legacy := &legacyAutomationHost{}
	if _, ok := HostAutomationHistory(legacy); ok {
		t.Fatal("a host without the optional reader must not report support")
	}
	host := dialHostOverBufconn(t, legacy)
	reader, ok := HostAutomationHistory(host)
	require.True(t, ok, "the remote wrapper exists even when the server lacks the RPCs")
	_, err := reader.ListRuns(context.Background(), AutomationRunQuery{RequestID: "old-host", WorkspaceID: "workspace-1"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unimplemented, st.Code())

	if _, _, err := host.GetState(context.Background(), "scope", "scope-id", "key"); err != nil {
		t.Fatalf("legacy Host methods must keep working: %v", err)
	}
}
