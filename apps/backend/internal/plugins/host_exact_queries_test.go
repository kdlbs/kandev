package plugins

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newExactTaskReadHost(t *testing.T, data taskDataSource) *pluginHost {
	return newExactReadHost(t, data, []string{"tasks"}, []string{"host.v2.read:tasks"})
}

func newExactReadHost(t *testing.T, data taskDataSource, resources, grants []string) *pluginHost {
	t.Helper()
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID:           "exact-reader",
			Capabilities: manifest.Capabilities{APIRead: resources},
		},
		InstallationID: "exact-reader-installation",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(store.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-one", 1,
		ManifestCapabilityDigest(record.Manifest), grants, "human", "grant", "approval-one"); err != nil {
		t.Fatalf("grant task read: %v", err)
	}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	host.taskData = data
	return host
}

func TestExactWorkspaceSnapshot(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	first := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", State: "in_progress", Title: "First", UpdatedAt: base}
	second := &taskmodels.Task{ID: "task-two", WorkspaceID: "workspace-one", State: "review", Title: "Second", UpdatedAt: base.Add(time.Second)}
	foreign := &taskmodels.Task{ID: "task-foreign", WorkspaceID: "workspace-two", State: "in_progress", Title: "Foreign", UpdatedAt: base}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}, {ID: "workspace-two"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {first, second}, "workspace-two": {foreign}},
	}
	host := newExactTaskReadHost(t, data)

	page, err := host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "snapshot-first-page", WorkspaceID: "workspace-one",
		Page: pluginsdk.ExactReadPage{Limit: 1},
	})
	if err != nil {
		t.Fatalf("ListTasksExact first page: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Task.ID != "task-one" || page.Items[0].CanonicalStatus != "in_progress" || !page.Items[0].StatusKnown {
		t.Fatalf("first exact task page = %+v, want canonical first task", page.Items)
	}
	if page.PageInfo.SnapshotVersion == "" || page.PageInfo.NextCursor == "" {
		t.Fatalf("first page info = %+v, want snapshot and next cursor", page.PageInfo)
	}
	wireHost := dialPluginHostOverWire(t, host)
	queryManager, ok := pluginsdk.HostExactQueries(wireHost)
	if !ok {
		t.Fatal("plugin Host transport does not expose exact queries")
	}
	wirePage, err := queryManager.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "snapshot-wire-page", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 1},
	})
	if err != nil || len(wirePage.Items) != 1 || wirePage.Items[0].Task.ID != "task-one" || wirePage.PageInfo.Receipt.InstallationID != "exact-reader-installation" {
		t.Fatalf("wire exact task page = %+v, err=%v", wirePage, err)
	}

	second.Title = "Changed after snapshot"
	second.UpdatedAt = base.Add(2 * time.Second)
	data.tasksByWorkspace["workspace-one"] = []*taskmodels.Task{first, second}
	next, err := host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "snapshot-second-page", WorkspaceID: "workspace-one",
		Page: pluginsdk.ExactReadPage{Limit: 1, Cursor: page.PageInfo.NextCursor, SnapshotVersion: page.PageInfo.SnapshotVersion},
	})
	if err != nil {
		t.Fatalf("ListTasksExact second page: %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].Task.ID != "task-two" || next.Items[0].Task.Title != "Second" {
		t.Fatalf("second page = %+v, want the original snapshot row", next.Items)
	}
	fresh, err := host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "snapshot-reconcile", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(fresh.Items) != 2 || fresh.Items[1].Task.Title != "Changed after snapshot" {
		t.Fatalf("fresh reconciliation snapshot = %+v, err=%v; want current task rows", fresh.Items, err)
	}

	if _, err := host.service.approvalGrant(host.installationID, "workspace-two", 1,
		ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest),
		[]string{"host.v2.read:tasks"}, "human", "grant", "approval-workspace-two"); err != nil {
		t.Fatalf("grant second workspace: %v", err)
	}
	_, err = host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "cross-workspace-cursor", WorkspaceID: "workspace-two",
		Page: pluginsdk.ExactReadPage{Limit: 1, Cursor: page.PageInfo.NextCursor, SnapshotVersion: page.PageInfo.SnapshotVersion},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("cross-workspace cursor error = %v, want FailedPrecondition", err)
	}
	_, err = host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "tampered-cursor", WorkspaceID: "workspace-one",
		Page: pluginsdk.ExactReadPage{Limit: 1, Cursor: page.PageInfo.NextCursor + "x", SnapshotVersion: page.PageInfo.SnapshotVersion},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("tampered cursor error = %v, want InvalidArgument", err)
	}
	_, err = host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "changed-filter", WorkspaceID: "workspace-one", Filter: pluginsdk.TaskFilter{States: []string{"review"}},
		Page: pluginsdk.ExactReadPage{Limit: 1, Cursor: page.PageInfo.NextCursor, SnapshotVersion: page.PageInfo.SnapshotVersion},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("changed-filter cursor error = %v, want FailedPrecondition", err)
	}

	if _, err := host.service.approvalGrant(host.installationID, "workspace-one", 2,
		ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest),
		[]string{"host.v2.read:tasks"}, "human", "regrant", "approval-two"); err != nil {
		t.Fatalf("advance approval revision: %v", err)
	}
	_, err = host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "stale-approval-cursor", WorkspaceID: "workspace-one",
		Page: pluginsdk.ExactReadPage{Limit: 1, Cursor: page.PageInfo.NextCursor, SnapshotVersion: page.PageInfo.SnapshotVersion},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("stale-approval cursor error = %v, want FailedPrecondition", err)
	}
}

func TestExactSessionObservationIncludesRecoveryExecutionFence(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{ID: "task-recovery", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	session := &taskmodels.TaskSession{
		ID: "session-recovery", TaskID: task.ID, State: taskmodels.TaskSessionStateFailed,
		AgentExecutionID: "execution-current", UpdatedAt: base.Add(time.Second),
	}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {task}},
		tasksByID:        map[string]*taskmodels.Task{task.ID: task},
		sessionsByTask:   map[string][]*taskmodels.TaskSession{task.ID: {session}},
	}
	host := newExactReadHost(t, data, []string{"sessions"}, []string{"host.v2.read:sessions"})
	page, err := host.ListSessions(context.Background(), pluginsdk.ExactSessionQuery{
		RequestID: "recovery-session-read", WorkspaceID: "workspace-one",
		Filter: pluginsdk.SessionFilter{TaskIDs: []string{task.ID}}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListSessions = %+v, err=%v; want one observed session", page.Items, err)
	}
	if page.Items[0].ExecutionID != "execution-current" || page.Items[0].ResourceVersion == "" {
		t.Fatalf("session recovery fence = %+v, want current execution ID and resource version", page.Items[0])
	}

	wireHost := dialPluginHostOverWire(t, host)
	queries, ok := pluginsdk.HostExactQueries(wireHost)
	if !ok {
		t.Fatal("plugin Host transport does not expose exact queries")
	}
	wirePage, err := queries.ListSessions(context.Background(), pluginsdk.ExactSessionQuery{
		RequestID: "recovery-session-wire-read", WorkspaceID: "workspace-one",
		Filter: pluginsdk.SessionFilter{TaskIDs: []string{task.ID}}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(wirePage.Items) != 1 || wirePage.Items[0].ExecutionID != "execution-current" {
		t.Fatalf("wire recovery session fence = %+v, err=%v; want execution ID", wirePage.Items, err)
	}
}

type fakePendingTaskTransitionSource struct {
	items []PendingTaskTransitionRecord
	err   error
}

func (s *fakePendingTaskTransitionSource) ListPendingTaskTransitions(context.Context) ([]PendingTaskTransitionRecord, error) {
	return s.items, s.err
}

func TestExactTaskInboxAndPendingTransitions(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", WorkflowID: "workflow-one", WorkflowStepID: "step-one", State: "in_progress", UpdatedAt: base}
	foreign := &taskmodels.Task{ID: "task-foreign", WorkspaceID: "workspace-two", WorkflowID: "workflow-two", WorkflowStepID: "step-two", State: "in_progress", UpdatedAt: base}
	session := &taskmodels.TaskSession{ID: "session-one", TaskID: task.ID, State: "running", UpdatedAt: base}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}, {ID: "workspace-two"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {task}, "workspace-two": {foreign}},
		tasksByID:        map[string]*taskmodels.Task{task.ID: task, foreign.ID: foreign},
		sessionsByTask:   map[string][]*taskmodels.TaskSession{task.ID: {session}},
	}
	host := newExactReadHost(t, data,
		[]string{"tasks", "task_inbox", "task_transitions", "interactions"},
		[]string{"host.v2.read:tasks", "host.v2.read:task_inbox", "host.v2.read:task_transitions", "host.v2.read:interactions"},
	)
	host.interactionData = &fakeInteractionDataSource{pending: []*taskmodels.Interaction{{
		ID: "interaction-one", Kind: taskmodels.InteractionKindPermission, TaskID: task.ID, SessionID: session.ID,
		Status: taskmodels.InteractionStatusPending, CreatedAt: base,
	}}}
	host.pendingTaskTransitions = func() pendingTaskTransitionSource {
		return &fakePendingTaskTransitionSource{items: []PendingTaskTransitionRecord{
			{ID: "move-one", TaskID: task.ID, SessionID: session.ID, WorkflowID: "workflow-two", WorkflowStepID: "step-next", QueuedAt: base.Add(time.Minute)},
			{ID: "foreign-move", TaskID: foreign.ID, SessionID: "foreign-session", WorkflowID: "workflow-two", WorkflowStepID: "step-next", QueuedAt: base.Add(2 * time.Minute)},
		}}
	}

	transitions, err := host.ListPendingTaskTransitions(context.Background(), pluginsdk.ExactPendingTaskTransitionsQuery{
		RequestID: "pending-moves", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(transitions.Items) != 1 {
		t.Fatalf("pending transitions = %+v, err=%v; want one workspace-scoped row", transitions.Items, err)
	}
	move := transitions.Items[0]
	if move.ID != "move-one" || move.FromWorkflowID != task.WorkflowID || move.FromStepID != task.WorkflowStepID || move.ToWorkflowID != "workflow-two" || move.ToStepID != "step-next" || move.State != "pending" || move.ResourceVersion == "" {
		t.Fatalf("pending transition = %+v, want canonical current and requested targets", move)
	}

	inbox, err := host.ListTaskInbox(context.Background(), pluginsdk.ExactTaskInboxQuery{
		RequestID: "task-inbox", WorkspaceID: "workspace-one", TaskID: task.ID, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(inbox.Items) != 2 {
		t.Fatalf("task inbox = %+v, err=%v; want pending interaction and workflow transition", inbox.Items, err)
	}
	if inbox.Items[0].TaskID != task.ID || inbox.Items[1].TaskID != task.ID {
		t.Fatalf("task inbox leaked another workspace: %+v", inbox.Items)
	}
	for _, item := range inbox.Items {
		if item.ResourceVersion == "" || item.State != "pending" {
			t.Fatalf("task inbox item = %+v, want pending state and resource version", item)
		}
	}
	capabilityContext, err := host.GetCapabilityContext(context.Background(), "workspace-one")
	if err != nil {
		t.Fatalf("GetCapabilityContext: %v", err)
	}
	wantSupport := map[string]bool{
		exactReadListTaskInbox:       true,
		exactReadListTaskTransitions: true,
		exactReadGetTaskDirective:    false,
		exactReadListTaskDirectives:  false,
	}
	for _, operation := range capabilityContext.Operations {
		if want, tracked := wantSupport[operation.Method]; tracked && operation.Supported != want {
			t.Fatalf("operation %s supported = %v, want %v (%s)", operation.Method, operation.Supported, want, operation.UnavailableReason)
		}
	}

	_, err = host.ListPendingTaskTransitions(context.Background(), pluginsdk.ExactPendingTaskTransitionsQuery{
		RequestID: "foreign-task", WorkspaceID: "workspace-one", TaskID: foreign.ID, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign task transition filter error = %v, want InvalidArgument", err)
	}
	_, err = host.ListTaskInbox(context.Background(), pluginsdk.ExactTaskInboxQuery{
		RequestID: "foreign-session", WorkspaceID: "workspace-one", TaskID: task.ID, SessionID: "foreign-session", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign session inbox filter error = %v, want InvalidArgument", err)
	}
	wireHost := dialPluginHostOverWire(t, host)
	queryManager, ok := pluginsdk.HostExactQueries(wireHost)
	if !ok {
		t.Fatal("plugin Host transport does not expose exact query extension")
	}
	wireTransitions, err := queryManager.ListPendingTaskTransitions(context.Background(), pluginsdk.ExactPendingTaskTransitionsQuery{
		RequestID: "wire-pending-moves", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(wireTransitions.Items) != 1 || wireTransitions.Items[0].ToStepID != "step-next" {
		t.Fatalf("wire pending transitions = %+v, err=%v", wireTransitions.Items, err)
	}
}

func TestExactTaskRelationsAreVersionedAndWorkspaceScoped(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	parent := &taskmodels.Task{ID: "parent", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	child := &taskmodels.Task{ID: "child", WorkspaceID: "workspace-one", ParentID: parent.ID, State: "in_progress", UpdatedAt: base.Add(time.Second)}
	foreign := &taskmodels.Task{ID: "foreign", WorkspaceID: "workspace-two", State: "in_progress", UpdatedAt: base}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}, {ID: "workspace-two"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {parent, child}, "workspace-two": {foreign}},
	}
	host := newExactReadHost(t, data, []string{"tasks", "task_relations"}, []string{"host.v2.read:tasks", "host.v2.read:task_relations"})
	page, err := host.ListTaskRelations(context.Background(), pluginsdk.ExactTaskRelationsQuery{
		RequestID: "relations", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("task relations = %+v, err=%v", page.Items, err)
	}
	relation := page.Items[0]
	if relation.SourceTaskID != parent.ID || relation.TargetTaskID != child.ID || relation.Kind != "parent" || relation.SourceResourceVersion == "" || relation.TargetResourceVersion == "" || relation.ResourceVersion == "" {
		t.Fatalf("task relation = %+v, want a versioned parent edge", relation)
	}
	_, err = host.ListTaskRelations(context.Background(), pluginsdk.ExactTaskRelationsQuery{
		RequestID: "foreign-relation-filter", WorkspaceID: "workspace-one", TaskID: foreign.ID, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign relation filter error = %v, want InvalidArgument", err)
	}
}

func TestExactTaskProjectionIncludesSummaryActivityAndBlockers(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	data := &exactObservationTestData{
		fakeTaskDataSource: &fakeTaskDataSource{
			workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
			tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {task}},
		},
		statusSummaries: map[string]*statussummary.TaskStatusSummary{
			task.ID: {
				Revision: 7, ForegroundActivity: "waiting_for_human", PendingAction: "permission",
				PrimarySession: &statussummary.PrimarySessionSummary{ID: "session-one", State: "waiting_for_input"},
				LaunchQueue:    &statussummary.LaunchQueueSummary{Reason: statussummary.LaunchQueueReasonSessionCapacity},
				ActiveError:    &statussummary.ActiveErrorSummary{Category: "provider_auth"},
			},
		},
	}
	host := newExactTaskReadHost(t, data)
	page, err := host.ListTasks(context.Background(), pluginsdk.ExactTaskQuery{
		RequestID: "task-summary", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("task observation = %+v, err=%v", page.Items, err)
	}
	item := page.Items[0]
	if !item.StatusKnown || item.SemanticActivity != "waiting_for_human" || item.ExecutionState != "waiting_for_input" {
		t.Fatalf("task activity/status = %+v, want summary projection", item)
	}
	wantReasons := []string{"launch_queue:session_capacity", "pending_action:permission", "provider_auth"}
	if len(item.BlockingReasons) != len(wantReasons) {
		t.Fatalf("blocking reasons = %v, want %v", item.BlockingReasons, wantReasons)
	}
	for i, reason := range wantReasons {
		if item.BlockingReasons[i] != reason {
			t.Fatalf("blocking reasons = %v, want %v", item.BlockingReasons, wantReasons)
		}
	}
}

func TestExactSanitizedMessageSnapshot(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	session := &taskmodels.TaskSession{ID: "session-one", TaskID: task.ID, State: "running", UpdatedAt: base}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {task}},
		sessionsByTask:   map[string][]*taskmodels.TaskSession{task.ID: {session}},
	}
	host := newExactReadHost(t, data, []string{"tasks", "messages"}, []string{"host.v2.read:tasks", "host.v2.read:messages"})
	host.messageData = &fakeMessageDataSource{messages: []*taskmodels.Message{{
		ID: "message-one", TaskID: task.ID, TaskSessionID: session.ID, AuthorType: taskmodels.MessageAuthorUser,
		Content: "before <kandev-system>private instruction</kandev-system> after", CreatedAt: base,
	}}}
	page, err := host.ListSanitizedMessages(context.Background(), pluginsdk.ExactMessageQuery{
		RequestID: "messages", WorkspaceID: "workspace-one", Filter: pluginsdk.MessageFilter{TaskIDs: []string{task.ID}},
		Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("sanitized message page = %+v, err=%v", page.Items, err)
	}
	if strings.Contains(page.Items[0].Message.Content, "private instruction") || !strings.Contains(page.Items[0].Message.Content, "before") || page.Items[0].Sanitization != "system_context_removed" {
		t.Fatalf("message content = %+v, want private system content removed", page.Items[0])
	}
}
