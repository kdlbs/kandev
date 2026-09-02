package handlers

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// alwaysStaleHandoffsRepo wraps the real repository and reports every
// SetTaskHandoffsIfUnchanged call as a stale-value mismatch, regardless of
// the actual stored value, so ensureHandoffReverseLink's 5-attempt retry
// loop is forced to exhaust deterministically (AC-27's bounded-retry clause,
// AC-32's exhaustion clause) without racing a real concurrent writer.
type alwaysStaleHandoffsRepo struct {
	*sqliterepo.Repository
	mu       sync.Mutex
	setCalls int
}

func (r *alwaysStaleHandoffsRepo) SetTaskHandoffsIfUnchanged(
	_ context.Context, _, expectedHandoffsJSON, _ string,
) (bool, string, error) {
	r.mu.Lock()
	r.setCalls++
	r.mu.Unlock()
	return false, expectedHandoffsJSON, nil
}

func (r *alwaysStaleHandoffsRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setCalls
}

// --- AC-26a: two concurrent calls sharing one external_id ---

func TestHandleHandoffTask_ConcurrentSameExternalIDProducesOneTaskBothCallersGetIt(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-same-source-race"})
	msg := makeWSMessage(t, ws.ActionMCPHandoffTask, payload)

	responses := make([]*ws.Message, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		go func(i int) {
			defer wg.Done()
			responses[i], errs[i] = f.h.handleHandoffTask(f.ctx(), msg)
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	result0 := decodeHandoffResult(t, responses[0])
	result1 := decodeHandoffResult(t, responses[1])

	assert.Equal(t, result0.TaskID, result1.TaskID, "both callers must receive the one task id")
	outcomes := []string{result0.Outcome, result1.Outcome}
	assert.Contains(t, outcomes, handoffOutcomeCreated, "exactly one caller must observe created")
	for _, o := range outcomes {
		assert.Contains(t,
			[]string{handoffOutcomeCreated, handoffOutcomeFoundSettled, handoffOutcomeFoundUnsettled}, o,
			"neither caller may receive an error outcome on account of losing the race")
	}
	assert.Equal(t, 1, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID))

	sourceTask, err := f.svc.GetTask(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	handoffs, ok := sourceTask.Metadata[models.MetaKeyHandoffs].([]interface{})
	require.True(t, ok)
	assert.Len(t, handoffs, 1, "at-most-once even under a same-external-id race")
}

// --- AC-27: two concurrent handoffs from the same source, both must appear ---

func TestHandleHandoffTask_ConcurrentDifferentHandoffsFromSameSourceBothAppear(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	payloadA := f.validPayload(map[string]interface{}{handoffFieldTitle: "Deliver A"})
	payloadB := f.validPayload(map[string]interface{}{handoffFieldTitle: "Deliver B"})
	msgA := makeWSMessage(t, ws.ActionMCPHandoffTask, payloadA)
	msgB := makeWSMessage(t, ws.ActionMCPHandoffTask, payloadB)

	var respA, respB *ws.Message
	var errA, errB error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		respA, errA = f.h.handleHandoffTask(f.ctx(), msgA)
	}()
	go func() {
		defer wg.Done()
		respB, errB = f.h.handleHandoffTask(f.ctx(), msgB)
	}()
	wg.Wait()

	require.NoError(t, errA)
	require.NoError(t, errB)
	resultA := decodeHandoffResult(t, respA)
	resultB := decodeHandoffResult(t, respB)
	require.NotEqual(t, resultA.TaskID, resultB.TaskID, "two distinct deliveries must produce two distinct tasks")
	assert.True(t, resultA.ReverseLinkRecorded, "neither concurrent append may be lost")
	assert.True(t, resultB.ReverseLinkRecorded, "neither concurrent append may be lost")

	sourceTask, err := f.svc.GetTask(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	handoffs, ok := sourceTask.Metadata[models.MetaKeyHandoffs].([]interface{})
	require.True(t, ok)
	require.Len(t, handoffs, 2, "both concurrent appends must survive")
	ids := map[string]bool{}
	for _, raw := range handoffs {
		entry := raw.(map[string]interface{})
		ids[entry[keyTaskID].(string)] = true
	}
	assert.True(t, ids[resultA.TaskID])
	assert.True(t, ids[resultB.TaskID])
}

// --- AC-27: a concurrent write to a different metadata key must not spuriously conflict ---

func TestHandleHandoffTask_ConcurrentWriteToDifferentMetadataKeyDoesNotConflict(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	msg := makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil))

	var resp *ws.Message
	var handleErr, writeErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		resp, handleErr = f.h.handleHandoffTask(f.ctx(), msg)
	}()
	go func() {
		defer wg.Done()
		_, _, writeErr = f.repo.SetTaskMetadataKeyIfDifferentStamp(
			context.Background(), f.sourceTaskID, "some_other_key", "distinct-stamp", "bar")
	}()
	wg.Wait()

	require.NoError(t, handleErr)
	require.NoError(t, writeErr)
	result := decodeHandoffResult(t, resp)
	assert.True(t, result.ReverseLinkRecorded, "a write to an unrelated key must not cause a spurious CAS conflict")

	sourceTask, err := f.svc.GetTask(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	handoffs, ok := sourceTask.Metadata[models.MetaKeyHandoffs].([]interface{})
	require.True(t, ok)
	require.Len(t, handoffs, 1, "the write touched only handoffs")
	assert.Equal(t, "bar", sourceTask.Metadata["some_other_key"], "the sibling key's own write must survive too")
}

// --- AC-27: source task missing surfaces the AC-29 partial failure, not a retry loop ---

func TestEnsureHandoffReverseLink_SourceTaskMissingSurfacesPartialFailure(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	result := f.h.ensureHandoffReverseLink(
		context.Background(), "task-does-not-exist", "delivery-task-id", f.targetWorkspaceID,
		"2024-01-01T00:00:00.000Z", false,
	)
	assert.False(t, result.recorded)
	assert.Contains(t, result.errMsg, "source task no longer exists")
}

// --- AC-27: a malformed entry inside an otherwise-valid array is corrupt, not dropped ---

func TestHandleHandoffTask_MalformedEntryInsideHandoffsArrayStillCreatesTask(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const corrupted = `[{"task_id":"existing-delivery","target_workspace_id":"ws-x","handed_off_at":"2024-01-01T00:00:00.000Z"},` +
		`{"task_id":"bad-entry","handed_off_at":"not-a-timestamp"}]`
	stored, _, err := f.repo.SetTaskHandoffsIfUnchanged(context.Background(), f.sourceTaskID, "", corrupted)
	require.NoError(t, err)
	require.True(t, stored, "precondition: seeding the malformed array must succeed")

	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-malformed-entry"})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
	assert.True(t, result.CreationComplete)
	assert.False(t, result.ReverseLinkRecorded)
	assert.NotEmpty(t, result.ReverseLinkError)

	raw, err := f.repo.GetTaskHandoffsRaw(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	assert.Equal(t, corrupted, raw, "malformed data must be left byte-identical, not repaired or dropped")
}

// --- AC-27/AC-32: the CAS retry loop exhausts after 5 attempts, and the
// launch still proceeds independently of the reverse link ---

func TestHandleHandoffTask_ReverseLinkCASExhaustionStillStartsAgent(t *testing.T) {
	launcher := newHandoffFakeLauncher(nil)
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)
	staleRepo := &alwaysStaleHandoffsRepo{Repository: f.repo}
	f.h.taskRepo = staleRepo

	payload := f.validPayload(map[string]interface{}{handoffFieldStartAgent: true})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
	assert.True(t, result.CreationComplete)
	assert.True(t, result.Started, "the launch must not be blocked by a reverse-link conflict")
	assert.Empty(t, result.StartError)
	assert.False(t, result.ReverseLinkRecorded)
	assert.Contains(t, result.ReverseLinkError, "conflicted repeatedly across 5 attempts")
	assert.Equal(t, 5, staleRepo.callCount(), "the retry loop must be bounded at exactly 5 attempts")
}
