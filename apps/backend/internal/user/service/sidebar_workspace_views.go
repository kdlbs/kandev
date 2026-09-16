package service

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

// SetSidebarWorkspaceAccess supplies the caller-authorized workspace snapshot.
func (s *Service) SetSidebarWorkspaceAccess(list func(context.Context) ([]string, error)) {
	s.sidebarWorkspaceAccess = list
}

func normalizeSidebarWorkspace(state models.SidebarWorkspaceState) models.SidebarWorkspaceState {
	if len(state.Views) == 0 {
		state.Views = store.DefaultSidebarViews()
	}
	if !sidebarViewIDExists(state.Views, state.ActiveViewID) {
		state.ActiveViewID = state.Views[0].ID
	}
	if state.Draft != nil && (state.Draft.BaseViewID != state.ActiveViewID || !sidebarViewIDExists(state.Views, state.Draft.BaseViewID)) {
		state.Draft = nil
	}
	return state
}

func cloneSidebarWorkspace(state models.SidebarWorkspaceState) (models.SidebarWorkspaceState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return state, err
	}
	var copy models.SidebarWorkspaceState
	err = json.Unmarshal(raw, &copy)
	return normalizeSidebarWorkspace(copy), err
}

func (s *Service) migrateSidebarWorkspaces(ctx context.Context, ids []string) (*models.UserSettings, error) {
	return s.updateUserSettingsCAS(ctx, func(settings *models.UserSettings) (bool, error) {
		if settings.SidebarWorkspaceVersion >= 1 {
			return false, nil
		}
		entries := maps.Clone(settings.SidebarViewsByWorkspace)
		if entries == nil {
			entries = map[string]models.SidebarWorkspaceState{}
		}
		legacy := models.SidebarWorkspaceState{Views: settings.SidebarViews, ActiveViewID: settings.SidebarActiveViewID, Draft: settings.SidebarDraft}
		for _, id := range ids {
			if _, exists := entries[id]; exists {
				continue
			}
			copy, err := cloneSidebarWorkspace(legacy)
			if err != nil {
				return false, err
			}
			entries[id] = copy
		}
		settings.SidebarViewsByWorkspace = entries
		settings.SidebarWorkspaceVersion = 1
		return true, nil
	}, nil)
}

func projectSidebarWorkspaces(settings *models.UserSettings, ids []string) *models.UserSettings {
	copy := *settings
	copy.SidebarViewsByWorkspace = make(map[string]models.SidebarWorkspaceState, len(ids))
	for _, id := range ids {
		copy.SidebarViewsByWorkspace[id] = normalizeSidebarWorkspace(settings.SidebarViewsByWorkspace[id])
	}
	return &copy
}

func (s *Service) getSidebarWorkspaceSettings(ctx context.Context, settings *models.UserSettings) (*models.UserSettings, error) {
	if s.sidebarWorkspaceAccess == nil {
		return settings, nil
	}
	ids, err := s.sidebarWorkspaceAccess(ctx)
	if err != nil {
		return nil, err
	}
	if settings.SidebarWorkspaceVersion < 1 {
		settings, err = s.migrateSidebarWorkspaces(ctx, ids)
		if err != nil {
			return nil, err
		}
	}
	return projectSidebarWorkspaces(settings, ids), nil
}

func (s *Service) validateSidebarWorkspacePatch(ctx context.Context, req *UpdateUserSettingsRequest) error {
	if req.SidebarViewState == nil {
		return nil
	}
	if s.sidebarWorkspaceAccess == nil {
		return fmt.Errorf("%w: workspace access unavailable", ErrValidation)
	}
	ids, err := s.sidebarWorkspaceAccess(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(ids, req.SidebarViewState.WorkspaceID) {
		return fmt.Errorf("%w: invalid sidebar workspace", ErrValidation)
	}
	if hasLegacySidebarPatch(req) {
		return fmt.Errorf("%w: mixed sidebar preference scopes", ErrValidation)
	}
	return nil
}

func hasLegacySidebarPatch(req *UpdateUserSettingsRequest) bool {
	return req.SidebarViews != nil || req.SidebarActiveViewID != nil || req.SidebarDraft != nil
}

func applySidebarWorkspacePatch(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if settings.SidebarWorkspaceVersion >= 1 && hasLegacySidebarPatch(req) {
		return fmt.Errorf("%w: sidebar preferences require workspace scope; refresh the client", ErrValidation)
	}
	patch := req.SidebarViewState
	if patch == nil {
		return nil
	}
	state := normalizeSidebarWorkspace(settings.SidebarViewsByWorkspace[patch.WorkspaceID])
	temporary := &models.UserSettings{SidebarViews: state.Views, SidebarActiveViewID: state.ActiveViewID, SidebarDraft: state.Draft}
	if err := applySidebarViews(temporary, &UpdateUserSettingsRequest{SidebarViews: patch.Views}); err != nil {
		return fmt.Errorf("%w: %s", ErrValidation, err)
	}
	state = normalizeSidebarWorkspace(models.SidebarWorkspaceState{Views: temporary.SidebarViews, ActiveViewID: temporary.SidebarActiveViewID, Draft: temporary.SidebarDraft})
	if patch.ActiveViewID != nil {
		id := strings.TrimSpace(*patch.ActiveViewID)
		if !sidebarViewIDExists(state.Views, id) {
			return fmt.Errorf("%w: active view does not exist in workspace", ErrValidation)
		}
		state.ActiveViewID = id
	}
	if len(patch.Draft) > 0 {
		if err := json.Unmarshal(patch.Draft, &state.Draft); err != nil {
			return fmt.Errorf("%w: invalid sidebar draft", ErrValidation)
		}
		if state.Draft != nil && state.Draft.BaseViewID != state.ActiveViewID {
			return fmt.Errorf("%w: draft does not match active workspace view", ErrValidation)
		}
	}
	state = normalizeSidebarWorkspace(state)
	if reflect.DeepEqual(settings.SidebarViewsByWorkspace[patch.WorkspaceID], state) {
		return nil
	}
	entries := maps.Clone(settings.SidebarViewsByWorkspace)
	if entries == nil {
		entries = map[string]models.SidebarWorkspaceState{}
	}
	entries[patch.WorkspaceID] = state
	settings.SidebarViewsByWorkspace = entries
	return nil
}
