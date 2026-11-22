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
	captured := s.captureSidebarGroupingRecord(pluginID)
	record := captured.record
	if record == nil || record.Status != store.StatusActive {
		return nil, unavailable
	}
	action, found := sidebarGroupingAction(record, groupID)
	if !found {
		return nil, unavailable
	}
	generation := captured.generation()
	response, err := invokeSidebarGroupingAction(ctx, record, action, pluginsdk.VerifiedActionContext{
		ActorID: identity.UserID, WorkspaceID: workspaceID,
	}, invoke)
	if err != nil {
		return nil, err
	}
	current := s.captureSidebarGroupingRecord(pluginID)
	if current.record == nil || current.record.Status != store.StatusActive || current.generation() != generation {
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

type sidebarGroupingRecordSnapshot struct {
	record *store.Record
	epoch  uint64
}

func (s *Service) captureSidebarGroupingRecord(id string) sidebarGroupingRecordSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, _ := s.registry.Get(id)
	return sidebarGroupingRecordSnapshot{record: record, epoch: s.sidebarGroupingCatalogGenerations[id]}
}

func (captured sidebarGroupingRecordSnapshot) generation() string {
	record := captured.record
	value, _ := json.Marshal([]any{record.InstallationID, record.Version, record.InstallPath, record.InstalledAt, record.AgentToolPolicyVersion, captured.epoch})
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

// SidebarGroupingCatalog lists active compatible providers, including plugins
// that do not ship a native browser bundle.
func (s *Service) SidebarGroupingCatalog() []pluginsdk.SidebarGroupingProvider {
	return projectSidebarGroupingCatalog(s.captureSidebarGroupingCatalog())
}

func (s *Service) captureSidebarGroupingCatalog() []sidebarGroupingRecordSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := s.registry.List()
	captured := make([]sidebarGroupingRecordSnapshot, len(records))
	for i, record := range records {
		captured[i] = sidebarGroupingRecordSnapshot{record: record, epoch: s.sidebarGroupingCatalogGenerations[record.ID]}
	}
	return captured
}

func projectSidebarGroupingCatalog(captured []sidebarGroupingRecordSnapshot) []pluginsdk.SidebarGroupingProvider {
	providers := make([]pluginsdk.SidebarGroupingProvider, 0)
	for _, snapshot := range captured {
		record := snapshot.record
		if record.Status != store.StatusActive {
			continue
		}
		generation := snapshot.generation()
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
