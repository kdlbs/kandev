package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// This fixture never supplies a runtime or sends a prompt. The
// existing fake LaunchAgent seam observes real durable prepare, then refuses
// attach so the production path cannot promote or stop the source runtime.
var errAllocationAttachBoundary = errors.New("WorkflowAllocation synthetic external attach boundary")

func commitProfileSwitchEntry(t *testing.T, fixture *profileSwitchFixture, stepID string) {
	t.Helper()
	task, err := fixture.repo.GetTask(t.Context(), "t1")
	require.NoError(t, err)
	task.WorkflowStepID = stepID
	require.NoError(t, fixture.repo.UpdateTask(t.Context(), task))
}

type allocationHarness struct {
	f              *profileSwitchFixture
	target, source *wfmodels.WorkflowStep
	entry          int64
	entered        chan string
	release        chan struct{}
	once           sync.Once
	wg             sync.WaitGroup
	attach         atomic.Int64
}

func allocationFixture(t *testing.T) *allocationHarness {
	t.Helper()
	f := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	h := &allocationHarness{f: f, entered: make(chan string, 8), release: make(chan struct{})}
	t.Cleanup(func() { h.once.Do(func() { close(h.release) }); h.wg.Wait() })
	f.svc.messageQueue = nil
	f.svc.initWorkflowEngine()
	h.target = &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", AgentProfileID: "profile-b", ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew, Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}
	h.source = &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", AgentProfileID: "profile-a", ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark}
	f.stepGetter.steps[h.target.ID] = h.target
	f.stepGetter.steps[h.source.ID] = h.source
	f.current.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, f.repo.UpdateTaskSession(t.Context(), f.current))
	task, err := f.repo.GetTask(t.Context(), "t1")
	require.NoError(t, err)
	task.WorkflowStepID = h.target.ID
	require.NoError(t, f.repo.UpdateTask(t.Context(), task))
	h.entry, err = f.repo.GetLatestTaskStepTransitionID(t.Context(), "t1")
	require.NoError(t, err)
	require.Positive(t, h.entry)
	f.agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		h.attach.Add(1)
		h.entered <- req.SessionID
		<-h.release
		return nil, errAllocationAttachBoundary
	}
	return h
}

func (h *allocationHarness) attempt() <-chan error {
	result := make(chan error, 1)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		err := h.dispatch()
		result <- err
	}()
	return result
}

func (h *allocationHarness) dispatch() error {
	return switchWorkflowDispatcher(h.f.svc)(context.Background(), "t1", h.f.current.ID, engine.TriggerOnEnter, fmt.Sprintf("allocation:accepted-entry:%d", h.entry), h.source.ID)
}

func allocationReceive[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("test-owned barrier did not finish")
		var zero T
		return zero
	}
}

func (h *allocationHarness) unblock() { h.once.Do(func() { close(h.release) }) }

func (h *allocationHarness) snapshot(t *testing.T, label string) ([]*models.TaskSession, models.WorkflowSessionRoute) {
	t.Helper()
	ctx := t.Context()
	rows, err := h.f.repo.ListTaskSessions(ctx, "t1")
	require.NoError(t, err)
	task, err := h.f.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	route, found := models.LoadWorkflowSessionRoute(task.Metadata)
	require.True(t, found)
	type row struct {
		ID                 string
		State              models.TaskSessionState
		Primary            bool
		Turns, UserPrompts int
	}
	observed := []row{}
	users := 0
	for _, s := range rows {
		turns, e := h.f.repo.ListTurnsBySession(ctx, s.ID)
		require.NoError(t, e)
		msgs, e := h.f.repo.ListMessages(ctx, s.ID)
		require.NoError(t, e)
		n := 0
		for _, m := range msgs {
			if m.AuthorType == models.MessageAuthorUser {
				n++
				users++
			}
		}
		observed = append(observed, row{s.ID, s.State, s.IsPrimary, len(turns), n})
	}
	b, err := json.Marshal(struct {
		Label       string
		Entry       int64
		Route       models.WorkflowSessionRoute
		Rows        []row
		AttachCalls int64
	}{label, h.entry, route, observed, h.attach.Load()})
	require.NoError(t, err)
	t.Log(string(b))
	require.Zero(t, users, "harness must never dispatch USER prompts")
	source, err := h.f.repo.GetTaskSession(ctx, h.f.current.ID)
	require.NoError(t, err)
	require.True(t, source.IsPrimary)
	require.Equal(t, models.TaskSessionStateWaitingForInput, source.State)
	return rows, route
}

func TestWorkflowAllocationSameAcceptedEntryDoesNotAllocateTwice(t *testing.T) {
	h := allocationFixture(t)
	first := h.attempt()
	firstID := allocationReceive(t, h.entered)
	before, route := h.snapshot(t, "first owner held after durable prepare")
	require.Len(t, before, 2)
	require.Equal(t, firstID, route.DestinationID)
	dest, err := h.f.repo.GetTaskSession(t.Context(), firstID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCreated, dest.State)
	second := h.attempt()
	select {
	case id := <-h.entered:
		t.Logf("second attach entered while original owner held: %s", id)
	case err := <-second:
		t.Logf("replay returned without attach: %v", err)
		second = nil
	case <-time.After(3 * time.Second):
		t.Log("replay remains pending; release original owner before final assertions")
	}
	held, afterRoute := h.snapshot(t, "same-entry replay while owner held")
	h.unblock()
	require.Error(t, allocationReceive(t, first))
	if second != nil {
		require.Error(t, allocationReceive(t, second))
	}
	h.snapshot(t, "all fake attach attempts joined")
	require.Equal(t, route.OperationID, afterRoute.OperationID)
	require.Len(t, held, 2, "same accepted operation allocated another durable destination")
	require.Equal(t, firstID, afterRoute.DestinationID, "recorded exact destination was overwritten")
}

func TestWorkflowAllocationConcurrentFirstAdmissionCreatesOneDestination(t *testing.T) {
	h := allocationFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			<-start
			err := h.dispatch()
			results <- err
		}()
	}
	close(start)
	allocationReceive(t, h.entered)
	completed := 0
	select {
	case id := <-h.entered:
		t.Logf("competing admission attached independent destination: %s", id)
	case err := <-results:
		require.Error(t, err)
		completed++
	case <-time.After(10 * time.Second):
		t.Fatal("competing admission neither refused nor reached attach")
	}
	rows, _ := h.snapshot(t, "concurrent first admission before release")
	h.unblock()
	for ; completed < 2; completed++ {
		require.Error(t, allocationReceive(t, results))
	}
	h.snapshot(t, "concurrent attempts joined")
	require.Len(t, rows, 2, "one accepted operation must insert one destination across competing admissions")
}

func TestWorkflowAllocationFailedAndLaunchErrorRefusalControls(t *testing.T) {
	for _, launchError := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed_zero_turn", true: "recorded_launch_error"}[launchError], func(t *testing.T) {
			h := allocationFixture(t)
			first := h.attempt()
			id := allocationReceive(t, h.entered)
			h.unblock()
			require.Error(t, allocationReceive(t, first))
			failed, err := h.f.repo.GetTaskSession(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateFailed, failed.State)
			if launchError {
				require.NoError(t, h.f.repo.SetTaskMetadataKey(t.Context(), "t1", models.MetaKeyLastLaunchError, models.TaskLaunchError{Message: "WorkflowAllocation synthetic recorded launch error", OccurredAt: time.Now().UTC(), SessionID: id, Scope: "task", StampValue: "allocation-error"}))
			}
			_, old := h.snapshot(t, "settled failed original")
			replay := h.attempt()
			require.Error(t, allocationReceive(t, replay))
			rows, route := h.snapshot(t, "failed same-operation replay")
			preserved, err := h.f.repo.GetTaskSession(t.Context(), id)
			require.NoError(t, err)
			t.Run("never_revive_failed", func(t *testing.T) {
				require.Equal(t, models.TaskSessionStateFailed, preserved.State)
				turns, e := h.f.repo.ListTurnsBySession(t.Context(), id)
				require.NoError(t, e)
				require.Empty(t, turns)
			})
			require.Equal(t, old.OperationID, route.OperationID)
			require.Len(t, rows, 2, "refusal must not fall through to allocation for the same accepted entry")
		})
	}
}

func TestWorkflowAllocationNewAcceptedEntryRetainsNewSessionSemantics(t *testing.T) {
	h := allocationFixture(t)
	first := h.attempt()
	firstID := allocationReceive(t, h.entered)
	h.unblock()
	require.Error(t, allocationReceive(t, first))
	_, old := h.snapshot(t, "old accepted entry")
	task, err := h.f.repo.GetTask(t.Context(), "t1")
	require.NoError(t, err)
	task.WorkflowStepID = h.source.ID
	require.NoError(t, h.f.repo.UpdateTask(t.Context(), task))
	task, err = h.f.repo.GetTask(t.Context(), "t1")
	require.NoError(t, err)
	task.WorkflowStepID = h.target.ID
	require.NoError(t, h.f.repo.UpdateTask(t.Context(), task))
	h.entry, err = h.f.repo.GetLatestTaskStepTransitionID(t.Context(), "t1")
	require.NoError(t, err)
	next := h.attempt()
	require.Error(t, allocationReceive(t, next))
	rows, route := h.snapshot(t, "new accepted revisit")
	require.Len(t, rows, 3)
	require.NotEqual(t, old.OperationID, route.OperationID)
	require.NotEqual(t, firstID, route.DestinationID)
}

func TestWorkflowAllocationUnknownPreparedOwnerRefusesAllocationAndAttach(t *testing.T) {
	h := allocationFixture(t)
	ctx := t.Context()
	// Seed through the real durable preparation API only. No launch or owner
	// lease is manufactured; absence of an owner is deliberately not death proof.
	task, err := h.f.svc.scheduler.GetTask(ctx, "t1")
	require.NoError(t, err)
	route := workflowProfileSessionRoute("t1", h.f.current, h.target, "profile-b", h.f.svc.workflowEntryIdentity(ctx, "t1", h.entry), models.WorkflowProfileSessionStartPolicyNew)
	id, err := h.f.svc.executor.PrepareSessionForExistingEnvironmentWithWorkflowRoute(ctx, task, "profile-b", h.f.current.ExecutorID, h.f.current.ExecutorProfileID, h.target.ID, h.f.current.TaskEnvironmentID, route)
	require.NoError(t, err)
	before, original := h.snapshot(t, "durable CREATED destination with no proven owner fence")
	require.Len(t, before, 2)
	require.Equal(t, id, original.DestinationID)
	h.unblock()
	err = h.dispatch()
	rows, after := h.snapshot(t, "unknown-owner same-entry admission")
	retained, readErr := h.f.repo.GetTaskSession(ctx, id)
	require.NoError(t, readErr)
	t.Run("original_created_is_not_rearmed", func(t *testing.T) {
		require.Equal(t, models.TaskSessionStateCreated, retained.State)
		require.Empty(t, retained.ErrorMessage)
	})
	require.Error(t, err)
	require.Zero(t, h.attach.Load(), "unknown owner must not admit another attach")
	require.Len(t, rows, 2, "unknown ownership must not cause new allocation")
	require.Equal(t, original, after)
}

func TestWorkflowAllocationAlreadyAppliedDispatcherOperationRefusesReplay(t *testing.T) {
	h := allocationFixture(t)
	require.NoError(t, h.f.svc.workflowStore.MarkOperationApplied(t.Context(), fmt.Sprintf("allocation:accepted-entry:%d", h.entry)))
	require.NoError(t, h.dispatch())
	rows, err := h.f.repo.ListTaskSessions(t.Context(), "t1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Zero(t, h.attach.Load())
	task, err := h.f.repo.GetTask(t.Context(), "t1")
	require.NoError(t, err)
	_, found := models.LoadWorkflowSessionRoute(task.Metadata)
	require.False(t, found)
}
