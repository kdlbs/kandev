package orchestrator

import (
	"context"
	"errors"
	"reflect"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	builtinStateUnknown  = "unknown"
	builtinStateAccepted = "accepted"
	builtinStateLegacy   = "legacy"
	builtinAvailable     = "available"
	builtinUnavailable   = "unavailable"
)

func (s *Service) SetBuiltinPolicyReader(reader executor.BuiltinPolicyReader) {
	s.executor.SetBuiltinPolicyReader(reader)
}

// BuiltinTaskScope resolves management eligibility through the launch profile resolver.
func (s *Service) BuiltinTaskScope(ctx context.Context, taskID string) (string, p.Surface, error) {
	if err := s.authorizeTask(ctx, taskID); err != nil {
		return "", "", err
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", "", err
	}
	if task == nil {
		return "", "", repoerrors.ErrTaskNotFound
	}
	primaryReader, ok := s.repo.(interface {
		GetPrimarySessionByTaskID(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return "", "", mcpconfig.ErrBuiltinPolicyInvalid
	}
	session, err := primaryReader.GetPrimarySessionByTaskID(ctx, taskID)
	if err != nil && !errors.Is(err, repoerrors.ErrNoPrimarySession) {
		return "", "", err
	}
	if session == nil {
		session = &models.TaskSession{TaskID: taskID}
	}
	profile, err := s.executor.ResolveTaskSessionMCPProfile(ctx, taskID, session, false)
	return task.WorkspaceID, profile.Surface, err
}

// ReadBuiltinSessionMCP returns desired configuration and current acceptance as separate facts.
func (s *Service) ReadBuiltinSessionMCP(ctx context.Context, sessionID string) (*mcpconfig.BuiltinSessionView, error) {
	task, session, err := s.loadBuiltinSessionOwner(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	profile, err := s.executor.PrepareTaskSessionMCPProfile(ctx, task.ID, session, false)
	if err != nil {
		return nil, err
	}
	view := &mcpconfig.BuiltinSessionView{WorkspaceID: task.WorkspaceID, TaskID: task.ID, SessionID: sessionID, Desired: profile.BuiltinPackages, BindingState: builtinStateUnknown, WalkthroughAvailability: builtinStateUnknown}
	if history, ok := runtime.LoadMCPAttachmentHistory(session.Metadata[models.SessionMetaKeyMCPAttachmentState]); ok && history.Current.TaskID == task.ID && history.Current.SessionID == sessionID {
		view.History = &history
	}
	if session.State == models.TaskSessionStateCreated {
		view.WalkthroughAvailability = builtinSelectionAvailability(view.Desired)
		return view, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, err
	}
	if running == nil {
		return view, nil
	}
	if running.TaskID != task.ID || running.SessionID != sessionID {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	view.ExecutionID = running.AgentExecutionID
	applyBuiltinSessionEvidence(view, running)
	return view, nil
}

func applyBuiltinSessionEvidence(view *mcpconfig.BuiltinSessionView, running *models.ExecutorRunning) {
	if running.AgentExecutionID == "" {
		return
	}
	encoded, present := running.Metadata[p.BuiltinBindingMetadataKey]
	if !present {
		view.BindingState = builtinStateLegacy
		if view.History != nil && view.History.Current.ExecutionID == running.AgentExecutionID && view.History.Current.BuiltinPackages == nil {
			view.WalkthroughAvailability = legacyWalkthroughAvailability(view.History.Current)
		}
		return
	}
	profile, err := p.ParseBuiltinBindingMetadata(encoded)
	if err != nil || profile.BuiltinPackages.TaskID != view.TaskID || profile.BuiltinPackages.WorkspaceID != view.WorkspaceID || view.History == nil {
		return
	}
	binding := view.History.BuiltinBinding
	if binding == nil || binding.ExecutionID != running.AgentExecutionID || !reflect.DeepEqual(binding.Manifest, profile.BuiltinPackages) {
		return
	}
	view.BindingState = builtinStateAccepted
	view.Bound = binding
	view.WalkthroughAvailability = builtinSelectionAvailability(binding.Manifest)
}

func builtinSelectionAvailability(manifest *p.BuiltinManifest) string {
	if manifest == nil {
		return builtinStateUnknown
	}
	if builtinWalkthroughSelected(manifest) {
		return builtinAvailable
	}
	return builtinUnavailable
}

func legacyWalkthroughAvailability(attempt streams.MCPAttachmentAttempt) string {
	if legacyWalkthroughObserved(attempt, attempt.ExecutionID) {
		return builtinAvailable
	}
	for _, server := range attempt.Servers {
		if server.Name == "kandev" && server.Source == streams.MCPServerSourceKandev && server.ToolsListedAt != nil && !server.ToolCatalogTruncated {
			return builtinUnavailable
		}
	}
	return builtinStateUnknown
}

func (s *Service) loadBuiltinSessionOwner(ctx context.Context, sessionID string) (*models.Task, *models.TaskSession, error) {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return nil, nil, err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		return nil, nil, models.ErrTaskSessionNotFound
	}
	if err := s.authorizeTask(ctx, session.TaskID); err != nil {
		return nil, nil, err
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if task == nil {
		return nil, nil, repoerrors.ErrTaskNotFound
	}
	return task, session, nil
}
