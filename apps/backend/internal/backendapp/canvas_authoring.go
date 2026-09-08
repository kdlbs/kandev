package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/canvasskill"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/webapp"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

const (
	canvasSourceRootPrefix     = ".kandev/canvases"
	canvasPublishWindow        = 5 * time.Minute
	canvasPublishAttempts      = 10
	canvasSourceCleanupTimeout = 10 * time.Second
	canvasSourceActor          = "agent"
	canvasEditOrigin           = "canvas_edit"
	canvasErrorCodeDefault     = "canvas_error"
	canvasErrorCodeInvalid     = "invalid_canvas"
	canvasInvalidRelease       = "invalid_release"
	// This key is written only by the authenticated canvas edit endpoint. MCP
	// canvas authorization treats it as trusted session state, never as tool
	// input or task metadata.
	canvasEditSessionTargetMetadataKey = "canvas_edit_target"
)

type canvasEditSessionTarget struct {
	Origin    string `json:"origin"`
	TaskID    string `json:"task_id"`
	CanvasID  string `json:"canvas_id"`
	ReleaseID string `json:"release_id"`
}

// canvasAuthoringService is the trusted bridge between the in-session MCP
// handler and the canvas/domain services. It deliberately accepts the
// execution context separately from all user-provided canvas fields.
type canvasAuthoringService struct {
	canvases   *canvas.Service
	plugins    *plugins.Service
	tasks      *taskservice.Service
	executions canvasExecutionResolver
	home       string
	log        *logger.Logger

	mu       sync.Mutex
	attempts map[string][]time.Time
	inflight map[string]bool
}

var _ mcphandlers.CanvasAuthoringService = (*canvasAuthoringService)(nil)

func newCanvasAuthoringService(
	canvases *canvas.Service,
	pluginsSvc *plugins.Service,
	tasks *taskservice.Service,
	executions canvasExecutionResolver,
	home string,
	log *logger.Logger,
) *canvasAuthoringService {
	return &canvasAuthoringService{
		canvases: canvases, plugins: pluginsSvc, tasks: tasks,
		executions: executions, home: home, log: log,
		attempts: make(map[string][]time.Time), inflight: make(map[string]bool),
	}
}

func (s *canvasAuthoringService) ListCanvases(ctx context.Context, request mcphandlers.CanvasListRequest) (any, error) {
	_, task, err := s.resolveExecution(ctx, request.Agent)
	if err != nil {
		return nil, err
	}
	if target, editSession, err := s.canvasEditTarget(ctx, task, request.Agent); err != nil {
		return nil, canvasOperationError("execution_invalid", "the canvas edit session is no longer valid", err)
	} else if editSession {
		item, err := s.authorizedCanvasWithTarget(ctx, task, request.Agent, target, target.CanvasID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"canvases": []canvas.Canvas{*item}}, nil
	}
	items, err := s.canvases.ListForTask(ctx, task.WorkspaceID, task.ID, false)
	if err != nil {
		return nil, canvasOperationError("list_failed", "canvas list is unavailable", err)
	}
	return map[string]any{"canvases": items}, nil
}

func (s *canvasAuthoringService) ReadCanvasAuthoringSkill(ctx context.Context, request mcphandlers.CanvasReadSkillRequest) (any, error) {
	if _, err := s.resolveExecutionOnly(ctx, request.Agent); err != nil {
		return nil, err
	}
	if err := canvasskill.EnsureMaterialized(s.home); err != nil {
		return nil, canvasOperationError("skill_unavailable", "canvas authoring skill is unavailable", err)
	}
	if request.Path == "" {
		bundle, err := canvasCoreBundle(s.home)
		if err != nil {
			return nil, canvasOperationError("skill_unavailable", "canvas authoring skill is unavailable", err)
		}
		return bundle, nil
	}
	content, err := canvasskill.ReadMaterialized(s.home, request.Path)
	if err != nil {
		return nil, canvasOperationError("skill_path_invalid", "canvas authoring skill path is not available", err)
	}
	return map[string]any{
		"slug": canvasskill.Slug, "version": canvasskill.Version,
		"path": request.Path, "content": string(content),
	}, nil
}

func (s *canvasAuthoringService) CreateCanvas(ctx context.Context, request mcphandlers.CanvasCreateRequest) (any, error) {
	execution, task, err := s.resolveExecution(ctx, request.Agent)
	if err != nil {
		return nil, err
	}
	if _, editSession, err := s.canvasEditTarget(ctx, task, request.Agent); err != nil {
		return nil, canvasOperationError("execution_invalid", "the canvas edit session is no longer valid", err)
	} else if editSession {
		return nil, canvasOperationError("canvas_edit_restricted", "canvas edit sessions can only modify their target canvas", nil)
	}
	ownerID := s.workspaceOwner(ctx, task.WorkspaceID)
	created, err := s.canvases.CreateCanvas(ctx, canvas.CreateCanvasRequest{
		WorkspaceID: task.WorkspaceID, TaskID: task.ID,
		OriginTaskID: task.ID, Title: request.Title,
		CreatedBySessionID: taskSessionID(request.Agent),
	})
	if err != nil {
		return nil, canvasOperationError(canvasErrorCode(err), "canvas could not be created", err)
	}
	root := canvasSourceRoot(created.ID)
	client := execution.GetAgentCtlClient()
	if client == nil {
		_ = s.canvases.Remove(ctx, created.ID)
		return nil, canvasOperationError("agent_unavailable", "the task agent is not ready", errors.New("agentctl client is unavailable"))
	}
	if err := canvasskill.EnsureMaterialized(s.home); err != nil {
		_ = s.canvases.Remove(ctx, created.ID)
		return nil, canvasOperationError("skill_unavailable", "canvas authoring skill is unavailable", err)
	}
	scaffold, err := canvasScaffoldFiles(created, request.Summary)
	if err != nil {
		_ = s.canvases.Remove(ctx, created.ID)
		return nil, canvasOperationError("skill_unavailable", "canvas scaffold is unavailable", err)
	}
	// Stage the marker and every scaffold file under an authenticated agentctl
	// path, then rename the complete directory into its assigned root. This
	// keeps local, Docker, SSH, and other executors consistent and lets every
	// failure path remove the complete staged tree.
	if err := materializeCanvasScaffoldAtomically(ctx, client, created.ID, scaffold); err != nil {
		_ = s.canvases.Remove(ctx, created.ID)
		return nil, canvasOperationError("source_unavailable", "canvas scaffold could not be created", err)
	}
	return canvasCreateResponse(created, root, ownerID, scaffold), nil
}

func canvasCreateResponse(created *canvas.Canvas, root, ownerID string, scaffold []canvasScaffoldFile) map[string]any {
	return map[string]any{
		"canvas":      *created,
		"canvas_id":   created.ID,
		"source_path": root,
		"source_root": root,
		"owner_id":    ownerID,
		"skill": map[string]any{
			"slug": canvasskill.Slug, "version": canvasskill.Version,
			"read_tool": "read_canvas_authoring_skill_kandev",
		},
		"manifest_scaffold":  string(scaffold[0].Content),
		"scaffold_inventory": canvasskill.ScaffoldInventory(),
	}
}

func canvasCoreBundle(home string) (map[string]any, error) {
	files := make([]map[string]string, 0, len(canvasskill.CoreInventory()))
	for _, rel := range canvasskill.CoreInventory() {
		content, err := canvasskill.ReadMaterialized(home, rel)
		if err != nil {
			return nil, fmt.Errorf("read canvas core file %q: %w", rel, err)
		}
		files = append(files, map[string]string{"path": rel, "content": string(content)})
	}
	if len(files) == 0 {
		return nil, errors.New("canvas core bundle is empty")
	}
	return map[string]any{
		"slug": canvasskill.Slug, "version": canvasskill.Version,
		"path": "SKILL.md", "content": files[0]["content"],
		"inventory":          canvasskill.Inventory(),
		"core_inventory":     canvasskill.CoreInventory(),
		"scaffold_inventory": canvasskill.ScaffoldInventory(),
		"core": map[string]any{
			"inventory": canvasskill.CoreInventory(),
			"files":     files,
		},
	}, nil
}

type canvasScaffoldFile struct {
	Path    string
	Content []byte
}

func canvasScaffoldFiles(item *canvas.Canvas, summary string) ([]canvasScaffoldFile, error) {
	if item == nil {
		return nil, errors.New("canvas is required for scaffold generation")
	}
	manifest := []byte(canvasManifestScaffold(item, summary))
	templates, err := canvasskill.ScaffoldTemplateFiles()
	if err != nil {
		return nil, err
	}
	files := make([]canvasScaffoldFile, 0, 1+len(templates))
	files = append(files, canvasScaffoldFile{Path: "manifest.yaml", Content: manifest})
	for _, template := range templates {
		files = append(files, canvasScaffoldFile{Path: template.Path, Content: template.Content})
	}
	return files, nil
}

func materializeCanvasScaffold(ctx context.Context, client canvasAgentCtlClient, root string, files []canvasScaffoldFile) error {
	for _, file := range files {
		path := filepath.ToSlash(filepath.Join(root, file.Path))
		if _, err := client.CreateFile(ctx, path, ""); err != nil {
			return fmt.Errorf("create %s: %w", file.Path, err)
		}
		content := string(file.Content)
		if _, err := client.ApplyFileDiff(ctx, path, canvasScaffoldDiff(path, file.Content), "", "", &content); err != nil {
			return fmt.Errorf("write %s: %w", file.Path, err)
		}
	}
	return nil
}

func materializeCanvasScaffoldAtomically(ctx context.Context, client canvasAgentCtlClient, canvasID string, files []canvasScaffoldFile) error {
	stagingRoot := canvasStagingRoot(canvasID)
	finalRoot := canvasSourceRoot(canvasID)
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), canvasSourceCleanupTimeout)
		defer cancel()
		// Delete both paths. A failed rename can leave the staging tree, while a
		// transport error after a successful rename can leave the final tree.
		// DeleteFile is authenticated by the agentctl session and safely ignores
		// whichever path was never created.
		for _, root := range []string{stagingRoot, finalRoot} {
			// Cleanup is best effort, but the original operation error remains
			// the actionable response for the authoring tool.
			_, _ = client.DeleteFile(cleanupCtx, root, "")
		}
	}

	marker := filepath.ToSlash(filepath.Join(stagingRoot, ".canvas-root"))
	if _, err := client.CreateFile(ctx, marker, ""); err != nil {
		cleanup()
		return fmt.Errorf("create canvas marker: %w", err)
	}
	if err := materializeCanvasScaffold(ctx, client, stagingRoot, files); err != nil {
		cleanup()
		return err
	}
	if _, err := client.RenameFile(ctx, stagingRoot, finalRoot, ""); err != nil {
		cleanup()
		return fmt.Errorf("activate canvas source directory: %w", err)
	}
	return nil
}

func canvasScaffoldDiff(path string, content []byte) string {
	value := string(content)
	hasFinalNewline := strings.HasSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\n")
	lines := []string{}
	if value != "" {
		lines = strings.Split(value, "\n")
	}

	var diff strings.Builder
	if len(lines) == 0 {
		return fmt.Sprintf("--- %s\n+++ %s\n@@ -0,0 +0,0 @@\n", path, path)
	}
	fmt.Fprintf(&diff, "--- %s\n+++ %s\n@@ -0,0 +1,%d @@\n", path, path, len(lines))
	for index, line := range lines {
		diff.WriteByte('+')
		diff.WriteString(line)
		diff.WriteByte('\n')
		if index == len(lines)-1 && !hasFinalNewline {
			diff.WriteString("\\ No newline at end of file\n")
		}
	}
	return diff.String()
}

func (s *canvasAuthoringService) GetCanvas(ctx context.Context, request mcphandlers.CanvasGetRequest) (any, error) {
	_, task, err := s.resolveExecution(ctx, request.Agent)
	if err != nil {
		return nil, err
	}
	item, err := s.authorizedCanvas(ctx, task, request.Agent, request.CanvasID)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *canvasAuthoringService) PublishCanvas(ctx context.Context, request mcphandlers.CanvasPublishRequest) (any, error) {
	execution, task, err := s.resolveExecution(ctx, request.Agent)
	if err != nil {
		return nil, err
	}
	target, editSession, err := s.canvasEditTarget(ctx, task, request.Agent)
	if err != nil {
		return nil, canvasOperationError("execution_invalid", "the canvas edit session is no longer valid", err)
	}
	item, err := s.authorizedCanvas(ctx, task, request.Agent, request.CanvasID)
	if err != nil {
		return nil, err
	}
	if err := validateCanvasPublishSource(item, request); err != nil {
		return nil, err
	}
	if err := s.beginPublish(request.Agent.SessionID, item.ID); err != nil {
		return nil, err
	}
	defer s.endPublish(request.Agent.SessionID, item.ID)
	expectedAuthority := instances.PublishAuthority{
		InstanceID: item.PluginInstanceID, ScopeKind: item.ScopeKind,
		WorkspaceID: item.WorkspaceID, TaskID: item.TaskID,
		Status: item.Status, ActiveReleaseID: item.ActiveReleaseID,
		GrantGeneration: item.GrantGeneration,
	}
	expectedBaseReleaseID := ""
	if editSession {
		expectedBaseReleaseID = target.ReleaseID
	}
	return s.publishCanvasSource(ctx, execution, task, item, request, expectedAuthority, expectedBaseReleaseID)
}

func validateCanvasPublishSource(item *canvas.Canvas, request mcphandlers.CanvasPublishRequest) error {
	if request.SourcePath != canvasSourceRoot(item.ID) {
		return canvasOperationError("invalid_source_root", "source_path must be the assigned canvas directory", nil)
	}
	return nil
}

func (s *canvasAuthoringService) publishCanvasSource(ctx context.Context, execution *canvasAgentExecution, task *models.Task, item *canvas.Canvas, request mcphandlers.CanvasPublishRequest, expectedAuthority instances.PublishAuthority, expectedBaseReleaseID string) (any, error) {
	client := execution.GetAgentCtlClient()
	if client == nil {
		return nil, canvasOperationError("agent_unavailable", "the task agent is not ready", nil)
	}
	artifacts := s.plugins.WebArtifacts()
	store := s.plugins.Instances()
	if artifacts == nil || store == nil {
		return nil, canvasOperationError("runtime_unavailable", "canvas release storage is unavailable", nil)
	}
	// Reserve the maximum expanded artifact size before reading agentctl. The
	// exact package size is not known until the bounded stream validates, and
	// this reservation prevents concurrent publishers from bypassing the
	// workspace and installation budget during transfer.
	reservation, err := store.ReserveBytes(ctx, item.WorkspaceID, webapp.MaxExpandedBytes, instances.WorkspaceArtifactLimitBytes, instances.InstallationArtifactLimitBytes)
	if err != nil {
		return nil, canvasOperationError(canvasErrorCode(err), "canvas release storage limit reached", err)
	}
	defer func() { _ = store.ReleaseBytes(context.WithoutCancel(ctx), reservation.ID) }()
	stream, err := client.StreamCanvasSource(ctx, canvasSourceRoot(item.ID))
	if err != nil {
		return nil, canvasOperationError("source_unavailable", "canvas source could not be read", err)
	}
	if stream == nil {
		return nil, canvasOperationError("source_unavailable", "canvas source could not be read", errors.New("agentctl returned an empty source stream"))
	}
	defer func() { _ = stream.Close() }()
	pkg, err := readCanvasSourcePackage(stream)
	if err != nil {
		return nil, err
	}
	if err := store.ResizeReservation(ctx, reservation.ID, pkg.ExpandedBytes, instances.WorkspaceArtifactLimitBytes, instances.InstallationArtifactLimitBytes); err != nil {
		return nil, canvasOperationError(canvasErrorCode(err), "canvas release storage limit reached", err)
	}
	artifact, created, err := artifacts.PutWithCreated(pkg)
	if err != nil {
		return nil, canvasOperationError("artifact_unavailable", "canvas release could not be stored", err)
	}
	releasePersisted := false
	defer func() {
		if created && !releasePersisted {
			_ = artifacts.Remove(artifact)
		}
	}()
	result, err := s.canvases.PublishPackage(ctx, canvas.PublishRequest{
		CanvasID: item.ID, Package: pkg, Artifact: artifact, ExpectedAuthority: expectedAuthority, ExpectedBaseReleaseID: expectedBaseReleaseID,
		SourceActorKind: canvasSourceActor, SourceUserID: s.workspaceOwner(ctx, task.WorkspaceID),
		SourceTaskID: task.ID, SourceSessionID: request.Agent.SessionID,
	})
	if result != nil && result.ReleasePersisted {
		releasePersisted = true
	}
	if err != nil {
		return nil, canvasOperationError(canvasErrorCode(err), "canvas release could not be published", err)
	}
	return map[string]any{
		"canvas":              *result.Canvas,
		"release":             result.Release,
		"activated":           result.Activated,
		"permission_required": result.PermissionRequired,
	}, nil
}

func readCanvasSourcePackage(stream io.Reader) (*webapp.Package, error) {
	counted := &canvasSourceCountingReader{reader: io.LimitReader(stream, int64(types.MaxCanvasSourceWireBytes)+1)}
	pkg, err := webapp.ValidateTarPackageWithLimits(counted, canvasSourcePackageLimits(), counted.bytes)
	if err != nil {
		if counted.bytes > int64(types.MaxCanvasSourceWireBytes) {
			return nil, canvasOperationError("source_limit_exceeded", "canvas source exceeds the transfer limit", nil)
		}
		return nil, canvasOperationError("invalid_release", "canvas source failed validation", err)
	}
	if counted.bytes > int64(types.MaxCanvasSourceWireBytes) {
		return nil, canvasOperationError("source_limit_exceeded", "canvas source exceeds the transfer limit", nil)
	}
	pkg.CompressedBytes = counted.bytes
	return pkg, nil
}

func (s *canvasAuthoringService) GetCanvasState(ctx context.Context, request mcphandlers.CanvasGetStateRequest) (any, error) {
	_, task, err := s.resolveExecution(ctx, request.Agent)
	if err != nil {
		return nil, err
	}
	item, err := s.authorizedCanvas(ctx, task, request.Agent, request.CanvasID)
	if err != nil {
		return nil, err
	}
	store := s.plugins.InstanceState()
	if store == nil {
		return nil, canvasOperationError("state_unavailable", "canvas state is unavailable", nil)
	}
	if request.Key == "" {
		entries, err := store.List(ctx, item.PluginInstanceID)
		if err != nil {
			return nil, canvasOperationError("state_unavailable", "canvas state is unavailable", err)
		}
		return map[string]any{"canvas_id": item.ID, "entries": entries}, nil
	}
	entry, found, err := store.Get(ctx, item.PluginInstanceID, request.Key)
	if err != nil {
		return nil, canvasOperationError("state_unavailable", "canvas state is unavailable", err)
	}
	if !found {
		return nil, canvasOperationError("state_not_found", "canvas state key was not found", nil)
	}
	return map[string]any{"canvas_id": item.ID, "entry": entry}, nil
}

func (s *canvasAuthoringService) SetCanvasState(ctx context.Context, request mcphandlers.CanvasSetStateRequest) (any, error) {
	_, task, err := s.resolveExecution(ctx, request.Agent)
	if err != nil {
		return nil, err
	}
	item, err := s.authorizedCanvas(ctx, task, request.Agent, request.CanvasID)
	if err != nil {
		return nil, err
	}
	store := s.plugins.InstanceState()
	if store == nil {
		return nil, canvasOperationError("state_unavailable", "canvas state is unavailable", nil)
	}
	entry, err := store.Set(ctx, item.PluginInstanceID, request.Key, request.Value, request.ExpectedRevision, "agent")
	if err != nil {
		var conflict *state.ConflictError
		if errors.As(err, &conflict) {
			return nil, &mcphandlers.CanvasAuthoringError{Code: "canvas_state_conflict", Message: "canvas state revision is stale", Details: map[string]interface{}{"current_revision": conflict.CurrentRevision}}
		}
		return nil, canvasOperationError("state_write_failed", "canvas state could not be written", err)
	}
	return map[string]any{"canvas_id": item.ID, "entry": entry}, nil
}

func (s *canvasAuthoringService) resolveExecution(ctx context.Context, agent mcphandlers.CanvasAgentContext) (*canvasAgentExecution, *models.Task, error) {
	execution, err := s.resolveExecutionOnly(ctx, agent)
	if err != nil {
		return nil, nil, err
	}
	if s.tasks == nil || agent.TaskID == "" {
		return nil, nil, canvasOperationError("execution_invalid", "canvas authoring requires a task execution", nil)
	}
	task, err := s.tasks.GetTask(ctx, agent.TaskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return nil, nil, canvasOperationError("task_unavailable", "the task execution is no longer available", err)
	}
	return execution, task, nil
}

func (s *canvasAuthoringService) resolveExecutionOnly(_ context.Context, agent mcphandlers.CanvasAgentContext) (*canvasAgentExecution, error) {
	if s == nil || s.executions == nil || agent.ExecutionID == "" || agent.TaskID == "" || agent.SessionID == "" {
		return nil, canvasOperationError("execution_invalid", "canvas authoring requires a bound task execution", nil)
	}
	execution, err := s.executions.ResolveCanvasExecution(agent.ExecutionID)
	if err != nil || execution == nil || execution.TaskID != agent.TaskID || execution.SessionID != agent.SessionID {
		return nil, canvasOperationError("execution_invalid", "canvas authoring execution is not valid", nil)
	}
	return execution, nil
}

func (s *canvasAuthoringService) authorizedCanvas(ctx context.Context, task *models.Task, agent mcphandlers.CanvasAgentContext, canvasID string) (*canvas.Canvas, error) {
	target, editSession, err := s.canvasEditTarget(ctx, task, agent)
	if err != nil {
		return nil, canvasOperationError("execution_invalid", "the canvas edit session is no longer valid", err)
	}
	if editSession {
		return s.authorizedCanvasWithTarget(ctx, task, agent, target, canvasID)
	}
	return s.authorizedTaskCanvas(ctx, task, agent, canvasID)
}

func (s *canvasAuthoringService) authorizedCanvasWithTarget(ctx context.Context, task *models.Task, agent mcphandlers.CanvasAgentContext, target canvasEditSessionTarget, canvasID string) (*canvas.Canvas, error) {
	if !canvasEditTargetMatches(target, task.ID, canvasID) {
		return nil, canvasOperationError("canvas_not_found", "canvas was not found for this edit session", nil)
	}
	item, err := s.canvases.GetCanvas(ctx, canvasID)
	if err != nil || item == nil || item.WorkspaceID != task.WorkspaceID || item.TaskID != "" || item.ScopeKind != canvas.ScopeWorkspace {
		return nil, canvasOperationError("canvas_not_found", "canvas was not found for this edit session", nil)
	}
	return item, nil
}

func (s *canvasAuthoringService) authorizedTaskCanvas(ctx context.Context, task *models.Task, _ mcphandlers.CanvasAgentContext, canvasID string) (*canvas.Canvas, error) {
	if strings.TrimSpace(canvasID) == "" {
		return nil, canvasOperationError("canvas_not_found", "canvas was not found", nil)
	}
	item, err := s.canvases.GetCanvas(ctx, canvasID)
	if err != nil || !taskCanvasMatchesTask(item, task) {
		return nil, canvasOperationError("canvas_not_found", "canvas was not found for this task", nil)
	}
	return item, nil
}

func taskCanvasMatchesTask(item *canvas.Canvas, task *models.Task) bool {
	return item != nil && task != nil &&
		item.WorkspaceID == task.WorkspaceID &&
		item.TaskID == task.ID &&
		item.ScopeKind == canvas.ScopeTask
}

// canvasEditTarget loads the server-written target binding for an edit
// session. A present but malformed binding remains an edit session and is
// therefore denied by the callers below; it must never fall back to the
// broader task-canvas authorization path.
func (s *canvasAuthoringService) canvasEditTarget(ctx context.Context, task *models.Task, agent mcphandlers.CanvasAgentContext) (canvasEditSessionTarget, bool, error) {
	if s.tasks == nil || task == nil || agent.SessionID == "" {
		return canvasEditSessionTarget{}, false, errors.New("canvas edit session context is unavailable")
	}
	session, err := s.tasks.GetTaskSession(ctx, agent.SessionID)
	if err != nil {
		return canvasEditSessionTarget{}, false, err
	}
	if session == nil || session.TaskID != task.ID {
		return canvasEditSessionTarget{}, false, errors.New("canvas edit session does not belong to task")
	}
	raw, present := session.Metadata[canvasEditSessionTargetMetadataKey]
	if !present {
		return canvasEditSessionTarget{}, false, nil
	}
	target, err := decodeCanvasEditSessionTarget(raw)
	if err != nil {
		if s.log != nil {
			s.log.Warn("malformed canvas edit session target", zap.String("session_id", agent.SessionID), zap.Error(err))
		}
		return canvasEditSessionTarget{}, true, nil
	}
	return target, true, nil
}

func decodeCanvasEditSessionTarget(raw interface{}) (canvasEditSessionTarget, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return canvasEditSessionTarget{}, err
	}
	var target canvasEditSessionTarget
	if err := json.Unmarshal(data, &target); err != nil {
		return canvasEditSessionTarget{}, err
	}
	return target, nil
}

func canvasEditTargetMatches(target canvasEditSessionTarget, taskID, canvasID string) bool {
	return target.Origin == canvasEditOrigin &&
		target.TaskID != "" && target.TaskID == taskID &&
		target.CanvasID != "" && target.CanvasID == canvasID &&
		target.ReleaseID != ""
}

func (s *canvasAuthoringService) workspaceOwner(ctx context.Context, workspaceID string) string {
	if s.tasks != nil {
		if workspace, err := s.tasks.GetWorkspace(ctx, workspaceID); err == nil && workspace != nil && workspace.OwnerID != "" {
			return workspace.OwnerID
		}
	}
	return userstore.DefaultUserID
}

func (s *canvasAuthoringService) beginPublish(sessionID, canvasID string) error {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[canvasID] {
		return canvasOperationError("publish_in_progress", "a canvas publish is already in progress", nil)
	}
	cutoff := now.Add(-canvasPublishWindow)
	window := s.attempts[sessionID][:0]
	for _, attempt := range s.attempts[sessionID] {
		if attempt.After(cutoff) {
			window = append(window, attempt)
		}
	}
	if len(window) >= canvasPublishAttempts {
		s.attempts[sessionID] = window
		return canvasOperationError("publish_rate_limited", "canvas publish rate limit exceeded", nil)
	}
	s.attempts[sessionID] = append(window, now)
	s.inflight[canvasID] = true
	return nil
}

func (s *canvasAuthoringService) endPublish(_ string, canvasID string) {
	s.mu.Lock()
	delete(s.inflight, canvasID)
	s.mu.Unlock()
}

func canvasSourceRoot(canvasID string) string {
	return filepath.ToSlash(filepath.Join(canvasSourceRootPrefix, canvasID))
}

func canvasStagingRoot(canvasID string) string {
	return filepath.ToSlash(filepath.Join(canvasSourceRootPrefix, ".canvas-"+canvasID+".staging"))
}

func canvasSourcePackageLimits() webapp.Limits {
	limits := webapp.DefaultLimits()
	limits.MaxCompressedBytes = int64(types.MaxCanvasSourceWireBytes)
	limits.MaxExpandedBytes = int64(types.MaxCanvasSourceFileData)
	limits.MaxFiles = types.MaxCanvasSourceFiles
	return limits
}

func canvasManifestScaffold(item *canvas.Canvas, summary string) string {
	compactID := strings.ReplaceAll(item.ID, "-", "")
	if len(compactID) > 12 {
		compactID = compactID[:12]
	}
	if compactID == "" {
		compactID = "app"
	}
	id := "canvas-" + compactID
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "Canvas"
	}
	return fmt.Sprintf("id: %q\napi_version: 2\nversion: %q\ndisplay_name: %q\ndescription: %q\nauthor: %q\nui:\n  web_apps:\n    - key: main\n      title: %q\n      entry: index.html\n      placements:\n        - task-canvas\n        - workspace-canvas\ncapabilities:\n  api_read:\n    - tasks\n    - workflows\n  api_write:\n    - messages\n", id, "1.0.0", title, strings.TrimSpace(summary), "Kandev task agent", title)
}

func taskSessionID(agent mcphandlers.CanvasAgentContext) string { return agent.SessionID }

func canvasOperationError(code, message string, cause error) *mcphandlers.CanvasAuthoringError {
	if cause != nil && code == "" {
		code = canvasErrorCodeDefault
	}
	if message == "" {
		message = "canvas operation failed"
	}
	return &mcphandlers.CanvasAuthoringError{Code: code, Message: message}
}

func canvasErrorCode(err error) string {
	switch {
	case errors.Is(err, canvas.ErrTaskCanvasLimit), errors.Is(err, canvas.ErrWorkspaceCanvasLimit):
		return "canvas_limit_exceeded"
	case errors.Is(err, instances.ErrWorkspaceStorageLimit), errors.Is(err, instances.ErrInstallationStorageLimit):
		return "canvas_storage_limit_exceeded"
	case errors.Is(err, instances.ErrInvalidRelease):
		return canvasInvalidRelease
	case errors.Is(err, canvas.ErrStalePromotionReview):
		return "promotion_review_stale"
	case errors.Is(err, canvas.ErrStaleCanvasEdit):
		return "canvas_edit_stale"
	case errors.Is(err, canvas.ErrStaleCanvasPublish):
		return "canvas_publish_stale"
	case errors.Is(err, canvas.ErrInvalidLifecycleState):
		return canvasErrorCodeInvalid
	case errors.Is(err, canvas.ErrCanvasNotFound):
		return "canvas_not_found"
	default:
		return canvasErrorCodeDefault
	}
}

type canvasSourceCountingReader struct {
	reader io.Reader
	bytes  int64
}

func (r *canvasSourceCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytes += int64(n)
	return n, err
}
