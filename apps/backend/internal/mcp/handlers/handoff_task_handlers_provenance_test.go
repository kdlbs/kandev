package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/dashboard"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeshared "github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// --- Test doubles for AC-19a's activity/run-id wiring ---

type fakeRunResolverCall struct {
	taskID, sessionID string
}

// fakeRunResolver lets tests control ResolveRunForTaskAndSession's answer
// directly, rather than re-exercising the real "which run is newest" logic
// (covered by TestResolveRunForTaskAndSession_MatchesNewestClaimedRunsSession
// in internal/office/service) — this only proves the handler wires the call
// correctly.
type fakeRunResolver struct {
	mu    sync.Mutex
	runID string
	calls []fakeRunResolverCall
}

func (r *fakeRunResolver) ResolveRunForTask(context.Context, string) string { return "" }

func (r *fakeRunResolver) ResolveRunForTaskAndSession(_ context.Context, taskID, sessionID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fakeRunResolverCall{taskID: taskID, sessionID: sessionID})
	return r.runID
}

type fakeActivityRepo struct {
	mu      sync.Mutex
	entries []*officemodels.ActivityEntry
}

func (r *fakeActivityRepo) CreateActivityEntry(_ context.Context, entry *officemodels.ActivityEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	return nil
}

// newHandoffDashboardService wires a DashboardService whose only live seams
// are activity logging and run resolution (AC-19/AC-19a) — the repository
// dependency is nil because neither seam touches it.
func newHandoffDashboardService(t *testing.T, runID string) (*dashboard.DashboardService, *fakeActivityRepo, *fakeRunResolver) {
	t.Helper()
	activityRepo := &fakeActivityRepo{}
	resolver := &fakeRunResolver{runID: runID}
	log := testLogger(t)
	activityLogger := officeshared.NewActivityLogger(activityRepo, log)
	svc := dashboard.NewDashboardService(nil, log, activityLogger, nil, nil)
	svc.SetRunResolver(resolver)
	return svc, activityRepo, resolver
}

// --- AC-19a: activity rows carry the resolved run id ---

func TestHandleHandoffTask_ActivityLogUsesResolvedRunID(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	dashSvc, activityRepo, resolver := newHandoffDashboardService(t, "run-newest-abc")
	f.h.SetDashboardService(dashSvc)

	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	require.Len(t, resolver.calls, 1)
	assert.Equal(t, f.sourceTaskID, resolver.calls[0].taskID)
	assert.Equal(t, f.sourceSessionID, resolver.calls[0].sessionID)

	require.Len(t, activityRepo.entries, 2)
	sourceEntry, targetEntry := activityRepo.entries[0], activityRepo.entries[1]

	assert.Equal(t, "task.handed_off", string(sourceEntry.Action))
	assert.Equal(t, f.sourceWorkspaceID, sourceEntry.WorkspaceID)
	assert.Equal(t, f.sourceTaskID, sourceEntry.TargetID)
	assert.Equal(t, "run-newest-abc", sourceEntry.RunID)
	assert.Equal(t, f.sourceSessionID, sourceEntry.SessionID)

	assert.Equal(t, "task.handoff_received", string(targetEntry.Action))
	assert.Equal(t, f.targetWorkspaceID, targetEntry.WorkspaceID)
	assert.Equal(t, result.TaskID, targetEntry.TargetID)
	assert.Equal(t, "run-newest-abc", targetEntry.RunID)

	// AC-19a: each side's Details payload must let a viewer of one workspace's
	// activity feed navigate to the counterpart task on the other side.
	var sourceDetails, targetDetails map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(sourceEntry.Details), &sourceDetails))
	require.NoError(t, json.Unmarshal([]byte(targetEntry.Details), &targetDetails))

	assert.Equal(t, result.TaskID, sourceDetails["counterpart_task_id"])
	assert.Equal(t, f.targetWorkspaceID, sourceDetails["counterpart_workspace_id"])
	assert.Equal(t, handoffOutcomeCreated, sourceDetails["outcome"])

	assert.Equal(t, f.sourceTaskID, targetDetails["counterpart_task_id"])
	assert.Equal(t, f.sourceWorkspaceID, targetDetails["counterpart_workspace_id"])
	assert.Equal(t, handoffOutcomeCreated, targetDetails["outcome"])
}

func TestHandleHandoffTask_ActivityLogEmptyRunIDWhenCallerSessionNotNewest(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	dashSvc, activityRepo, _ := newHandoffDashboardService(t, "")
	f.h.SetDashboardService(dashSvc)

	_, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)

	require.Len(t, activityRepo.entries, 2)
	assert.Empty(t, activityRepo.entries[0].RunID, "not a defect: the newest claimed run may belong to another session")
	assert.Empty(t, activityRepo.entries[1].RunID)
}

// --- AC-23a: the delivery task's provenance reads as an ordinary manual task ---

func TestHandleHandoffTask_DeliveryTaskHasManualObservableProvenance(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	workspaceBefore, err := f.svc.GetWorkspace(context.Background(), f.targetWorkspaceID)
	require.NoError(t, err)
	sequenceBefore := workspaceBefore.TaskSequence

	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	deliveryTask, err := f.svc.GetTask(context.Background(), result.TaskID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskOriginManual, deliveryTask.Origin,
		"AC-23a: origin must read as manual, not agent_created")
	assert.Empty(t, deliveryTask.Identifier, "AC-23a: no project sequence identifier")
	assert.False(t, deliveryTask.IsFromOffice, "AC-23a: IsFromOffice alone is an insufficient assertion and must also be false")

	workspaceAfter, err := f.svc.GetWorkspace(context.Background(), f.targetWorkspaceID)
	require.NoError(t, err)
	assert.Equal(t, sequenceBefore, workspaceAfter.TaskSequence, "AC-23a: target workspace task sequence must be unchanged")
}

// --- Helpers for AC-25/AC-25b's repair scenarios ---

// clearHandoffsMetadata resets a task's handoffs array to empty, simulating
// a reverse link that was lost (or never written), so a subsequent replay
// exercises ensureHandoffReverseLink's actual write path instead of the
// alreadyPresent short-circuit.
func clearHandoffsMetadata(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	raw, err := repo.GetTaskHandoffsRaw(ctx, taskID)
	require.NoError(t, err)
	stored, _, err := repo.SetTaskHandoffsIfUnchanged(ctx, taskID, raw, "[]")
	require.NoError(t, err)
	require.True(t, stored, "precondition: clearing the handoffs array must succeed")
}

// setDeliveryHandoffSource overwrites a delivery task's stored handoff_source
// metadata wholesale (UpdateTaskMetadata merges at the top-level key, so this
// replaces the nested map in one call), letting tests corrupt one field
// (typically handed_off_at) while keeping the rest realistic.
func setDeliveryHandoffSource(
	t *testing.T, svc *service.Service, deliveryTaskID, sourceTaskID, sourceWorkspaceID, sourceSessionID, callerAgentProfileID string,
	handedOffAt interface{},
) {
	t.Helper()
	_, err := svc.UpdateTaskMetadata(context.Background(), deliveryTaskID, map[string]interface{}{
		models.MetaKeyHandoffSource: map[string]interface{}{
			handoffSourceTaskIDKey:    sourceTaskID,
			"source_workspace_id":     sourceWorkspaceID,
			"source_session_id":       sourceSessionID,
			"source_agent_profile_id": callerAgentProfileID,
			handoffHandedOffAtKey:     handedOffAt,
		},
	})
	require.NoError(t, err)
}

// setDeliveryHandoffSourceMissingTimestampKey is setDeliveryHandoffSource's
// sibling for AC-25b's fourth shape: handoff_source with no handed_off_at key
// at all, distinct from a present-but-empty or present-but-unparseable value.
func setDeliveryHandoffSourceMissingTimestampKey(
	t *testing.T, svc *service.Service, deliveryTaskID, sourceTaskID, sourceWorkspaceID, sourceSessionID, callerAgentProfileID string,
) {
	t.Helper()
	_, err := svc.UpdateTaskMetadata(context.Background(), deliveryTaskID, map[string]interface{}{
		models.MetaKeyHandoffSource: map[string]interface{}{
			handoffSourceTaskIDKey:    sourceTaskID,
			"source_workspace_id":     sourceWorkspaceID,
			"source_session_id":       sourceSessionID,
			"source_agent_profile_id": callerAgentProfileID,
		},
	})
	require.NoError(t, err)
}

// --- AC-25: a replay repair reuses the stored timestamp, not the replay's clock ---

func TestHandleHandoffTask_ReplayRepairUsesStoredTimestampNotReplayClock(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-repair-timestamp"})

	resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)

	clearHandoffsMetadata(t, f.repo, f.sourceTaskID)
	// A stored timestamp far from "now" makes the discriminating assertion
	// below (result2.HandedOffAt == storedTimestamp) fail loudly if the repair
	// ever regresses to stamping the replay's own clock instead of reusing
	// the delivery task's stored handoff_source.handed_off_at.
	const storedTimestamp = "2020-01-01T00:00:00.000Z"
	setDeliveryHandoffSource(t, f.svc, result1.TaskID, f.sourceTaskID, f.sourceWorkspaceID, f.sourceSessionID, f.callerAgentProfileID, storedTimestamp)

	resp2, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result2 := decodeHandoffResult(t, resp2)

	assert.Equal(t, handoffOutcomeFoundSettled, result2.Outcome)
	assert.True(t, result2.ReverseLinkRecorded)
	assert.Equal(t, storedTimestamp, result2.HandedOffAt, "the repair must reuse the delivery task's stored handoff_source timestamp, not the replay's own clock")

	raw, err := f.repo.GetTaskHandoffsRaw(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	var entries []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, storedTimestamp, entries[0][handoffHandedOffAtKey])
}

// --- AC-25b: three shapes of an unreadable stored timestamp ---

func TestHandleHandoffTask_UnreadableStoredTimestampSurfacesPartialFailure(t *testing.T) {
	cases := []struct {
		name        string
		handedOffAt interface{}
		omitKey     bool
	}{
		{name: "empty_string", handedOffAt: ""},
		{name: "not_rfc3339", handedOffAt: "not-a-timestamp"},
		{name: "wrong_json_type", handedOffAt: 20260101},
		{name: "absent_key", omitKey: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
			payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-unreadable-" + tc.name})

			resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
			require.NoError(t, err)
			result1 := decodeHandoffResult(t, resp1)
			require.True(t, result1.ReverseLinkRecorded)

			clearHandoffsMetadata(t, f.repo, f.sourceTaskID)
			if tc.omitKey {
				setDeliveryHandoffSourceMissingTimestampKey(t, f.svc, result1.TaskID, f.sourceTaskID, f.sourceWorkspaceID, f.sourceSessionID, f.callerAgentProfileID)
			} else {
				setDeliveryHandoffSource(t, f.svc, result1.TaskID, f.sourceTaskID, f.sourceWorkspaceID, f.sourceSessionID, f.callerAgentProfileID, tc.handedOffAt)
			}

			resp2, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
			require.NoError(t, err)
			result2 := decodeHandoffResult(t, resp2)

			assert.Equal(t, handoffOutcomeFoundSettled, result2.Outcome)
			assert.False(t, result2.ReverseLinkRecorded)
			assert.Empty(t, result2.HandedOffAt)
			assert.Contains(t, result2.ReverseLinkError, "unreadable")

			raw, err := f.repo.GetTaskHandoffsRaw(context.Background(), f.sourceTaskID)
			require.NoError(t, err)
			assert.Equal(t, "[]", raw, "an unreadable timestamp must not be written")
		})
	}
}

// --- AC-25b precedence: AC-25a's source check wins before readability is ever checked ---

func TestHandleHandoffTask_CrossSourceMismatchTakesPrecedenceOverUnreadableTimestamp(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-precedence-source"})

	resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)

	setDeliveryHandoffSource(t, f.svc, result1.TaskID, "task-not-the-real-source", f.sourceWorkspaceID, f.sourceSessionID, f.callerAgentProfileID, "not-a-timestamp")

	resp2, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp2, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp2, "did not hand off")
}

// --- AC-25b precedence: presence is checked before readability ---

func TestHandleHandoffTask_ExistingReverseLinkEntryRecordedDespiteUnreadableStoredTimestamp(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-precedence-presence"})

	resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)
	require.True(t, result1.ReverseLinkRecorded, "precondition: the entry must already exist in the source's handoffs array")

	setDeliveryHandoffSource(t, f.svc, result1.TaskID, f.sourceTaskID, f.sourceWorkspaceID, f.sourceSessionID, f.callerAgentProfileID, "not-a-timestamp")

	resp2, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result2 := decodeHandoffResult(t, resp2)

	assert.True(t, result2.ReverseLinkRecorded, "presence must be checked before readability")
	assert.Empty(t, result2.ReverseLinkError)
	assert.Empty(t, result2.HandedOffAt, "an unreadable stored timestamp cannot be echoed back, even though the entry is already recorded")
}

// --- AC-28: a repaired/appended entry sorts chronologically, not always last ---

func TestHandleHandoffTask_ReverseLinkAppendSortsChronologicallyNotAlwaysLast(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const laterID = "delivery-task-in-the-future"
	seeded := fmt.Sprintf(`[{"task_id":%q,"target_workspace_id":%q,"handed_off_at":"2030-01-01T00:00:00.000Z"}]`,
		laterID, f.targetWorkspaceID)
	stored, _, err := f.repo.SetTaskHandoffsIfUnchanged(context.Background(), f.sourceTaskID, "", seeded)
	require.NoError(t, err)
	require.True(t, stored)

	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)
	require.True(t, result.ReverseLinkRecorded)

	raw, err := f.repo.GetTaskHandoffsRaw(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	var entries []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	require.Len(t, entries, 2)
	assert.Equal(t, result.TaskID, entries[0][keyTaskID],
		"the chronologically-earlier new entry must sort before the pre-seeded future entry")
	assert.Equal(t, laterID, entries[1][keyTaskID])
}
