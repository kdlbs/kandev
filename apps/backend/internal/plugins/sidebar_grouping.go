package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// ResolveSidebarGrouping reads classification through a declared supervised
// action. Task membership is still authorized independently by the repository.
func (s *Service) ResolveSidebarGrouping(ctx context.Context, workspaceID, reference string) (*taskmodels.SidebarPluginGrouping, error) {
	return s.resolveSidebarGrouping(ctx, workspaceID, reference, s.InvokeAction)
}

func (s *Service) resolveSidebarGrouping(ctx context.Context, workspaceID, reference string, invoke repositoryActionInvoker) (*taskmodels.SidebarPluginGrouping, error) {
	unavailable := taskmodels.ErrSidebarGroupingUnavailable
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	identity, authenticated := authn.IdentityFromContext(ctx)
	pluginID, groupID, valid := pluginsdk.ParseSidebarGroupingReference(reference)
	if !authenticated || identity.UserID == "" || !valid || !s.sidebarGroupingWorkspaceVisible(ctx, workspaceID) {
		return nil, unavailable
	}
	record, err := s.Get(pluginID)
	if err != nil || record.Status != store.StatusActive {
		return nil, unavailable
	}
	action, found := sidebarGroupingAction(record, groupID)
	if !found {
		return nil, unavailable
	}
	generation := s.sidebarGroupingGeneration(record)
	response, err := invokeSidebarGroupingAction(ctx, record, action, pluginsdk.VerifiedActionContext{
		ActorID: identity.UserID, WorkspaceID: workspaceID,
	}, invoke)
	if err != nil {
		return nil, err
	}
	current, err := s.Get(pluginID)
	if err != nil || current.Status != store.StatusActive || s.sidebarGroupingGeneration(current) != generation {
		return nil, unavailable
	}
	snapshot, err := pluginsdk.DecodeSidebarGroupingSnapshot(response.Body)
	if err != nil {
		return nil, unavailable
	}
	return &taskmodels.SidebarPluginGrouping{Reference: reference, Generation: generation, Snapshot: snapshot}, nil
}

func (s *Service) sidebarGroupingWorkspaceVisible(ctx context.Context, workspaceID string) bool {
	if s.taskData == nil {
		return false
	}
	workspaces, err := s.taskData.ListWorkspaces(ctx)
	return err == nil && containsWorkspace(workspaces, workspaceID)
}

func invokeSidebarGroupingAction(ctx context.Context, record *store.Record, action string, actor pluginsdk.VerifiedActionContext, invoke repositoryActionInvoker) (*pluginsdk.PluginActionResponse, error) {
	invokeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := invoke(invokeCtx, record.ID, dispatchGeneration(record), &pluginsdk.PluginActionRequest{
		ActionKey: action, Context: actor, Body: json.RawMessage(`{}`),
	})
	if invokeCtx.Err() != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, taskmodels.ErrSidebarGroupingUnavailable
	}
	if err != nil || response == nil || (response.Status != 0 && response.Status != 200) {
		return nil, taskmodels.ErrSidebarGroupingUnavailable
	}
	return response, nil
}

func sidebarGroupingAction(record *store.Record, groupID string) (string, bool) {
	for _, group := range record.SidebarGroupings {
		if group.ID != groupID {
			continue
		}
		for _, action := range record.Actions {
			if action.Key == group.Action && action.ResourceScope == manifest.ActionScopeWorkspace && action.EffectiveAccess() == manifest.ActionAccessAuthenticated {
				return action.Key, true
			}
		}
	}
	return "", false
}

func (s *Service) sidebarGroupingGeneration(record *store.Record) string {
	value, _ := json.Marshal([]any{record.InstallationID, record.Version, record.InstallPath, record.InstalledAt, record.AgentToolPolicyVersion, s.sidebarGroupingCatalogGeneration.Load()})
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

// SidebarGroupingCatalog lists active compatible providers, including plugins
// that do not ship a native browser bundle.
func (s *Service) SidebarGroupingCatalog() []pluginsdk.SidebarGroupingProvider {
	providers := make([]pluginsdk.SidebarGroupingProvider, 0)
	for _, record := range s.registry.List() {
		if record.Status != store.StatusActive {
			continue
		}
		generation := s.sidebarGroupingGeneration(record)
		for _, group := range record.SidebarGroupings {
			if _, found := sidebarGroupingAction(record, group.ID); !found {
				continue
			}
			reference := "plugin:" + record.ID + ":" + group.ID
			if _, _, valid := pluginsdk.ParseSidebarGroupingReference(reference); !valid {
				continue
			}
			providers = append(providers, pluginsdk.SidebarGroupingProvider{Reference: reference, Label: group.Label, LabelKey: group.LabelKey, Generation: generation})
		}
	}
	return providers
}
