package orchestrator

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// nopLogger returns a *logger.Logger backed by zap.NewNop() for use in tests.
// (The shared logger package does not expose a NewNop helper today.)
func nopLogger(t *testing.T) *logger.Logger {
	t.Helper()
	l, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("failed to build nop logger: %v", err)
	}
	return l
}

// fakeWatcherSource is a configurable test double for WatcherSource. Tests
// flip the public fields to drive coordinator branches; recorded* fields let
// the test assert which coordinator stages actually ran.
type fakeWatcherSource struct {
	name             string
	reserveOK        bool
	reserveErr       error
	buildReq         *IssueTaskRequest
	buildErr         error
	attachErr        error
	terminalAttach   bool
	autoStart        AutoStartParams
	watchID          string
	metadataKey      string
	maxInflightTasks *int
	recordedReserve  int
	recordedRelease  int
	recordedBuild    int
	recordedAttach   int
	recordedAutoArgs *AutoStartParams
}

func (f *fakeWatcherSource) Name() string { return f.name }

func (f *fakeWatcherSource) Reserve(_ context.Context, _ any) (bool, error) {
	f.recordedReserve++
	return f.reserveOK, f.reserveErr
}

func (f *fakeWatcherSource) Release(_ context.Context, _ any) {
	f.recordedRelease++
}

func (f *fakeWatcherSource) BuildTaskRequest(_ any) (*IssueTaskRequest, error) {
	f.recordedBuild++
	return f.buildReq, f.buildErr
}

func (f *fakeWatcherSource) AttachTaskID(_ context.Context, _ any, _ string) error {
	f.recordedAttach++
	return f.attachErr
}

func (f *fakeWatcherSource) IsTerminalAttachError(error) bool { return f.terminalAttach }

func (f *fakeWatcherSource) AutoStartParams(_ any) AutoStartParams {
	p := f.autoStart
	f.recordedAutoArgs = &p
	return p
}

func (f *fakeWatcherSource) WatchID(_ any) string { return f.watchID }

func (f *fakeWatcherSource) MaxInflightTasks(_ any) *int { return f.maxInflightTasks }

func (f *fakeWatcherSource) WatchMetadataKey() string { return f.metadataKey }

// AgentProfileID returns "" so the pre-flight check skips and behaviour
// matches pre-self-heal semantics for tests that don't wire a ProfileLookup.
func (f *fakeWatcherSource) AgentProfileID(_ any) string { return "" }

func (f *fakeWatcherSource) SelfHeal(_ context.Context, _ any, _ string) error { return nil }

// fakeTaskCreator captures the request and returns a canned task or error.
type fakeTaskCreator struct {
	createErr error
	gotReq    *IssueTaskRequest
	returned  *models.Task
}

func (f *fakeTaskCreator) CreateIssueTask(_ context.Context, req *IssueTaskRequest) (*models.Task, error) {
	f.gotReq = req
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.returned == nil {
		f.returned = &models.Task{ID: "task-1"}
	}
	return f.returned, nil
}

func newTestCoordinator(t *testing.T, tc *fakeTaskCreator, autoStart bool, starter taskStarter) *WatcherDispatchCoordinator {
	t.Helper()
	return &WatcherDispatchCoordinator{
		taskCreator: tc,
		logger:      nopLogger(t),
		shouldAutoStart: func(_ context.Context, _ string) bool {
			return autoStart
		},
		startTask: starter,
	}
}

// taskStarter is the indirection used inside Coordinator so tests can
// inspect whether StartTask was invoked.
type fakeTaskStarter struct {
	called    int
	gotID     string
	gotStep   string
	gotPrompt string
	err       error
}

func (f *fakeTaskStarter) Start(_ context.Context, taskID, stepID, prompt string, _ AutoStartParams) error {
	f.called++
	f.gotID = taskID
	f.gotStep = stepID
	f.gotPrompt = prompt
	return f.err
}

func TestCoordinator_Dispatch_HappyPath(t *testing.T) {
	src := &fakeWatcherSource{
		name:      "linear",
		reserveOK: true,
		buildReq: &IssueTaskRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf-1",
			WorkflowStepID: "step-1",
			Title:          "[ENG-1] Hello",
			Description:    "body",
		},
		autoStart: AutoStartParams{
			AgentProfileID:    "agent-1",
			ExecutorProfileID: "exec-1",
			WorkflowStepID:    "step-1",
		},
	}
	// returned.Description must drive auto-start prompt; if the created
	// task's description ever diverges from the request's (e.g. service-
	// side normalisation), auto-start MUST use the persisted body — pin
	// that contract here.
	tc := &fakeTaskCreator{returned: &models.Task{ID: "task-1", Description: "persisted body"}}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	if src.recordedReserve != 1 || src.recordedBuild != 1 || src.recordedAttach != 1 {
		t.Fatalf("expected reserve/build/attach each once, got R=%d B=%d A=%d",
			src.recordedReserve, src.recordedBuild, src.recordedAttach)
	}
	if src.recordedRelease != 0 {
		t.Fatalf("expected no release on happy path, got %d", src.recordedRelease)
	}
	if tc.gotReq == nil || tc.gotReq.Title != "[ENG-1] Hello" {
		t.Fatalf("expected task creator to receive built request, got %+v", tc.gotReq)
	}
	if starter.called != 1 {
		t.Fatalf("expected auto-start to fire once, got %d", starter.called)
	}
	if starter.gotID != "task-1" || starter.gotStep != "step-1" {
		t.Fatalf("auto-start received wrong args: id=%s step=%s", starter.gotID, starter.gotStep)
	}
	if starter.gotPrompt != "persisted body" {
		t.Fatalf("auto-start prompt must come from created task.Description, got %q", starter.gotPrompt)
	}
}

func TestCoordinator_Dispatch_ReserveLost(t *testing.T) {
	src := &fakeWatcherSource{name: "linear", reserveOK: false}
	tc := &fakeTaskCreator{}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	if src.recordedReserve != 1 {
		t.Fatalf("expected reserve to be attempted, got %d", src.recordedReserve)
	}
	if src.recordedBuild != 0 || src.recordedAttach != 0 || starter.called != 0 || tc.gotReq != nil {
		t.Fatalf("expected pipeline to stop after reserve=false, got B=%d A=%d Start=%d gotReq=%v",
			src.recordedBuild, src.recordedAttach, starter.called, tc.gotReq)
	}
}

func TestCoordinator_Dispatch_ReserveError(t *testing.T) {
	src := &fakeWatcherSource{name: "linear", reserveOK: false, reserveErr: errors.New("db down")}
	tc := &fakeTaskCreator{}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	if src.recordedBuild != 0 || tc.gotReq != nil {
		t.Fatal("expected pipeline to stop after reserve error")
	}
}

func TestCoordinator_Dispatch_BuildError_Releases(t *testing.T) {
	src := &fakeWatcherSource{name: "linear", reserveOK: true, buildErr: errors.New("bad payload")}
	tc := &fakeTaskCreator{}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	if src.recordedRelease != 1 {
		t.Fatalf("expected release after build error, got %d", src.recordedRelease)
	}
	if tc.gotReq != nil || starter.called != 0 {
		t.Fatal("expected pipeline to stop after build error")
	}
}

func TestCoordinator_Dispatch_CreateError_Releases(t *testing.T) {
	src := &fakeWatcherSource{
		name:      "linear",
		reserveOK: true,
		buildReq:  &IssueTaskRequest{WorkspaceID: "ws-1"},
	}
	tc := &fakeTaskCreator{createErr: errors.New("write failed")}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	if src.recordedRelease != 1 {
		t.Fatalf("expected release after create error, got %d", src.recordedRelease)
	}
	if src.recordedAttach != 0 || starter.called != 0 {
		t.Fatal("expected pipeline to stop after create error")
	}
}

func TestCoordinator_Dispatch_NoAutoStart(t *testing.T) {
	src := &fakeWatcherSource{
		name:      "linear",
		reserveOK: true,
		buildReq:  &IssueTaskRequest{WorkspaceID: "ws-1", WorkflowStepID: "step-1"},
	}
	tc := &fakeTaskCreator{}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, false, starter)

	c.Dispatch(context.Background(), src, "evt")

	if src.recordedAttach != 1 {
		t.Fatalf("expected attach to run, got %d", src.recordedAttach)
	}
	if starter.called != 0 {
		t.Fatalf("expected no auto-start when shouldAutoStart=false, got %d", starter.called)
	}
}

func TestCoordinator_Dispatch_AttachError_LogsButContinues(t *testing.T) {
	src := &fakeWatcherSource{
		name:      "linear",
		reserveOK: true,
		buildReq:  &IssueTaskRequest{WorkspaceID: "ws-1", WorkflowStepID: "step-1"},
		attachErr: errors.New("attach failed"),
	}
	tc := &fakeTaskCreator{}
	starter := &fakeTaskStarter{}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	// Today's createLinearIssueTask logs the attach failure but still auto-starts;
	// preserve that behaviour exactly.
	if starter.called != 1 {
		t.Fatalf("expected auto-start to still run after attach error, got %d", starter.called)
	}
	// Attach failure is best-effort: the task was already created, so we
	// MUST NOT Release the dedup row — that would orphan the task.
	if src.recordedRelease != 0 {
		t.Fatalf("expected no Release after attach error, got %d", src.recordedRelease)
	}
}

func TestCoordinator_Dispatch_TerminalAttachErrorSkipsAutoStart(t *testing.T) {
	src := &fakeWatcherSource{
		name:           "gitlab_review",
		reserveOK:      true,
		buildReq:       &IssueTaskRequest{WorkspaceID: "ws-1", WorkflowStepID: "step-1"},
		attachErr:      errors.New("watch ownership lost"),
		terminalAttach: true,
	}
	taskCreator := &fakeTaskCreator{}
	starter := &fakeTaskStarter{}
	coordinator := newTestCoordinator(t, taskCreator, true, starter)

	coordinator.Dispatch(context.Background(), src, "evt")

	if starter.called != 0 {
		t.Fatalf("auto-start called %d times after terminal attach error", starter.called)
	}
	if src.recordedRelease != 0 {
		t.Fatalf("terminal ownership loss released replacement reservation %d times", src.recordedRelease)
	}
}

func TestCoordinator_Dispatch_AutoStartError_LogsAndReturns(t *testing.T) {
	src := &fakeWatcherSource{
		name:      "linear",
		reserveOK: true,
		buildReq:  &IssueTaskRequest{WorkspaceID: "ws-1", WorkflowStepID: "step-1"},
	}
	tc := &fakeTaskCreator{}
	starter := &fakeTaskStarter{err: errors.New("start failed")}
	c := newTestCoordinator(t, tc, true, starter)

	c.Dispatch(context.Background(), src, "evt")

	if starter.called != 1 {
		t.Fatalf("expected auto-start to be attempted, got %d", starter.called)
	}
	// Auto-start failure is terminal: the task exists and the dedup row is
	// already attached. We must not Release.
	if src.recordedRelease != 0 {
		t.Fatalf("expected no Release after auto-start error, got %d", src.recordedRelease)
	}
}
