package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/proto"
)

// externalProcessTaskWriter is intentionally shaped like backendapp's adapter:
// the Host package cannot import backendapp without an import cycle. The
// backendapp adapter has its own field-mapping unit coverage; this fixture
// proves the external process reaches the real task service and SQLite store.
type externalProcessTaskWriter struct {
	svc                *taskservice.Service
	createWorkspaceIDs []string
	createWorkflowIDs  []string
	createPriorities   []string
	updatePriorities   []string
}

func (w *externalProcessTaskWriter) CreateTask(ctx context.Context, in TaskCreateInput) (*taskmodels.Task, error) {
	w.createWorkspaceIDs = append(w.createWorkspaceIDs, in.WorkspaceID)
	w.createWorkflowIDs = append(w.createWorkflowIDs, in.WorkflowID)
	w.createPriorities = append(w.createPriorities, in.Priority)
	result, err := w.svc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: in.WorkspaceID, WorkflowID: in.WorkflowID, WorkflowStepID: in.WorkflowStepID,
		Title: in.Title, Description: in.Description, ParentID: in.ParentID, Metadata: in.Metadata,
		PlanMode: in.PlanMode, Priority: in.Priority, StartAgent: in.StartAgent,
	})
	return result.Task, err
}

func (w *externalProcessTaskWriter) UpdateTask(ctx context.Context, in TaskUpdateInput) (*taskmodels.Task, error) {
	if in.Priority != nil {
		w.updatePriorities = append(w.updatePriorities, *in.Priority)
	}
	return w.svc.UpdateTask(ctx, in.ID, &taskservice.UpdateTaskRequest{
		Title: in.Title, Description: in.Description, Priority: in.Priority,
	})
}

func (w *externalProcessTaskWriter) DeleteTask(ctx context.Context, id string) error {
	return w.svc.DeleteTask(ctx, id)
}

func (w *externalProcessTaskWriter) MoveTask(context.Context, TaskMoveInput) (*TaskMoveResult, error) {
	return nil, nil
}

type externalPriorityProbeResult struct {
	CreateHighReadback    string `json:"create_high_readback"`
	CreateDefaultReadback string `json:"create_default_readback"`
	UpdateHighReadback    string `json:"update_high_readback"`
	InvalidCreateError    string `json:"invalid_create_error"`
	InvalidUpdateError    string `json:"invalid_update_error"`
}

// TestPluginPriorityWirePayload pins the exact protobuf representation compiled
// into the external fixture: CreateTaskRequest.priority is field 11 (0x5a),
// and optional UpdateTaskRequest.priority is field 6 (0x32).
func TestPluginPriorityWirePayload(t *testing.T) {
	high := "high"
	createPayload, err := proto.Marshal(&pluginv1.CreateTaskRequest{Priority: high})
	require.NoError(t, err)
	require.True(t, bytes.Contains(createPayload, []byte{0x5a, 0x04, 'h', 'i', 'g', 'h'}))
	updatePayload, err := proto.Marshal(&pluginv1.UpdateTaskRequest{Priority: &high})
	require.NoError(t, err)
	require.True(t, bytes.Contains(updatePayload, []byte{0x32, 0x04, 'h', 'i', 'g', 'h'}))
}

func TestPluginHost_ExternalProcessPersistsPriorityThroughTaskService(t *testing.T) {
	ctx := context.Background()
	taskSvc, closeDB := newExternalProcessTaskService(t)
	t.Cleanup(closeDB)

	workspaces, err := taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workflows, err := taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)

	writer := &externalProcessTaskWriter{svc: taskSvc}
	host := &pluginHost{
		pluginID:     "priority-probe",
		capabilities: manifest.Capabilities{APIRead: []string{"tasks"}, APIWrite: []string{"tasks"}},
		taskData:     taskSvc,
		taskWriter:   writer,
		workflows:    taskSvc,
	}

	bin := buildExternalPriorityProbe(t)
	installPath := t.TempDir()
	platform := goruntime.GOOS + "-" + goruntime.GOARCH
	relative := filepath.Join("server", "plugin-"+platform)
	destination := filepath.Join(installPath, relative)
	require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
	contents, err := os.ReadFile(bin)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(destination, contents, 0o755))

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	runtime := pluginruntime.NewManager(t.TempDir(), nil, log)
	t.Cleanup(runtime.StopAll)
	record := &store.Record{Manifest: manifest.Manifest{
		ID: "priority-probe", APIVersion: 1, Version: "1.0.0",
		Runtime: manifest.Runtime{Type: "binary", Executables: map[string]string{platform: relative}},
	}, InstallPath: installPath}
	require.NoError(t, runtime.Start(ctx, record, func(string) pluginsdk.Host { return host }))
	remote, ok := runtime.Get(record.ID)
	require.True(t, ok)
	var response *pluginsdk.WebhookResponse
	require.Eventually(t, func() bool {
		var callErr error
		response, callErr = remote.HandleWebhook(ctx, &pluginsdk.WebhookRequest{WebhookKey: "priority"})
		return callErr == nil && string(response.Body) != "no host"
	}, 5*time.Second, 20*time.Millisecond, "the plugin must receive its Host before the priority probe")
	require.Equalf(t, int32(200), response.Status, "external plugin response: %s", response.Body)

	var probe externalPriorityProbeResult
	require.NoError(t, json.Unmarshal(response.Body, &probe), "plugin response must expose immediate Host readback")
	require.Equal(t, "high", probe.CreateHighReadback)
	require.Equal(t, "medium", probe.CreateDefaultReadback)
	require.Equal(t, "high", probe.UpdateHighReadback)
	require.NotEmpty(t, probe.InvalidCreateError)
	require.NotEmpty(t, probe.InvalidUpdateError)
	require.Equal(t, []string{workspaces[0].ID, workspaces[0].ID}, writer.createWorkspaceIDs)
	require.Equal(t, []string{workflows[0].ID, workflows[0].ID}, writer.createWorkflowIDs)
	require.Equal(t, []string{"high", ""}, writer.createPriorities)
	require.Equal(t, []string{"high"}, writer.updatePriorities)
}

func buildExternalPriorityProbe(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "plugin-fixture")
	command := exec.Command("go", "build", "-o", bin, "../../cmd/plugin-fixture")
	command.Dir = "."
	output, err := command.CombinedOutput()
	require.NoErrorf(t, err, "build external plugin fixture: %s", output)
	return bin
}

func newExternalProcessTaskService(t *testing.T) (*taskservice.Service, func()) {
	t.Helper()
	connection, err := db.OpenSQLite(filepath.Join(t.TempDir(), "priority-probe.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(connection, "sqlite3")
	repository, cleanup, err := taskrepo.Provide(database, database, nil)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	service := taskservice.NewService(taskservice.Repos{
		Workspaces: repository, Tasks: repository, TaskRepos: repository, Workflows: repository,
		Messages: repository, Turns: repository, Sessions: repository, GitSnapshots: repository,
		RepoEntities: repository, RepositorySets: repository, Executors: repository, Environments: repository,
		TaskEnvironments: repository, Reviews: repository, StatusSummaries: repository,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	service.SetWorkspaceBootstrapper(repository)
	return service, func() {
		_ = cleanup()
		_ = database.Close()
	}
}
