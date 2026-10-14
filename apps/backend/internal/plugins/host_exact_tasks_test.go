package plugins

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
)

type fakeExactTaskCreateWriter struct {
	*fakeTaskWriter
	calls        int
	updateCalls  int
	moveCalls    int
	archiveCalls int
	externalID   string
	lastTask     *taskmodels.Task
	lastUpdate   ExactTaskUpdateInput
	lastMove     ExactTaskMoveInput
	failOnce     bool
}

func (f *fakeExactTaskCreateWriter) CreateTaskExact(_ context.Context, input ExactTaskCreateInput) (*taskmodels.Task, bool, error) {
	f.calls++
	f.externalID = input.ExternalID
	f.lastTask = &taskmodels.Task{
		ID: "task-created", WorkspaceID: input.Task.WorkspaceID,
		Title: input.Task.Title, UpdatedAt: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
	}
	if f.failOnce {
		f.failOnce = false
		return nil, false, context.DeadlineExceeded
	}
	return f.lastTask, f.calls > 1, nil
}

func (f *fakeExactTaskCreateWriter) UpdateTaskExact(_ context.Context, input ExactTaskUpdateInput) (*taskmodels.Task, bool, error) {
	f.updateCalls++
	f.lastUpdate = input
	updated := &taskmodels.Task{
		ID: input.TaskID, WorkspaceID: input.WorkspaceID,
		UpdatedAt: time.Date(2026, 9, 26, 10, 1, 0, 0, time.UTC),
	}
	if input.Labels != nil {
		updated.Labels = "[]"
		if len(*input.Labels) > 0 {
			updated.Labels = `["urgent","coordination"]`
		}
	}
	f.lastTask = updated
	return updated, false, nil
}

func (f *fakeExactTaskCreateWriter) MoveTaskExact(_ context.Context, input ExactTaskMoveInput) (*taskmodels.Task, bool, error) {
	f.moveCalls++
	f.lastMove = input
	updated := &taskmodels.Task{
		ID: input.TaskID, WorkspaceID: input.WorkspaceID,
		WorkflowID: "workflow-task-commands", WorkflowStepID: input.Move.WorkflowStepID,
		UpdatedAt: time.Date(2026, 9, 26, 10, 2, 0, 0, time.UTC),
	}
	f.lastTask = updated
	return updated, false, nil
}

func (f *fakeExactTaskCreateWriter) ArchiveTaskExact(_ context.Context, input ExactTaskArchiveInput) (*taskmodels.Task, bool, error) {
	f.archiveCalls++
	archivedAt := time.Date(2026, 9, 26, 10, 3, 0, 0, time.UTC)
	updated := &taskmodels.Task{
		ID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ArchivedAt: &archivedAt, UpdatedAt: archivedAt,
	}
	f.lastTask = updated
	return updated, false, nil
}

type exactDirectiveTaskData struct {
	taskDataSource
	task    *taskmodels.Task
	session *taskmodels.TaskSession
}

func (d *exactDirectiveTaskData) ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error) {
	return []*taskmodels.Workspace{{ID: d.task.WorkspaceID, UpdatedAt: time.Now().UTC()}}, nil
}

func (d *exactDirectiveTaskData) GetTask(_ context.Context, taskID string) (*taskmodels.Task, error) {
	if d.task == nil || d.task.ID != taskID {
		return nil, repoerrors.ErrTaskNotFound
	}
	task := *d.task
	return &task, nil
}

func (d *exactDirectiveTaskData) ListTaskSessions(_ context.Context, taskID string) ([]*taskmodels.TaskSession, error) {
	if d.session == nil || d.session.TaskID != taskID {
		return nil, nil
	}
	session := *d.session
	return []*taskmodels.TaskSession{&session}, nil
}

type exactDirectiveTransitions struct{ records []PendingTaskTransitionRecord }

func (d exactDirectiveTransitions) ListPendingTaskTransitions(context.Context) ([]PendingTaskTransitionRecord, error) {
	return append([]PendingTaskTransitionRecord(nil), d.records...), nil
}

func newExactTaskCommandHost(t *testing.T) (*pluginHost, *fakeExactTaskCreateWriter) {
	t.Helper()
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "task-command-plugin", Capabilities: manifest.Capabilities{
			APIRead: []string{"task_directives"}, APIWrite: []string{"tasks", "messages", "task_directives"},
		}},
		InstallationID: "task-command-installation",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-task-commands", 1,
		ManifestCapabilityDigest(record.Manifest), []string{"host.v2.read:task_directives", "host.v2.write:tasks", "host.v2.write:messages", "host.v2.write:task_directives"}, "human", "grant", "approval-task-commands"); err != nil {
		t.Fatalf("grant task commands: %v", err)
	}
	writer := &fakeExactTaskCreateWriter{fakeTaskWriter: &fakeTaskWriter{}}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	host.commandStore = commandStore
	host.taskWriter = writer
	return host, writer
}

func TestExactTaskCommands(t *testing.T) {
	host, writer := newExactTaskCommandHost(t)
	input := pluginsdk.ExactTaskCreate{
		RequestID: "request-create", WorkspaceID: "workspace-task-commands", IdempotencyKey: "create-1",
		ExternalID: "coordinator:proposal-17", ApprovalRevision: 1,
		ManifestDigest: ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest),
		Task:           pluginsdk.CreateTaskInput{WorkspaceID: "workspace-task-commands", WorkflowID: "workflow-task-commands", Title: "Add coordinator API"},
	}

	manager, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task commands")
	}
	result, task, err := manager.CreateTask(context.Background(), input)
	if err != nil || task == nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("first CreateTaskExact = result:%+v task:%+v err:%v", result, task, err)
	}
	if writer.calls != 1 || writer.externalID != input.ExternalID {
		t.Fatalf("create calls/external id = %d/%q, want 1/%q", writer.calls, writer.externalID, input.ExternalID)
	}
	host.taskData = &fakeTaskDataSource{tasksByID: map[string]*taskmodels.Task{writer.lastTask.ID: writer.lastTask}}

	replayed, replayedTask, err := manager.CreateTask(context.Background(), input)
	if err != nil || replayedTask == nil || replayed == nil || replayed.Status != pluginsdk.CommandApplied {
		t.Fatalf("retried CreateTaskExact = result:%+v task:%+v err:%v", replayed, replayedTask, err)
	}
	if writer.calls != 1 {
		t.Fatalf("create calls after retry = %d, want no repeated service admission", writer.calls)
	}

	labels := pluginsdk.ExactTaskLabels{
		RequestID: "request-labels", WorkspaceID: input.WorkspaceID, TaskID: writer.lastTask.ID,
		IdempotencyKey: "labels-1", ExpectedResourceVersion: "2026-09-26T10:00:00Z",
		ApprovalRevision: 1, ManifestDigest: input.ManifestDigest,
		Labels: []string{"urgent", "coordination"},
	}
	labelResult, labeledTask, err := manager.SetLabels(context.Background(), labels)
	if err != nil || labeledTask == nil || labelResult == nil || labelResult.Status != pluginsdk.CommandApplied {
		t.Fatalf("SetTaskLabelsExact = result:%+v task:%+v err:%v", labelResult, labeledTask, err)
	}
	if writer.updateCalls != 1 || writer.lastUpdate.Labels == nil || len(*writer.lastUpdate.Labels) != 2 {
		t.Fatalf("exact labels writer calls/input = %d/%+v", writer.updateCalls, writer.lastUpdate)
	}
	host.taskData = &fakeTaskDataSource{tasksByID: map[string]*taskmodels.Task{writer.lastTask.ID: writer.lastTask}}
	labelReplay, replayTask, err := manager.SetLabels(context.Background(), labels)
	if err != nil || replayTask == nil || labelReplay == nil || labelReplay.Status != pluginsdk.CommandApplied || writer.updateCalls != 1 {
		t.Fatalf("replayed SetTaskLabelsExact = result:%+v task:%+v calls:%d err:%v", labelReplay, replayTask, writer.updateCalls, err)
	}
	assignment := pluginsdk.ExactTaskAssignment{
		RequestID: "request-assignment", WorkspaceID: input.WorkspaceID, TaskID: writer.lastTask.ID,
		IdempotencyKey: "assignment-1", ExpectedResourceVersion: "2026-09-26T10:01:00Z",
		ApprovalRevision: 1, ManifestDigest: input.ManifestDigest, AssigneeUserID: "user-42",
	}
	assignmentResult, _, err := manager.Assign(context.Background(), assignment)
	if err != nil || assignmentResult == nil || assignmentResult.Status != pluginsdk.CommandApplied ||
		writer.lastUpdate.AssigneeUserID == nil || *writer.lastUpdate.AssigneeUserID != "user-42" {
		t.Fatalf("AssignTaskExact = result:%+v update:%+v err:%v", assignmentResult, writer.lastUpdate, err)
	}
	move := pluginsdk.ExactTaskMove{
		RequestID: "request-move", WorkspaceID: input.WorkspaceID, TaskID: writer.lastTask.ID,
		IdempotencyKey: "move-1", ExpectedResourceVersion: "2026-09-26T10:02:00Z",
		WorkflowStepID: "step-review", Position: 2, ApprovalRevision: 1, ManifestDigest: input.ManifestDigest,
	}
	moveResult, _, err := manager.Move(context.Background(), move)
	if err != nil || moveResult == nil || moveResult.Status != pluginsdk.CommandApplied || writer.moveCalls != 1 ||
		writer.lastMove.OperationID == "" || writer.lastMove.Move.WorkflowStepID != move.WorkflowStepID {
		t.Fatalf("MoveTaskExact = result:%+v move:%+v calls:%d err:%v", moveResult, writer.lastMove, writer.moveCalls, err)
	}
	moveReplay, _, err := manager.Move(context.Background(), move)
	if err != nil || moveReplay == nil || moveReplay.Status != pluginsdk.CommandApplied || writer.moveCalls != 1 {
		t.Fatalf("replayed MoveTaskExact = result:%+v calls:%d err:%v", moveReplay, writer.moveCalls, err)
	}
	archive := pluginsdk.ExactTaskArchive{
		RequestID: "request-archive", WorkspaceID: input.WorkspaceID, TaskID: writer.lastTask.ID,
		IdempotencyKey: "archive-1", ExpectedResourceVersion: "2026-09-26T10:02:00Z",
		ApprovalRevision: 1, ManifestDigest: input.ManifestDigest,
	}
	archiveResult, archivedTask, err := manager.Archive(context.Background(), archive)
	if err != nil || archivedTask == nil || archivedTask.ArchivedAt == nil || archiveResult == nil ||
		archiveResult.Status != pluginsdk.CommandApplied || writer.archiveCalls != 1 {
		t.Fatalf("ArchiveTaskExact = result:%+v task:%+v calls:%d err:%v", archiveResult, archivedTask, writer.archiveCalls, err)
	}
	archiveReplay, _, err := manager.Archive(context.Background(), archive)
	if err != nil || archiveReplay == nil || archiveReplay.Status != pluginsdk.CommandApplied || writer.archiveCalls != 1 {
		t.Fatalf("replayed ArchiveTaskExact = result:%+v calls:%d err:%v", archiveReplay, writer.archiveCalls, err)
	}

	conflict := input
	conflict.Task.Title = "Different payload"
	conflictResult, _, err := manager.CreateTask(context.Background(), conflict)
	if err != nil || conflictResult == nil || conflictResult.Status != pluginsdk.CommandConflict {
		t.Fatalf("same idempotency key with changed payload = result:%+v err:%v", conflictResult, err)
	}
	if writer.calls != 1 {
		t.Fatalf("changed-payload conflict called task service %d times, want 1", writer.calls)
	}

	wireHost := dialPluginHostOverWire(t, host)
	wireManager, ok := pluginsdk.HostTaskCommands(wireHost)
	if !ok {
		t.Fatal("gRPC Host client does not expose exact task commands")
	}
	wireInput := input
	wireInput.RequestID = "request-wire"
	wireInput.IdempotencyKey = "create-wire"
	wireResult, wireTask, err := wireManager.CreateTask(context.Background(), wireInput)
	if err != nil || wireTask == nil || wireResult == nil || wireResult.Status != pluginsdk.CommandAlreadyApplied {
		t.Fatalf("wire CreateTaskExact = result:%+v task:%+v err:%v", wireResult, wireTask, err)
	}
	wireLabels := labels
	wireLabels.RequestID = "request-labels-wire"
	wireLabels.IdempotencyKey = "labels-wire"
	wireLabels.ExpectedResourceVersion = "2026-09-26T10:01:00Z"
	wireLabelResult, wireLabeledTask, err := wireManager.SetLabels(context.Background(), wireLabels)
	if err != nil || wireLabeledTask == nil || wireLabelResult == nil || wireLabelResult.Status != pluginsdk.CommandApplied {
		t.Fatalf("wire SetTaskLabelsExact = result:%+v task:%+v err:%v", wireLabelResult, wireLabeledTask, err)
	}
	wireMove := move
	wireMove.RequestID = "request-move-wire"
	wireMove.IdempotencyKey = "move-wire"
	wireMove.WorkflowStepID = "step-wire"
	wireMoveResult, wireMovedTask, err := wireManager.Move(context.Background(), wireMove)
	if err != nil || wireMovedTask == nil || wireMoveResult == nil || wireMoveResult.Status != pluginsdk.CommandApplied ||
		wireMovedTask.ID != wireMove.TaskID {
		t.Fatalf("wire MoveTaskExact = result:%+v task:%+v err:%v", wireMoveResult, wireMovedTask, err)
	}
	wireArchive := archive
	wireArchive.RequestID = "request-archive-wire"
	wireArchive.IdempotencyKey = "archive-wire"
	wireArchiveResult, wireArchivedTask, err := wireManager.Archive(context.Background(), wireArchive)
	if err != nil || wireArchivedTask == nil || wireArchiveResult == nil || wireArchiveResult.Status != pluginsdk.CommandApplied {
		t.Fatalf("wire ArchiveTaskExact = result:%+v task:%+v err:%v", wireArchiveResult, wireArchivedTask, err)
	}
}

func TestExactTaskCreateRecoversDomainCommit(t *testing.T) {
	host, writer := newExactTaskCommandHost(t)
	writer.failOnce = true
	manager, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task commands")
	}
	input := pluginsdk.ExactTaskCreate{
		RequestID: "request-recovery", WorkspaceID: "workspace-task-commands", IdempotencyKey: "create-recovery",
		ExternalID: "proposal-recovery", ApprovalRevision: 1,
		ManifestDigest: ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest),
		Task:           pluginsdk.CreateTaskInput{WorkspaceID: "workspace-task-commands", WorkflowID: "workflow-task-commands", Title: "Recovered create"},
	}
	first, _, err := manager.CreateTask(context.Background(), input)
	if err != nil || first == nil || first.Status != pluginsdk.CommandUnavailable {
		t.Fatalf("interrupted create = result:%+v err:%v", first, err)
	}
	recovered, task, err := manager.CreateTask(context.Background(), input)
	if err != nil || task == nil || recovered == nil || recovered.Status != pluginsdk.CommandAlreadyApplied {
		t.Fatalf("recovered create = result:%+v task:%+v err:%v", recovered, task, err)
	}
	if writer.calls != 2 || writer.lastTask.ID != "task-created" {
		t.Fatalf("recovery calls/task = %d/%+v, want one create and one source-identity lookup", writer.calls, writer.lastTask)
	}
}

func TestExactTaskDirectiveIssueResolveAndRead(t *testing.T) {
	host, _ := newExactTaskCommandHost(t)
	now := time.Now().UTC().Truncate(time.Nanosecond)
	data := &exactDirectiveTaskData{
		task:    &taskmodels.Task{ID: "task-directive", WorkspaceID: "workspace-task-commands", UpdatedAt: now},
		session: &taskmodels.TaskSession{ID: "session-directive", TaskID: "task-directive", State: taskmodels.TaskSessionStateWaitingForInput, UpdatedAt: now},
	}
	host.taskData = data
	host.pendingTaskTransitions = func() pendingTaskTransitionSource { return exactDirectiveTransitions{} }
	manager, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		t.Fatal("exact task commands are unavailable")
	}
	manifestDigest := ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest)
	input := pluginsdk.ExactTaskDirectiveIssue{
		RequestID: "directive-issue", WorkspaceID: data.task.WorkspaceID, TaskID: data.task.ID, SessionID: data.session.ID,
		CapabilityClass: "host.v2.write:tasks", InstructionDigest: "sha256:" + strings.Repeat("a", 64),
		ExpectedTaskResourceVersion: now.Format(time.RFC3339Nano), ExpectedSessionResourceVersion: now.Format(time.RFC3339Nano),
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), IdempotencyKey: "directive-issue-1",
		ApprovalRevision: 1, ManifestDigest: manifestDigest,
	}
	result, directive, err := manager.IssueDirective(context.Background(), input)
	if err != nil || result.Status != pluginsdk.CommandApplied || directive.State != "pending" {
		t.Fatalf("IssueDirective = %+v, %+v, %v; want applied pending directive", result, directive, err)
	}
	replayed, directiveReplay, err := manager.IssueDirective(context.Background(), input)
	if err != nil || replayed.Status != pluginsdk.CommandApplied || directiveReplay.ID != directive.ID {
		t.Fatalf("replayed IssueDirective = %+v, %+v, %v; want the same accepted directive", replayed, directiveReplay, err)
	}
	if rows, err := host.commandStoreForDirective().ListTaskDirectives(context.Background(), host.installationID, input.WorkspaceID, input.TaskID); err != nil || len(rows) != 1 {
		t.Fatalf("directive rows = %d, err=%v; want one", len(rows), err)
	}

	resolution := pluginsdk.ExactTaskDirectiveResolve{
		RequestID: "directive-resolve", WorkspaceID: input.WorkspaceID, DirectiveID: directive.ID,
		ExpectedResourceVersion: directive.ResourceVersion, IdempotencyKey: "directive-resolve-1",
		Resolution: "completed", ResolutionDigest: "sha256:" + strings.Repeat("b", 64),
		ApprovalRevision: 1, ManifestDigest: manifestDigest,
	}
	resolved, finalDirective, err := manager.ResolveDirective(context.Background(), resolution)
	if err != nil || resolved.Status != pluginsdk.CommandApplied || finalDirective.State != "resolved" {
		t.Fatalf("ResolveDirective = %+v, %+v, %v; want applied resolved directive", resolved, finalDirective, err)
	}
	queries, ok := pluginsdk.HostExactQueries(host)
	if !ok {
		t.Fatal("exact task directive queries are unavailable")
	}
	observed, _, err := queries.GetTaskDirective(context.Background(), pluginsdk.ExactTaskDirectiveGetQuery{
		RequestID: "directive-read", WorkspaceID: input.WorkspaceID, DirectiveID: directive.ID,
	})
	if err != nil || observed.State != "resolved" || observed.ResourceVersion != finalDirective.ResourceVersion {
		t.Fatalf("GetTaskDirective = %+v, %v; want resolved directive", observed, err)
	}
}

func TestExactTaskDirectiveRejectsSelfDelegationAndFencesChangedSession(t *testing.T) {
	host, _ := newExactTaskCommandHost(t)
	now := time.Now().UTC().Truncate(time.Nanosecond)
	data := &exactDirectiveTaskData{
		task:    &taskmodels.Task{ID: "task-directive", WorkspaceID: "workspace-task-commands", UpdatedAt: now},
		session: &taskmodels.TaskSession{ID: "session-directive", TaskID: "task-directive", State: taskmodels.TaskSessionStateRunning, UpdatedAt: now},
	}
	host.taskData = data
	host.pendingTaskTransitions = func() pendingTaskTransitionSource { return exactDirectiveTransitions{} }
	manager, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		t.Fatal("exact task commands are unavailable")
	}
	manifestDigest := ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest)
	input := pluginsdk.ExactTaskDirectiveIssue{
		RequestID: "directive-self", WorkspaceID: data.task.WorkspaceID, TaskID: data.task.ID, SessionID: data.session.ID,
		CapabilityClass: exactTaskDirectiveCapability, InstructionDigest: "sha256:" + strings.Repeat("a", 64),
		ExpectedTaskResourceVersion: now.Format(time.RFC3339Nano), ExpectedSessionResourceVersion: now.Format(time.RFC3339Nano),
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), IdempotencyKey: "directive-self-1",
		ApprovalRevision: 1, ManifestDigest: manifestDigest,
	}
	result, _, err := manager.IssueDirective(context.Background(), input)
	if err != nil || result.Status != pluginsdk.CommandInvalid {
		t.Fatalf("self-delegating IssueDirective = %+v, %v; want INVALID", result, err)
	}

	input.RequestID = "directive-generation"
	input.IdempotencyKey = "directive-generation-1"
	input.CapabilityClass = "host.v2.write:tasks"
	_, directive, err := manager.IssueDirective(context.Background(), input)
	if err != nil || directive.State != "pending" {
		t.Fatalf("IssueDirective = %+v, %v; want pending", directive, err)
	}
	data.session.UpdatedAt = now.Add(time.Second)
	queries, ok := pluginsdk.HostExactQueries(host)
	if !ok {
		t.Fatal("exact task directive queries are unavailable")
	}
	observed, _, err := queries.GetTaskDirective(context.Background(), pluginsdk.ExactTaskDirectiveGetQuery{
		RequestID: "directive-stale-read", WorkspaceID: input.WorkspaceID, DirectiveID: directive.ID,
	})
	if err != nil || observed.State != "generation_fenced" {
		t.Fatalf("stale GetTaskDirective = %+v, %v; want generation_fenced", observed, err)
	}
}
