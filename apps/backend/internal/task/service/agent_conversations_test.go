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

type acFakeMessageRepo struct {
	mu       sync.Mutex
	messages []*models.Message
}

func (f *acFakeMessageRepo) CreateMessage(_ context.Context, msg *models.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return nil
}

type acFakeStateRepo struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newACFakeStateRepo() *acFakeStateRepo {
	return &acFakeStateRepo{data: make(map[string][]byte)}
}

func (f *acFakeStateRepo) Get(_ context.Context, scope, scopeID, key string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := scope + "/" + scopeID + "/" + key
	v, ok := f.data[k]
	return v, ok, nil
}

func (f *acFakeStateRepo) Set(_ context.Context, scope, scopeID, key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := scope + "/" + scopeID + "/" + key
	f.data[k] = value
	return nil
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

func newACTestService() (*AgentConversationService, *acFakeTaskRepo, *acFakeSessionRepo, *acFakeMessageRepo, *acFakeStateRepo, *acFakeEventBus) {
	tasks := &acFakeTaskRepo{}
	sess := newACFakeSessionRepo()
	msgs := &acFakeMessageRepo{}
	state := newACFakeStateRepo()
	eventer := &acFakeEventBus{}
	svc := NewAgentConversationService(tasks, sess, msgs, state, eventer)
	return svc, tasks, sess, msgs, state, eventer
}

// ── Tests ────────────────────────────────────────────────────────────────

func TestEnsureCreatesConversation(t *testing.T) {
	svc, _, _, _, _, eventer := newACTestService()

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
	if statusStr != "created" {
		t.Fatalf("status = %q, want created", statusStr)
	}

	// Verify event was published.
	published := eventer.getPublished()
	if len(published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(published))
	}
	if published[0].Type != events.TaskCreated {
		t.Fatalf("event type = %q, want %q", published[0].Type, events.TaskCreated)
	}
}

func TestEnsureReturnsExistsOnSecondCall(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

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
	if statusStr != "exists" {
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
	svc, _, _, _, _, _ := newACTestService()

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
	svc, tasks, sess, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}

	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if statusStr != "created" {
		t.Fatalf("status = %q, want created", statusStr)
	}

	// Delete the session to simulate a partial creation.
	sess.mu.Lock()
	delete(sess.sessions, desc.SessionID)
	sess.mu.Unlock()

	// Second Ensure should repair the missing session.
	desc2, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr != "exists" {
		t.Fatalf("status = %q, want exists", statusStr)
	}
	if desc2.TaskID != desc.TaskID {
		t.Fatalf("TaskID changed")
	}
	if desc2.SessionID == "" || desc2.SessionID == desc.SessionID {
		t.Fatalf("expected new SessionID, got %q", desc2.SessionID)
	}
	_ = tasks
}

func TestEnsureStoresProfileIDInMetadata(t *testing.T) {
	svc, tasks, _, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "profile-gpt4",
	}

	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if statusStr != "created" {
		t.Fatalf("status = %q, want created", statusStr)
	}

	// Verify profile ID is in the return value.
	if desc.AgentProfileID != "profile-gpt4" {
		t.Fatalf("AgentProfileID = %q, want profile-gpt4", desc.AgentProfileID)
	}

	// Verify the task metadata stored the profile ID.
	task, _, _ := tasks.ListTasksByWorkspace(context.Background(), "ws-1", "", "", "", 1, 100, "", false, true, true, false)
	if len(task) == 0 {
		t.Fatal("no tasks found")
	}
	if p, ok := task[0].Metadata[models.MetaKeyAgentProfileID].(string); !ok || p != "profile-gpt4" {
		t.Fatalf("metadata agent_profile_id = %v, want profile-gpt4", task[0].Metadata[models.MetaKeyAgentProfileID])
	}
}

func TestEnsureProfileInDescriptorFromExistingTask(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

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

	// Ensure again with matching profile.
	desc2, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr != "exists" {
		t.Fatalf("status = %q, want exists", statusStr)
	}
	if desc2.AgentProfileID != "profile-gpt4" {
		t.Fatalf("second AgentProfileID = %q", desc2.AgentProfileID)
	}
}

func TestEnsureCrossPluginSeparation(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}

	// Plugin A ensures a conversation.
	descA, _, err := svc.Ensure(context.Background(), "plugin-a", spec)
	if err != nil {
		t.Fatalf("plugin-a Ensure: %v", err)
	}

	// Plugin B ensures with same workspace/key — should create separate.
	descB, statusStr, err := svc.Ensure(context.Background(), "plugin-b", spec)
	if err != nil {
		t.Fatalf("plugin-b Ensure: %v", err)
	}
	if statusStr != "created" {
		t.Fatalf("plugin-b status = %q, want created (separate)", statusStr)
	}
	if descA.TaskID == descB.TaskID {
		t.Fatal("plugin-a and plugin-b should have different task IDs")
	}
}

func TestEnsureCrossWorkspaceSeparation(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

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
	if statusStr != "created" {
		t.Fatalf("ws-2 status = %q, want created", statusStr)
	}
	if desc1.TaskID == desc2.TaskID {
		t.Fatal("different workspaces should have different task IDs")
	}
}

func TestDispatchSendsMessage(t *testing.T) {
	svc, _, _, msgs, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	dispatch, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check the board", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatch.Status != "sent" {
		t.Fatalf("status = %q, want sent", dispatch.Status)
	}
	if dispatch.Descriptor.TaskID == "" {
		t.Fatal("expected descriptor TaskID")
	}

	// Verify the message was created.
	if len(msgs.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs.messages))
	}
	if msgs.messages[0].Content != "Check the board" {
		t.Fatalf("message content = %q", msgs.messages[0].Content)
	}
	if msgs.messages[0].AuthorType != models.MessageAuthorUser {
		t.Fatalf("author type = %q", msgs.messages[0].AuthorType)
	}
}

func TestDispatchBeforeEnsure(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

	_, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Hello", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("got code %v, want NotFound", code)
	}
}

func TestDispatchWithOccurrenceKey(t *testing.T) {
	svc, _, _, msgs, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	key := "wake:cycle:2026-08-17T10:00:00Z"
	first, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", key)
	if err != nil {
		t.Fatalf("first Dispatch: %v", err)
	}
	if first.Status != "sent" {
		t.Fatalf("first status = %q, want sent", first.Status)
	}

	// Same occurrence key should be duplicate.
	second, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check again", key)
	if err != nil {
		t.Fatalf("second Dispatch: %v", err)
	}
	if second.Status != "duplicate_occurrence" {
		t.Fatalf("second status = %q, want duplicate_occurrence", second.Status)
	}

	// Only one message should have been created.
	if len(msgs.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs.messages))
	}
}

func TestDispatchWithBusySession(t *testing.T) {
	svc, _, sess, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Mark the session as running.
	sess.mu.Lock()
	if session, ok := sess.sessions[desc.SessionID]; ok {
		session.State = models.TaskSessionStateRunning
	}
	sess.mu.Unlock()

	dispatch, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatch.Status != "skipped_busy" {
		t.Fatalf("status = %q, want skipped_busy", dispatch.Status)
	}
}

func TestDispatchRejectsEmptyArguments(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

	tests := []struct {
		name, plugin, ws, key, text string
	}{
		{"empty plugin", "", "ws-1", "key", "hello"},
		{"empty workspace", "p", "", "key", "hello"},
		{"empty key", "p", "ws-1", "", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Dispatch(context.Background(), tt.plugin, tt.ws, tt.key, tt.text, "")
			if err == nil {
				t.Fatal("expected error")
			}
			if code := status.Code(err); code != codes.InvalidArgument {
				t.Fatalf("got %v, want InvalidArgument", code)
			}
		})
	}
}

func TestDispatchRecordsProvenanceInMetadata(t *testing.T) {
	svc, _, _, msgs, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	_, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", "occ-1")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if len(msgs.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs.messages))
	}
	meta := msgs.messages[0].Metadata
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if source, ok := meta["source"].(string); !ok || source != "plugin:plugin-coordinator" {
		t.Fatalf("source = %v", meta["source"])
	}
	if key, ok := meta["occurrence_key"].(string); !ok || key != "occ-1" {
		t.Fatalf("occurrence_key = %v", meta["occurrence_key"])
	}
}

func TestDeleteRemovesConversation(t *testing.T) {
	svc, tasks, _, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	count, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-1", "coordinator")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("deleted count = %d, want 1", count)
	}

	// Verify it's gone.
	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure after delete: %v", err)
	}
	if statusStr != "created" {
		t.Fatalf("status = %q, want created", statusStr)
	}
	if desc.TaskID != "" {
		// A different task ID means the old one was deleted and a new one created.
		_ = tasks
	}
}

func TestDeleteOnlyOwnedConversations(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-a", spec); err != nil {
		t.Fatalf("plugin-a Ensure: %v", err)
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-b", spec); err != nil {
		t.Fatalf("plugin-b Ensure: %v", err)
	}

	// Delete plugin-a's conversation only.
	count, err := svc.Delete(context.Background(), "plugin-a", "ws-1", "coordinator")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("deleted = %d, want 1", count)
	}

	// Plugin-b's should still exist.
	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-b", spec)
	if err != nil {
		t.Fatalf("plugin-b Ensure: %v", err)
	}
	if statusStr != "exists" {
		t.Fatalf("plugin-b status = %q, want exists", statusStr)
	}
	_ = desc
}

func TestDeleteIdempotent(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

	// Delete a conversation that was never created.
	count, err := svc.Delete(context.Background(), "plugin-nonexistent", "ws-1", "key")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted = %d, want 0", count)
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

func TestStoresBasePromptInMetadata(t *testing.T) {
	svc, tasks, _, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		BasePrompt:      "You are a cycle coordinator.",
	}

	_, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	task, _, _ := tasks.ListTasksByWorkspace(context.Background(), "ws-1", "", "", "", 1, 100, "", false, true, true, false)
	if len(task) == 0 {
		t.Fatal("no tasks")
	}
	if p, ok := task[0].Metadata["kandev.base_prompt"].(string); !ok || p != "You are a cycle coordinator." {
		t.Fatalf("base_prompt = %v, want 'You are a cycle coordinator.'", task[0].Metadata["kandev.base_prompt"])
	}
}

func TestConcurrentEnsureIsIdempotent(t *testing.T) {
	svc, _, _, _, _, _ := newACTestService()

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

	// Collect unique task IDs; there should be exactly one.
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

func TestOccurrenceKeyIdempotentAcrossDispatches(t *testing.T) {
	svc, _, _, msgs, state, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	key := "wake:standup:2026-08-17"
	d1, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Standup", key)
	if err != nil {
		t.Fatalf("Dispatch 1: %v", err)
	}
	if d1.Status != "sent" {
		t.Fatalf("status 1 = %q", d1.Status)
	}

	// Verify the state backend has the claim.
	k := "plugin:plugin-coordinator"
	sid := "ws-1/coordinator"
	raw, found, err := state.Get(context.Background(), k, sid, "occurrence:"+key)
	if err != nil {
		t.Fatalf("state.Get: %v", err)
	}
	if !found {
		t.Fatal("occurrence key not found in state")
	}
	var claim map[string]interface{}
	if err := json.Unmarshal(raw, &claim); err != nil {
		t.Fatalf("unmarshal claim: %v", err)
	}
	if claim["claimed"] != true {
		t.Fatal("claim not true")
	}

	// Repeat with same key.
	d2, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Standup again", key)
	if err != nil {
		t.Fatalf("Dispatch 2: %v", err)
	}
	if d2.Status != "duplicate_occurrence" {
		t.Fatalf("status 2 = %q, want duplicate_occurrence", d2.Status)
	}

	// Only 1 message across both dispatches.
	if len(msgs.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs.messages))
	}
}

func TestDispatchRecordsOccurrenceInMessageMetadata(t *testing.T) {
	svc, _, _, msgs, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	_, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Run cycle", "cycle:2026-08-17T10:00")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	msg := msgs.messages[0]
	if key, ok := msg.Metadata["occurrence_key"].(string); !ok || key != "cycle:2026-08-17T10:00" {
		t.Fatalf("occurrence_key = %v, want cycle:2026-08-17T10:00", msg.Metadata["occurrence_key"])
	}
}

// Test to ensure ManagedConversation workflow
func TestAgentConversationTaskIsCreatedAsEphemeral(t *testing.T) {
	svc, tasks, _, _, _, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Verify through the task repo that the task is ephemeral.
	all, _, _ := tasks.ListTasksByWorkspace(context.Background(), "ws-1", "", "", "", 1, 100, "", false, true, true, false)
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
