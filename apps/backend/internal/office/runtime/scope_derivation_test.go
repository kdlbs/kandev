package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

type stubRunnerLister struct {
	ids   []string
	total int
	err   error
	calls int

	// gotAgentID, gotWorkspaceID, and gotCapAt record the arguments passed to
	// the most recent call, so tests can assert ListRunnerSetTaskIDs is
	// invoked with the run's own agent/workspace and the package's cap
	// rather than some other value that happens to still compile.
	gotAgentID     string
	gotWorkspaceID string
	gotCapAt       int
}

func (s *stubRunnerLister) ListRunnerSetTaskIDs(
	_ context.Context, agentID, workspaceID string, capAt int,
) ([]string, int, error) {
	s.calls++
	s.gotAgentID = agentID
	s.gotWorkspaceID = workspaceID
	s.gotCapAt = capAt
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.ids, s.total, nil
}

func taskAgent() *models.AgentInstance {
	return &models.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1", Role: models.AgentRoleCEO}
}

func TestBuild_TaskBoundRunScopesToTrimmedPayloadTaskID(t *testing.T) {
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{"task_id":"  task-1  "}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if runCtx.TaskID != "task-1" {
		t.Fatalf("TaskID = %q, want trimmed task-1", runCtx.TaskID)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourcePayload {
		t.Fatalf("TaskScopeSource = %q, want payload", runCtx.Capabilities.TaskScopeSource)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 1 || runCtx.Capabilities.AllowedTaskIDs[0] != "task-1" {
		t.Fatalf("AllowedTaskIDs = %v, want [task-1]", runCtx.Capabilities.AllowedTaskIDs)
	}
}

func TestBuild_WhitespaceOnlyPayloadTaskIDIsTaskless(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{"task_id":"   "}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if runCtx.TaskID != "" {
		t.Fatalf("TaskID = %q, want empty (taskless)", runCtx.TaskID)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourceRunnerSet {
		t.Fatalf("TaskScopeSource = %q, want runner_set", runCtx.Capabilities.TaskScopeSource)
	}
}

func TestBuild_TasklessRunMaterializesRunnerSet(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1", "t2"}, total: 2}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := runCtx.Capabilities.AllowedTaskIDs; len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Fatalf("AllowedTaskIDs = %v, want [t1 t2]", got)
	}
	if lister.calls != 1 {
		t.Fatalf("expected 1 runner-set query, got %d", lister.calls)
	}
	if lister.gotAgentID != "agent-1" {
		t.Fatalf("gotAgentID = %q, want %q — the query must scope to the run's own agent, "+
			"not an arbitrary one", lister.gotAgentID, "agent-1")
	}
	if lister.gotWorkspaceID != "ws-1" {
		t.Fatalf("gotWorkspaceID = %q, want %q — the query must scope to the agent's own "+
			"workspace, not an arbitrary one", lister.gotWorkspaceID, "ws-1")
	}
	if lister.gotCapAt != scopeRunnerSetCap {
		t.Fatalf("gotCapAt = %d, want %d — the query must use the package's runner-set cap",
			lister.gotCapAt, scopeRunnerSetCap)
	}
}

func TestBuild_NilListerYieldsUnavailableProvisionalScope(t *testing.T) {
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 0 {
		t.Fatalf("AllowedTaskIDs = %v, want empty", runCtx.Capabilities.AllowedTaskIDs)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourceUnavailable {
		t.Fatalf("TaskScopeSource = %q, want unavailable", runCtx.Capabilities.TaskScopeSource)
	}
}

func TestBuild_EmptyRunWorkspaceIssuesNoQuery(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: &models.AgentInstance{ID: "agent-1", WorkspaceID: ""}},
		RunnerLister: lister,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if lister.calls != 0 {
		t.Fatalf("expected no query for empty workspace, got %d calls", lister.calls)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourceUnavailable {
		t.Fatalf("TaskScopeSource = %q, want unavailable", runCtx.Capabilities.TaskScopeSource)
	}
}

func TestBuild_RunnerSetQueryErrorFailsClosed(t *testing.T) {
	lister := &stubRunnerLister{err: errors.New("db down")}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 0 {
		t.Fatalf("AllowedTaskIDs = %v, want empty on query failure", runCtx.Capabilities.AllowedTaskIDs)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourceUnavailable {
		t.Fatalf("TaskScopeSource = %q, want unavailable", runCtx.Capabilities.TaskScopeSource)
	}
}

// TestBuild_WildcardPayloadTaskIDStaysTaskBoundWithEmptyScope is the Review
// round 3 regression test (R3REV-03). A run injecting task_id="*" must NOT
// be reclassified as taskless: doing so would grant it the runner-set
// mutation scope (up to 500 tasks) plus workspace-wide annotation, which is
// strictly MORE authority than the correct outcome — staying task-bound
// with an empty, permanently-non-matching scope, since WithTaskScope
// already strips the sentinel from AllowedTaskIDs. No AC sanctions
// promoting a wildcard payload to the wider taskless class; AC-003.10 only
// forbids assigning the wildcard scope, which the fail-closed empty scope
// here satisfies without needing the reclassification.
func TestBuild_WildcardPayloadTaskIDStaysTaskBoundWithEmptyScope(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{"task_id":"*"}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourcePayload {
		t.Fatalf(
			"TaskScopeSource = %q, want %q — a wildcard payload must stay task-bound, not fall "+
				"through to the taskless runner-set derivation",
			runCtx.Capabilities.TaskScopeSource, TaskScopeSourcePayload,
		)
	}
	if lister.calls != 0 {
		t.Fatalf("expected no runner-set query for a wildcard payload task_id, got %d calls", lister.calls)
	}
	for _, id := range runCtx.Capabilities.AllowedTaskIDs {
		if id == WildcardTaskScope {
			t.Fatalf(
				"a wildcard payload task_id must never grant workspace-wide mutation authority: %v",
				runCtx.Capabilities.AllowedTaskIDs,
			)
		}
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 0 {
		t.Fatalf("AllowedTaskIDs = %v, want empty — the stripped wildcard must not be replaced "+
			"by any wider grant", runCtx.Capabilities.AllowedTaskIDs)
	}
	if runCtx.CanMutateTask("some-other-task") {
		t.Fatal("a run injecting task_id=\"*\" must not gain authority over an arbitrary task")
	}
	if runCtx.CanMutateTask("t1") {
		t.Fatal("a run injecting task_id=\"*\" must not gain authority over the agent's own runner-set tasks either")
	}
}

func TestBuild_RunnerSetScopeExcludesWildcard(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.Build(context.Background(), run)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, id := range runCtx.Capabilities.AllowedTaskIDs {
		if id == WildcardTaskScope {
			t.Fatalf("taskless run must never receive the wildcard scope: %v", runCtx.Capabilities.AllowedTaskIDs)
		}
	}
}

// -- BuildAndPersist / snapshot reuse --

func TestBuildAndPersist_FinalRunnerSetMarkerIsReusedWithoutQuery(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"fresh"}, total: 1}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
	}
	persistedCaps, err := MarshalCapabilities(Capabilities{
		AllowedTaskIDs:  []string{"persisted-1", "persisted-2"},
		TaskScopeSource: TaskScopeSourceRunnerSet,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`, Capabilities: persistedCaps}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if lister.calls != 0 {
		t.Fatalf("expected no runner-set query on reuse, got %d", lister.calls)
	}
	got := runCtx.Capabilities.AllowedTaskIDs
	if len(got) != 2 || got[0] != "persisted-1" || got[1] != "persisted-2" {
		t.Fatalf("AllowedTaskIDs = %v, want reused persisted scope", got)
	}
}

func TestBuildAndPersist_ProvisionalUnavailableMarkerDerivesAgain(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"fresh"}, total: 1}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
	}
	persistedCaps, err := MarshalCapabilities(Capabilities{TaskScopeSource: TaskScopeSourceUnavailable})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`, Capabilities: persistedCaps}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("expected a fresh runner-set query, got %d calls", lister.calls)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 1 || runCtx.Capabilities.AllowedTaskIDs[0] != "fresh" {
		t.Fatalf("AllowedTaskIDs = %v, want [fresh]", runCtx.Capabilities.AllowedTaskIDs)
	}
}

func TestBuildAndPersist_UnrecognizedMarkerDerivesFresh(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"fresh"}, total: 1}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
	}
	run := &models.Run{
		ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`,
		Capabilities: `{"task_scope_source":"something-else"}`,
	}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("expected a fresh derive for an unrecognized marker, got %d", lister.calls)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 1 || runCtx.Capabilities.AllowedTaskIDs[0] != "fresh" {
		t.Fatalf("AllowedTaskIDs = %v, want [fresh]", runCtx.Capabilities.AllowedTaskIDs)
	}
}

func TestBuildAndPersist_UnparseableSnapshotDerivesFresh(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"fresh"}, total: 1}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`, Capabilities: `not json`}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("expected a fresh derive for unparseable capabilities, got %d", lister.calls)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 1 || runCtx.Capabilities.AllowedTaskIDs[0] != "fresh" {
		t.Fatalf("AllowedTaskIDs = %v, want [fresh]", runCtx.Capabilities.AllowedTaskIDs)
	}
}

func TestBuildAndPersist_CapabilityBooleansAlwaysRederiveOnReuse(t *testing.T) {
	lister := &stubRunnerLister{}
	store := &recordingRunSnapshotStore{casWins: true}
	agent := taskAgent()
	agent.Permissions = `{"can_create_tasks":true}`
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: agent}, Runs: store, RunnerLister: lister}
	persistedCaps, err := MarshalCapabilities(Capabilities{
		AllowedTaskIDs:  []string{"t1"},
		TaskScopeSource: TaskScopeSourceRunnerSet,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`, Capabilities: persistedCaps}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if !runCtx.Capabilities.Allows(CapabilityCreateAgent) {
		t.Fatal("expected capability booleans to re-derive from the agent even on scope reuse")
	}
}

func TestBuildAndPersist_TruncationEmitsScopeTruncatedEvent(t *testing.T) {
	ids := make([]string, scopeRunnerSetCap)
	for i := range ids {
		ids[i] = "t"
	}
	lister := &stubRunnerLister{ids: ids, total: scopeRunnerSetCap + 7}
	store := &recordingRunSnapshotStore{casWins: true}
	events := &recordingRunEvents{}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
		ScopeEvents:  events,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	if _, err := builder.BuildAndPersist(context.Background(), run); err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if len(events.events) != 1 || events.events[0].eventType != "runtime.scope_truncated" {
		t.Fatalf("events = %+v, want one runtime.scope_truncated", events.events)
	}
	if events.events[0].payload["total"] != scopeRunnerSetCap+7 {
		t.Fatalf("payload total = %v, want %d", events.events[0].payload["total"], scopeRunnerSetCap+7)
	}
}

// TestBuildAndPersist_EmptyRunnerSetIsFinalAndNotReDerived is the Review
// round 3 regression test (R3REV-06, AC-003.7/AC-003.9): an agent with no
// tasks in its runner set at all is a legitimate, final outcome — the
// resulting empty AllowedTaskIDs marked TaskScopeSourceRunnerSet must be
// treated the same as any other final scope (reused verbatim on the next
// build, not re-queried), not conflated with the provisional "unavailable"
// marker that legitimately retries.
func TestBuildAndPersist_EmptyRunnerSetIsFinalAndNotReDerived(t *testing.T) {
	lister := &stubRunnerLister{ids: nil, total: 0}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, Runs: store, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if runCtx.Capabilities.TaskScopeSource != TaskScopeSourceRunnerSet {
		t.Fatalf("TaskScopeSource = %q, want %q — an empty runner set is still a final "+
			"derivation, not unavailable", runCtx.Capabilities.TaskScopeSource, TaskScopeSourceRunnerSet)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 0 {
		t.Fatalf("AllowedTaskIDs = %v, want empty", runCtx.Capabilities.AllowedTaskIDs)
	}
	if runCtx.CanMutateTask("any-task") {
		t.Fatal("an empty runner set must refuse mutation authority over any task")
	}
	if lister.calls != 1 {
		t.Fatalf("expected exactly 1 runner-set query for the first build, got %d", lister.calls)
	}

	// A second build on the same (now-persisted) run must reuse the final
	// empty scope rather than re-querying the runner set.
	runCtx2, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("second BuildAndPersist: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("expected no additional runner-set query on reuse, got %d total calls", lister.calls)
	}
	if runCtx2.Capabilities.TaskScopeSource != TaskScopeSourceRunnerSet {
		t.Fatalf("reused TaskScopeSource = %q, want %q", runCtx2.Capabilities.TaskScopeSource, TaskScopeSourceRunnerSet)
	}
	if len(runCtx2.Capabilities.AllowedTaskIDs) != 0 {
		t.Fatalf("reused AllowedTaskIDs = %v, want empty", runCtx2.Capabilities.AllowedTaskIDs)
	}
}

func TestBuildAndPersist_UnavailableEmitsScopeUnavailableEvent(t *testing.T) {
	lister := &stubRunnerLister{err: errors.New("db down")}
	store := &recordingRunSnapshotStore{casWins: true}
	events := &recordingRunEvents{}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
		ScopeEvents:  events,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	if _, err := builder.BuildAndPersist(context.Background(), run); err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if len(events.events) != 1 || events.events[0].eventType != "runtime.scope_unavailable" {
		t.Fatalf("events = %+v, want one runtime.scope_unavailable", events.events)
	}
}

func TestBuildAndPersist_TaskBoundRunEmitsNoScopeEvent(t *testing.T) {
	store := &recordingRunSnapshotStore{casWins: true}
	events := &recordingRunEvents{}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, Runs: store, ScopeEvents: events}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{"task_id":"task-1"}`}

	if _, err := builder.BuildAndPersist(context.Background(), run); err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if len(events.events) != 0 {
		t.Fatalf("expected no scope event for a task-bound run, got %+v", events.events)
	}
}

func TestBuildAndPersist_NoRunsStorePersistsAndEmitsNothing(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	events := &recordingRunEvents{}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, RunnerLister: lister, ScopeEvents: events}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if len(events.events) != 0 {
		t.Fatalf("expected no events with no Runs store, got %+v", events.events)
	}
	if len(runCtx.Capabilities.AllowedTaskIDs) != 1 {
		t.Fatalf("expected a built context even with no Runs store: %+v", runCtx)
	}
}

func TestBuildAndPersist_NoAppenderSkipsEventButStillPersists(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, Runs: store, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	if _, err := builder.BuildAndPersist(context.Background(), run); err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected persistence even with no appender, got %d calls", len(store.calls))
	}
}

// -- Concurrency: CAS first-write-wins --

func TestBuildAndPersist_CASLoserAdoptsPersistedFinalScopeAndEmitsNoEvent(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"mine"}, total: 1}
	winnerCaps, err := MarshalCapabilities(Capabilities{
		AllowedTaskIDs:  []string{"winner-task"},
		TaskScopeSource: TaskScopeSourceRunnerSet,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	store := &recordingRunSnapshotStore{
		casWins: false,
		runs:    map[string]*models.Run{"run-1": {ID: "run-1", Capabilities: winnerCaps}},
	}
	events := &recordingRunEvents{}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
		ScopeEvents:  events,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	runCtx, err := builder.BuildAndPersist(context.Background(), run)
	if err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if got := runCtx.Capabilities.AllowedTaskIDs; len(got) != 1 || got[0] != "winner-task" {
		t.Fatalf("AllowedTaskIDs = %v, want the winner's persisted scope", got)
	}
	if len(events.events) != 0 {
		t.Fatalf("loser must emit no scope event, got %+v", events.events)
	}
	// The loser still performs the ordinary write with the adopted scope.
	if len(store.calls) != 1 {
		t.Fatalf("expected the loser to persist the adopted scope, got %d calls", len(store.calls))
	}
}

func TestBuildAndPersist_CASExhaustsAttemptsWithNoFinalMarker(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	store := &recordingRunSnapshotStore{
		casWins: false,
		runs:    map[string]*models.Run{"run-1": {ID: "run-1", Capabilities: `{}`}},
	}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	_, err := builder.BuildAndPersist(context.Background(), run)
	if err == nil {
		t.Fatal("expected an error after exhausting CAS attempts")
	}
	if lister.calls != maxScopeSwapAttempts {
		t.Fatalf("expected %d derive attempts, got %d", maxScopeSwapAttempts, lister.calls)
	}
	if len(store.casCalls) != maxScopeSwapAttempts {
		t.Fatalf("expected %d CAS attempts, got %d", maxScopeSwapAttempts, len(store.casCalls))
	}
	// The first attempt must compare against the run's own capabilities as
	// Build originally read them (empty here, since run.Capabilities is
	// unset); every retry after a lost race must compare against the
	// re-read run's capabilities, not the scope this attempt is about to
	// write — otherwise the CAS predicate could never observe a real prior
	// value and would lose every race forever.
	if store.casCalls[0].PrevCapabilities != "" {
		t.Fatalf("first CAS attempt PrevCapabilities = %q, want empty", store.casCalls[0].PrevCapabilities)
	}
	for i := 1; i < len(store.casCalls); i++ {
		if store.casCalls[i].PrevCapabilities != store.runs["run-1"].Capabilities {
			t.Fatalf(
				"CAS attempt %d PrevCapabilities = %q, want the re-read run's capabilities %q",
				i, store.casCalls[i].PrevCapabilities, store.runs["run-1"].Capabilities,
			)
		}
		if store.casCalls[i].PrevCapabilities == store.casCalls[i].Capabilities {
			t.Fatalf(
				"CAS attempt %d compared the scope it is about to write against itself: %q",
				i, store.casCalls[i].Capabilities,
			)
		}
	}
}

func TestBuildAndPersist_CASErrorReturnsError(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1"}, total: 1}
	store := &recordingRunSnapshotStore{casErr: errors.New("db down")}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, Runs: store, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	if _, err := builder.BuildAndPersist(context.Background(), run); err == nil {
		t.Fatal("expected the CAS error to propagate")
	}
}

func TestBuildAndPersist_NilRunReturnsErrorNotPanic(t *testing.T) {
	builder := ContextBuilder{}

	if _, err := builder.BuildAndPersist(context.Background(), nil); err == nil {
		t.Fatal("expected an error for a nil run, not a panic")
	}
}

func TestBuildAndPersist_CASLoserReadFailureReturnsError(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"mine"}, total: 1}
	store := &recordingRunSnapshotStore{casWins: false, getErr: errors.New("db down")}
	builder := ContextBuilder{
		Agents:       &recordingAgentReader{agent: taskAgent()},
		Runs:         store,
		RunnerLister: lister,
	}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	_, err := builder.BuildAndPersist(context.Background(), run)
	if err == nil {
		t.Fatal("expected the re-read failure to propagate")
	}
	if !strings.Contains(err.Error(), "re-read run after lost scope race") {
		t.Fatalf("error = %v, want it to mention the re-read failure", err)
	}
}

func TestBuildAndPersist_PersistsSerializedRunnerSetScope(t *testing.T) {
	lister := &stubRunnerLister{ids: []string{"t1", "t2"}, total: 2}
	store := &recordingRunSnapshotStore{casWins: true}
	builder := ContextBuilder{Agents: &recordingAgentReader{agent: taskAgent()}, Runs: store, RunnerLister: lister}
	run := &models.Run{ID: "run-1", AgentProfileID: "agent-1", Payload: `{}`}

	if _, err := builder.BuildAndPersist(context.Background(), run); err != nil {
		t.Fatalf("BuildAndPersist: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 snapshot write, got %d", len(store.calls))
	}
	if len(store.casCalls) != 1 || store.casCalls[0].PrevCapabilities != "" {
		t.Fatalf(
			"a fresh run with no persisted capabilities must CAS against an empty prior value: %+v",
			store.casCalls,
		)
	}
	if run.Capabilities == "" {
		t.Fatal("BuildAndPersist must mutate run.Capabilities in place — the JWT is minted from it")
	}

	var persistedCaps Capabilities
	if err := json.Unmarshal([]byte(store.calls[0].Capabilities), &persistedCaps); err != nil {
		t.Fatalf("decode persisted capabilities: %v", err)
	}
	if got := persistedCaps.AllowedTaskIDs; len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Fatalf("persisted AllowedTaskIDs = %v, want [t1 t2]", got)
	}
	if persistedCaps.TaskScopeSource != TaskScopeSourceRunnerSet {
		t.Fatalf("persisted TaskScopeSource = %q, want runner_set", persistedCaps.TaskScopeSource)
	}

	var persistedRunCtx RunContext
	if err := json.Unmarshal([]byte(store.calls[0].InputSnapshot), &persistedRunCtx); err != nil {
		t.Fatalf("decode persisted input snapshot: %v", err)
	}
	if got := persistedRunCtx.Capabilities.AllowedTaskIDs; len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Fatalf("persisted input snapshot AllowedTaskIDs = %v, want [t1 t2]", got)
	}

	// run.Capabilities is the third carrier of the derived scope — the run
	// token minted for this execution is marshalled from it, not from the
	// DB row or the returned RunContext, so it must reflect the same scope.
	var mutatedRunCaps Capabilities
	if err := json.Unmarshal([]byte(run.Capabilities), &mutatedRunCaps); err != nil {
		t.Fatalf("decode run.Capabilities: %v", err)
	}
	if got := mutatedRunCaps.AllowedTaskIDs; len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Fatalf("run.Capabilities AllowedTaskIDs = %v, want [t1 t2]", got)
	}
	if mutatedRunCaps.TaskScopeSource != TaskScopeSourceRunnerSet {
		t.Fatalf("run.Capabilities TaskScopeSource = %q, want runner_set", mutatedRunCaps.TaskScopeSource)
	}
}
