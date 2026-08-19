package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── fakes ────────────────────────────────────────────────────────────────

type acFakeTaskRepo struct {
	mu      sync.Mutex
	tasks   []*models.Task
	nextIdx int
}

func (f *acFakeTaskRepo) ListTasksByWorkspace(ctx context.Context, workspaceID, _, _, _ string, page, pageSize int, _ string, _, includeEphemeral, onlyEphemeral, _ bool) ([]*models.Task, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var filtered []*models.Task
	for _, t := range f.tasks {
		if t.WorkspaceID != workspaceID {
			continue
		}
		if onlyEphemeral && !t.IsEphemeral {
			continue
		}
		if !includeEphemeral && t.IsEphemeral {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered, len(filtered), nil
}

func (f *acFakeTaskRepo) ListEphemeralTasksAllWorkspaces(_ context.Context) ([]*models.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var filtered []*models.Task
	for _, t := range f.tasks {
		if t.IsEphemeral {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func (f *acFakeTaskRepo) CreateTask(_ context.Context, task *models.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	task.ID = f.makeID()
	f.tasks = append(f.tasks, task)
	return nil
}

func (f *acFakeTaskRepo) DeleteTask(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, t := range f.tasks {
		if t.ID == id {
			f.tasks = append(f.tasks[:i], f.tasks[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *acFakeTaskRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tasks)
}

func (f *acFakeTaskRepo) makeID() string {
	f.nextIdx++
	return "task-" + acItoa(f.nextIdx)
}

func acItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

type acFakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]*models.TaskSession
	nextIdx  int
}

func newACFakeSessionRepo() *acFakeSessionRepo {
	return &acFakeSessionRepo{sessions: make(map[string]*models.TaskSession)}
}

func (f *acFakeSessionRepo) GetPrimarySessionByTaskID(_ context.Context, taskID string) (*models.TaskSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.TaskID == taskID && s.IsPrimary {
			return s, nil
		}
	}
	return nil, nil
}

func (f *acFakeSessionRepo) CreateTaskSession(_ context.Context, session *models.TaskSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextIdx++
	session.ID = "session-" + acItoa(f.nextIdx)
	f.sessions[session.ID] = session
	return nil
}

func (f *acFakeSessionRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

func (f *acFakeSessionRepo) setState(id string, state models.TaskSessionState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sessions[id]; ok {
		s.State = state
	}
}

// acFakeProfileRepo defaults every profile ID to "found and enabled" so
// tests that don't care about profile validation (most Ensure tests
// predating it) need no setup. Tests exercising the configuration_required
// gate mark specific IDs missing/disabled explicitly.
type acFakeProfileRepo struct {
	mu       sync.Mutex
	missing  map[string]bool
	disabled map[string]bool
	calls    []string
}

func newACFakeProfileRepo() *acFakeProfileRepo {
	return &acFakeProfileRepo{missing: map[string]bool{}, disabled: map[string]bool{}}
}

func (f *acFakeProfileRepo) GetProfile(_ context.Context, profileID string) (AgentConversationProfileInfo, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, profileID)
	if f.missing[profileID] {
		return AgentConversationProfileInfo{}, false, nil
	}
	if f.disabled[profileID] {
		return AgentConversationProfileInfo{Enabled: false}, true, nil
	}
	return AgentConversationProfileInfo{Enabled: true}, true, nil
}

func (f *acFakeProfileRepo) markMissing(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.missing[id] = true
}

func (f *acFakeProfileRepo) markDisabled(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disabled[id] = true
}

// acFakeStateRepo simulates the durable atomic Claim primitive
// (internal/plugins/state.Store.Claim in production) with an in-process
// mutex-guarded map — sufficient to prove AgentConversationService's own
// concurrency handling; Store.Claim's SQL-level atomicity and restart
// durability are proven separately in internal/plugins/state.
type acFakeStateRepo struct {
	mu     sync.Mutex
	claims map[string]bool
}

func newACFakeStateRepo() *acFakeStateRepo {
	return &acFakeStateRepo{claims: map[string]bool{}}
}

func (f *acFakeStateRepo) Claim(_ context.Context, pluginID, scope, scopeID, key string, _ json.RawMessage) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := pluginID + "/" + scope + "/" + scopeID + "/" + key
	if f.claims[k] {
		return false, nil
	}
	f.claims[k] = true
	return true, nil
}

// acDeliverCall records one call reaching the fake dispatcher — i.e. one
// call that would have reached the real agent runtime in production.
type acDeliverCall struct {
	taskID        string
	sessionID     string
	text          string
	source        string
	idempotencyID string
}

// acFakeDispatcher stands in for the orchestrator-backed dispatcher.
// Recording calls here (rather than just asserting on a created message row,
// as the pre-fix tests did) is what proves Dispatch reaches the runtime
// delivery seam and not just message persistence.
type acFakeDispatcher struct {
	mu    sync.Mutex
	calls []acDeliverCall
	err   error
}

func newACFakeDispatcher() *acFakeDispatcher {
	return &acFakeDispatcher{}
}

func (f *acFakeDispatcher) Deliver(_ context.Context, taskID string, session *models.TaskSession, text, source, idempotencyID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.calls = append(f.calls, acDeliverCall{taskID: taskID, sessionID: session.ID, text: text, source: source, idempotencyID: idempotencyID})
	if session.State == models.TaskSessionStateCreated {
		return "started", nil
	}
	return "sent", nil
}

func (f *acFakeDispatcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *acFakeDispatcher) lastCall() acDeliverCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return acDeliverCall{}
	}
	return f.calls[len(f.calls)-1]
}

type acFakeEventBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (f *acFakeEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return nil
}

func (f *acFakeEventBus) getPublished() []*bus.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*bus.Event, len(f.events))
	copy(out, f.events)
	return out
}

type acTestDeps struct {
	tasks      *acFakeTaskRepo
	sess       *acFakeSessionRepo
	profiles   *acFakeProfileRepo
	state      *acFakeStateRepo
	dispatcher *acFakeDispatcher
	eventer    *acFakeEventBus
}

func newACTestService() (*AgentConversationService, acTestDeps) {
	deps := acTestDeps{
		tasks:      &acFakeTaskRepo{},
		sess:       newACFakeSessionRepo(),
		profiles:   newACFakeProfileRepo(),
		state:      newACFakeStateRepo(),
		dispatcher: newACFakeDispatcher(),
		eventer:    &acFakeEventBus{},
	}
	svc := NewAgentConversationService(deps.tasks, deps.sess, deps.profiles, deps.state, deps.eventer)
	svc.SetDispatcher(deps.dispatcher)
	return svc, deps
}

// ── Ensure tests ────────────────────────────────────────────────────────

func TestEnsureCreatesConversation(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		BasePrompt:      "You are a coordinator.",
	}

	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if desc.TaskID == "" {
		t.Fatal("expected non-empty TaskID")
	}
	if desc.SessionID == "" {
		t.Fatal("expected non-empty SessionID")
	}
	if desc.WorkspaceID != "ws-1" {
		t.Fatalf("WorkspaceID = %q, want ws-1", desc.WorkspaceID)
	}
	if desc.ConversationKey != "coordinator" {
		t.Fatalf("ConversationKey = %q, want coordinator", desc.ConversationKey)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("status = %q, want created", statusStr)
	}

	published := deps.eventer.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(published))
	}
	if published[0].Type != events.TaskCreated {
		t.Fatalf("event type = %q, want %q", published[0].Type, events.TaskCreated)
	}
}

func TestEnsureReturnsExistsOnSecondCall(t *testing.T) {
	svc, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}

	desc1, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	desc2, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusExists {
		t.Fatalf("status = %q, want exists", statusStr)
	}
	if desc1.TaskID != desc2.TaskID {
		t.Fatalf("TaskID changed: %q → %q", desc1.TaskID, desc2.TaskID)
	}
	if desc1.SessionID != desc2.SessionID {
		t.Fatalf("SessionID changed: %q → %q", desc1.SessionID, desc2.SessionID)
	}
}

func TestEnsureRejectsEmptyArguments(t *testing.T) {
	svc, _ := newACTestService()

	tests := []struct {
		name   string
		plugin string
		spec   pluginsdk.AgentConversationSpec
	}{
		{"empty plugin id", "", pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "key"}},
		{"empty workspace", "p", pluginsdk.AgentConversationSpec{WorkspaceID: "", ConversationKey: "key"}},
		{"empty key", "p", pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.Ensure(context.Background(), tt.plugin, tt.spec)
			if err == nil {
				t.Fatal("expected error")
			}
			if code := status.Code(err); code != codes.InvalidArgument {
				t.Fatalf("got code %v, want InvalidArgument", code)
			}
		})
	}
}

func TestEnsureRepairsMissingPrimarySession(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}

	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("status = %q, want created", statusStr)
	}

	// Delete the session to simulate a partial creation.
	deps.sess.mu.Lock()
	delete(deps.sess.sessions, desc.SessionID)
	deps.sess.mu.Unlock()

	// Second Ensure should repair the missing session.
	desc2, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusExists {
		t.Fatalf("status = %q, want exists", statusStr)
	}
	if desc2.TaskID != desc.TaskID {
		t.Fatalf("TaskID changed")
	}
	if desc2.SessionID == "" || desc2.SessionID == desc.SessionID {
		t.Fatalf("expected new SessionID, got %q", desc2.SessionID)
	}
}

func TestEnsureStoresProfileIDInMetadata(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "profile-gpt4",
	}

	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("status = %q, want created", statusStr)
	}

	if desc.AgentProfileID != "profile-gpt4" {
		t.Fatalf("AgentProfileID = %q, want profile-gpt4", desc.AgentProfileID)
	}

	task, _, _ := deps.tasks.ListTasksByWorkspace(context.Background(), "ws-1", "", "", "", 1, 100, "", false, true, true, false)
	if len(task) == 0 {
		t.Fatal("no tasks found")
	}
	if p, ok := task[0].Metadata[models.MetaKeyAgentProfileID].(string); !ok || p != "profile-gpt4" {
		t.Fatalf("metadata agent_profile_id = %v, want profile-gpt4", task[0].Metadata[models.MetaKeyAgentProfileID])
	}
}

func TestEnsureProfileInDescriptorFromExistingTask(t *testing.T) {
	svc, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "profile-gpt4",
	}

	desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if desc.AgentProfileID != "profile-gpt4" {
		t.Fatalf("first AgentProfileID = %q", desc.AgentProfileID)
	}

	desc2, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusExists {
		t.Fatalf("status = %q, want exists", statusStr)
	}
	if desc2.AgentProfileID != "profile-gpt4" {
		t.Fatalf("second AgentProfileID = %q", desc2.AgentProfileID)
	}
}

func TestEnsureCrossPluginSeparation(t *testing.T) {
	svc, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}

	descA, _, err := svc.Ensure(context.Background(), "plugin-a", spec)
	if err != nil {
		t.Fatalf("plugin-a Ensure: %v", err)
	}

	descB, statusStr, err := svc.Ensure(context.Background(), "plugin-b", spec)
	if err != nil {
		t.Fatalf("plugin-b Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("plugin-b status = %q, want created (separate)", statusStr)
	}
	if descA.TaskID == descB.TaskID {
		t.Fatal("plugin-a and plugin-b should have different task IDs")
	}
}

func TestEnsureCrossWorkspaceSeparation(t *testing.T) {
	svc, _ := newACTestService()

	desc1, _, err := svc.Ensure(context.Background(), "plugin-c", pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	})
	if err != nil {
		t.Fatalf("ws-1 Ensure: %v", err)
	}

	desc2, statusStr, err := svc.Ensure(context.Background(), "plugin-c", pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-2",
		ConversationKey: "coordinator",
	})
	if err != nil {
		t.Fatalf("ws-2 Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("ws-2 status = %q, want created", statusStr)
	}
	if desc1.TaskID == desc2.TaskID {
		t.Fatal("different workspaces should have different task IDs")
	}
}

func TestConcurrentEnsureIsIdempotent(t *testing.T) {
	svc, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}

	var wg sync.WaitGroup
	results := make([]struct {
		desc pluginsdk.AgentConversationDescriptor
		err  error
	}, 10)

	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
			results[idx] = struct {
				desc pluginsdk.AgentConversationDescriptor
				err  error
			}{desc, err}
		}(i)
	}
	wg.Wait()

	taskIDs := make(map[string]int)
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("concurrent Ensure error: %v", r.err)
		}
		taskIDs[r.desc.TaskID]++
	}
	if len(taskIDs) != 1 {
		t.Fatalf("expected 1 unique task ID, got %d: %v", len(taskIDs), taskIDs)
	}
}

func TestStoresBasePromptInMetadata(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		BasePrompt:      "You are a cycle coordinator.",
	}

	_, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	task, _, _ := deps.tasks.ListTasksByWorkspace(context.Background(), "ws-1", "", "", "", 1, 100, "", false, true, true, false)
	if len(task) == 0 {
		t.Fatal("no tasks")
	}
	if p, ok := task[0].Metadata["kandev.base_prompt"].(string); !ok || p != "You are a cycle coordinator." {
		t.Fatalf("base_prompt = %v, want 'You are a cycle coordinator.'", task[0].Metadata["kandev.base_prompt"])
	}
}

func TestAgentConversationTaskIsCreatedAsEphemeral(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	all, _, _ := deps.tasks.ListTasksByWorkspace(context.Background(), "ws-1", "", "", "", 1, 100, "", false, true, true, false)
	if len(all) != 1 {
		t.Fatalf("expected 1 task, got %d", len(all))
	}
	if !all[0].IsEphemeral {
		t.Fatal("task should be ephemeral")
	}
	if all[0].Origin != models.TaskOriginManual {
		t.Fatalf("origin = %q, want manual", all[0].Origin)
	}
	if all[0].Priority != "medium" {
		t.Fatalf("priority = %q, want medium", all[0].Priority)
	}
	if all[0].State != v1.TaskStateCreated {
		t.Fatalf("state = %v, want CREATED", all[0].State)
	}
}

func TestIsManagedConversationTask(t *testing.T) {
	tests := []struct {
		name string
		task *models.Task
		want bool
	}{
		{
			name: "nil task",
			task: nil,
			want: false,
		},
		{
			name: "nil metadata",
			task: &models.Task{IsEphemeral: true},
			want: false,
		},
		{
			name: "ephemeral with plugin id",
			task: &models.Task{
				IsEphemeral: true,
				Metadata: map[string]interface{}{
					metaKeyPluginID: "plugin-coordinator",
				},
			},
			want: true,
		},
		{
			name: "non-ephemeral with plugin id",
			task: &models.Task{
				IsEphemeral: false,
				Metadata: map[string]interface{}{
					metaKeyPluginID: "plugin-coordinator",
				},
			},
			want: false,
		},
		{
			name: "ephemeral without plugin id",
			task: &models.Task{
				IsEphemeral: true,
				Metadata: map[string]interface{}{
					"some_other_key": "value",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsManagedConversationTask(tt.task)
			if got != tt.want {
				t.Fatalf("IsManagedConversationTask = %v, want %v", got, tt.want)
			}
		})
	}
}
