package orchestrator

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/authz"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

type builtinProviderSnapshotReader interface {
	GetAcceptedProviderMCPBinding(context.Context, string, string) (*models.ProviderMCPBindingSnapshot, error)
}

func (s *Service) loadBuiltinProviderView(ctx context.Context, view *mcpconfig.BuiltinSessionView, running *models.ExecutorRunning) error {
	if _, scoped := running.Metadata[p.ProviderMCPBindingMetadataKey]; !scoped {
		return nil
	}
	view.BindingState = builtinStateUnknown
	view.WalkthroughAvailability = builtinStateUnknown
	reader, ok := s.repo.(builtinProviderSnapshotReader)
	if !ok {
		return nil
	}
	snapshot, err := reader.GetAcceptedProviderMCPBinding(ctx, view.SessionID, view.ExecutionID)
	if err != nil || snapshot == nil {
		return err
	}
	accepted, err := builtinProviderView(snapshot)
	if err != nil {
		return err
	}
	if accepted.Scope.ExecutionID != view.ExecutionID || accepted.SessionID != view.SessionID || accepted.Manifest.TaskID != view.TaskID || accepted.Manifest.WorkspaceID != view.WorkspaceID {
		return errors.New("provider MCP snapshot owner changed")
	}
	view.ProviderBinding = accepted
	view.BindingState = builtinStateAccepted
	view.WalkthroughAvailability = builtinSelectionAvailability(accepted.Manifest)
	return nil
}

func builtinProviderView(snapshot *models.ProviderMCPBindingSnapshot) (*mcpconfig.BuiltinProviderBindingView, error) {
	binding, profile, err := p.ParseProviderMCPBinding(snapshot.BindingJSON)
	if err != nil {
		return nil, err
	}
	var catalog map[string][]string
	if err := json.Unmarshal([]byte(snapshot.CatalogJSON), &catalog); err != nil {
		return nil, err
	}
	if snapshot.AcceptedAt == nil || len(catalog) == 0 {
		return nil, errors.New("provider MCP snapshot is unconfirmed")
	}
	return &mcpconfig.BuiltinProviderBindingView{SessionID: snapshot.SessionID, Scope: binding.Scope, Manifest: profile.BuiltinPackages.Clone(), Catalog: catalog, AcceptedAt: *snapshot.AcceptedAt}, nil
}

func (s *Service) canRetryBuiltinApplication(ctx context.Context, view *mcpconfig.BuiltinSessionView, session *models.TaskSession) bool {
	if !builtinApplicationCanRetry(view.Application) || session.State != models.TaskSessionStateWaitingForInput || s.builtinApplicationAuthorize == nil {
		return false
	}

	if _, err := mcpconfig.BuiltinHumanActor(ctx); err != nil {
		return false
	}
	task, err := s.repo.GetTask(ctx, view.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return false
	}
	return s.builtinApplicationAuthorize(ctx, view.WorkspaceID, authz.ScopeTaskWrite) == nil
}

func builtinProviderEffectsClosed(running *models.ExecutorRunning) bool {
	value, present := running.Metadata[p.ProviderMCPEffectsClosedMetadataKey]
	if !present {
		return false
	}
	closed, valid := value.(bool)
	return !valid || closed
}

func (s *Service) retainedProviderBuiltinIntent(ctx context.Context, taskID, sessionID string, running *models.ExecutorRunning) (string, error) {
	if builtinProviderEffectsClosed(running) {
		return "", mcpconfig.ErrBuiltinPolicyInvalid
	}
	reader, ok := s.repo.(builtinProviderSnapshotReader)
	if !ok {
		return "", mcpconfig.ErrBuiltinPolicyInvalid
	}
	snapshot, err := reader.GetAcceptedProviderMCPBinding(ctx, sessionID, running.AgentExecutionID)
	if err != nil {
		return "", err
	}
	if snapshot == nil {
		return "", mcpconfig.ErrBuiltinPolicyInvalid
	}
	accepted, err := builtinProviderView(snapshot)
	if err != nil {
		return "", err
	}
	if accepted.Manifest.TaskID != taskID || accepted.SessionID != sessionID || !builtinWalkthroughSelected(accepted.Manifest) {
		return "", mcpconfig.ErrBuiltinPolicyInvalid
	}
	return accepted.Scope.ExecutionID, nil
}

func builtinApplicationCanRetry(application *models.BuiltinMCPApplication) bool {
	return application != nil && (application.Stage == models.BuiltinMCPApplicationFailed || application.Stage == models.BuiltinMCPApplicationAccepting && application.Reason == "acceptance_unconfirmed")
}
