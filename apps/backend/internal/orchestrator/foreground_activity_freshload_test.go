package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	taskdto "github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Fresh-load / second-tab correctness (§spec:live-propagation-fallback): the boot
// payload and the session/task DTOs must carry the current substate so first paint
// is right WITHOUT waiting for a transition — and, after a restart, an untracked
// in-flight session must serialize a not-done reading, never done. boot_state.go
// stamps the wire with exactly EnrichForegroundActivity / EnrichTaskForegroundActivity
// and marshals the DTOs with their json tags, so enrich-then-marshal against the real
// orchestrator provider is a faithful proxy for what a fresh page / second tab reads.

// TestFreshLoad_UntrackedRunningSessionSerializesGenerating proves a freshly
// fetched RUNNING session whose substate the tracker never saw (post-restart)
// puts "generating" on the wire at both the session and task-aggregate level, so
// a first paint / second tab reads working, not done.
func TestFreshLoad_UntrackedRunningSessionSerializesGenerating(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-freshload"

	sessionDTO := &taskdto.TaskSessionDTO{ID: sessionID, State: models.TaskSessionStateRunning}
	taskdto.EnrichForegroundActivity(sessionDTO, svc)
	if got := marshalField(t, sessionDTO); got != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("session wire foreground_activity: got %q, want generating", got)
	}

	taskDTO := &taskdto.TaskDTO{ID: "task-freshload"}
	sessions := []*models.TaskSession{{ID: sessionID, State: models.TaskSessionStateRunning}}
	taskdto.EnrichTaskForegroundActivity(taskDTO, sessions, svc)
	if got := marshalField(t, taskDTO); got != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("task wire foreground_activity: got %q, want generating", got)
	}
}

// TestFreshLoad_SettledSessionOmitsSubstate proves the flip side: a non-RUNNING
// session (and a task with no running session) must NOT fabricate a substate, so
// a genuinely-done task reads done on fresh load. omitempty drops the key entirely.
func TestFreshLoad_SettledSessionOmitsSubstate(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	sessionDTO := &taskdto.TaskSessionDTO{ID: "session-done", State: models.TaskSessionStateCompleted}
	taskdto.EnrichForegroundActivity(sessionDTO, svc)
	if body := marshalDTO(t, sessionDTO); strings.Contains(body, "foreground_activity") {
		t.Fatalf("settled session must omit foreground_activity on the wire, got %s", body)
	}

	taskDTO := &taskdto.TaskDTO{ID: "task-done"}
	sessions := []*models.TaskSession{{ID: "session-done", State: models.TaskSessionStateCompleted}}
	taskdto.EnrichTaskForegroundActivity(taskDTO, sessions, svc)
	if body := marshalDTO(t, taskDTO); strings.Contains(body, "foreground_activity") {
		t.Fatalf("task with no running session must omit foreground_activity, got %s", body)
	}
}

func TestFreshLoad_SettledSessionWithDetachedWorkSerializesBackground(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-detached"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)
	sessionDTO := &taskdto.TaskSessionDTO{ID: sessionID, State: models.TaskSessionStateWaitingForInput}
	taskdto.EnrichForegroundActivity(sessionDTO, svc)
	if got := marshalField(t, sessionDTO); got != string(v1.ForegroundActivityBackground) {
		t.Fatalf("settled detached session foreground_activity: got %q, want background", got)
	}

	taskDTO := &taskdto.TaskDTO{ID: "task-detached"}
	sessions := []*models.TaskSession{{ID: sessionID, State: models.TaskSessionStateWaitingForInput}}
	taskdto.EnrichTaskForegroundActivity(taskDTO, sessions, svc)
	if got := marshalField(t, taskDTO); got != string(v1.ForegroundActivityBackground) {
		t.Fatalf("detached task foreground_activity: got %q, want background", got)
	}
}

// marshalField marshals the DTO and returns the string value of foreground_activity.
func marshalField(t *testing.T, dto any) string {
	t.Helper()
	var wire map[string]json.RawMessage
	if err := json.Unmarshal([]byte(marshalDTO(t, dto)), &wire); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	raw, ok := wire["foreground_activity"]
	if !ok {
		t.Fatalf("foreground_activity absent from wire: %v", wire)
	}
	var val string
	if err := json.Unmarshal(raw, &val); err != nil {
		t.Fatalf("decode foreground_activity: %v", err)
	}
	return val
}

func marshalDTO(t *testing.T, dto any) string {
	t.Helper()
	body, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal dto: %v", err)
	}
	return string(body)
}
