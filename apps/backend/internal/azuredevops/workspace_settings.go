package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// QueryPreset is a provider-native browse query offered in the Azure scope bar.
type QueryPreset struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Group   string         `json:"group"`
	Filters map[string]any `json:"filters"`
}

// ActionPreset is a configurable task launcher entry for an Azure item.
type ActionPreset struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Hint           string `json:"hint"`
	Icon           string `json:"icon"`
	PromptTemplate string `json:"promptTemplate"`
}

type WorkspaceSettings struct {
	WorkspaceID        string         `json:"workspaceId"`
	WorkItemQueries    []QueryPreset  `json:"workItemQueries"`
	PullRequestQueries []QueryPreset  `json:"pullRequestQueries"`
	WorkItemActions    []ActionPreset `json:"workItemActions"`
	PullRequestActions []ActionPreset `json:"pullRequestActions"`
}

type UpdateWorkspaceSettingsRequest struct {
	WorkspaceID           string          `json:"-"`
	WorkItemQueries       *[]QueryPreset  `json:"workItemQueries,omitempty"`
	PullRequestQueries    *[]QueryPreset  `json:"pullRequestQueries,omitempty"`
	WorkItemActions       *[]ActionPreset `json:"workItemActions,omitempty"`
	PullRequestActions    *[]ActionPreset `json:"pullRequestActions,omitempty"`
	WorkItemQueriesSet    bool            `json:"-"`
	PullRequestQueriesSet bool            `json:"-"`
	WorkItemActionsSet    bool            `json:"-"`
	PullRequestActionsSet bool            `json:"-"`
}

func (r *UpdateWorkspaceSettingsRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateWorkspaceSettingsRequest
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = UpdateWorkspaceSettingsRequest(decoded)
	if _, ok := raw["workItemQueries"]; ok {
		r.WorkItemQueriesSet = true
	}
	if _, ok := raw["pullRequestQueries"]; ok {
		r.PullRequestQueriesSet = true
	}
	if _, ok := raw["workItemActions"]; ok {
		r.WorkItemActionsSet = true
	}
	if _, ok := raw["pullRequestActions"]; ok {
		r.PullRequestActionsSet = true
	}
	return nil
}

type workspaceSettingsOverrides map[string]json.RawMessage

const workspaceSettingsUpdateAttempts = 3

func DefaultWorkItemQueryPresets() []QueryPreset {
	const start = "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project"
	const order = " ORDER BY [System.ChangedDate] DESC"
	return []QueryPreset{
		{ID: "recent", Label: "Recently updated", Group: "inbox", Filters: map[string]any{"wiql": start + order, "top": 50}},
		{ID: "assigned", Label: "Assigned to me", Group: "inbox", Filters: map[string]any{"wiql": start + " AND [System.AssignedTo] = @Me" + order, "top": 50}},
		{ID: "active", Label: "Active", Group: "inbox", Filters: map[string]any{"wiql": start + " AND [System.State] <> 'Closed' AND [System.State] <> 'Done'" + order, "top": 50}},
		{ID: "created", Label: "Created by me", Group: "created", Filters: map[string]any{"wiql": start + " AND [System.CreatedBy] = @Me" + order, "top": 50}},
	}
}
func DefaultPullRequestQueryPresets() []QueryPreset {
	return []QueryPreset{
		{ID: "review-requested", Label: "Review requested", Group: "inbox", Filters: map[string]any{"status": "active", "reviewer": "@me", "creator": ""}},
		{ID: "active", Label: "Open", Group: "inbox", Filters: map[string]any{"status": "active", "reviewer": "", "creator": ""}},
		{ID: "completed", Label: "Completed", Group: "created", Filters: map[string]any{"status": "completed", "reviewer": "", "creator": ""}},
		{ID: "created", Label: "Created by me", Group: "created", Filters: map[string]any{"status": "active", "reviewer": "", "creator": "@me"}},
	}
}
func DefaultWorkItemActionPresets() []ActionPreset {
	return []ActionPreset{
		{ID: "implement", Label: "Implement", Hint: "Build and open a PR", Icon: "code", PromptTemplate: "Implement the Azure DevOps work item at {{url}} (title: \"{{title}}\"). Open a pull request when complete."},
		{ID: "investigate", Label: "Investigate", Hint: "Find the root cause", Icon: "search", PromptTemplate: "Investigate the Azure DevOps work item at {{url}} (title: \"{{title}}\"). Identify the root cause and summarize findings."},
		{ID: "reproduce", Label: "Reproduce", Hint: "Document repro steps", Icon: "bug", PromptTemplate: "Reproduce the Azure DevOps work item at {{url}} (title: \"{{title}}\"). Document the reproduction steps."},
	}
}
func DefaultPullRequestActionPresets() []ActionPreset {
	return []ActionPreset{
		{ID: "review", Label: "Review", Hint: "Read the diff, flag issues", Icon: "eye", PromptTemplate: "Review the Azure DevOps pull request at {{url}}. Provide feedback on code quality and correctness."},
		{ID: "address-feedback", Label: "Address feedback", Hint: "Apply review comments", Icon: "message", PromptTemplate: "Review and address the feedback on the Azure DevOps pull request at {{url}}."},
		{ID: "fix-ci", Label: "Fix CI", Hint: "Diagnose failing checks", Icon: "tool", PromptTemplate: "Investigate and fix CI failures for the Azure DevOps pull request at {{url}}."},
	}
}

func (s *Service) GetWorkspaceSettings(ctx context.Context, workspaceID string) (*WorkspaceSettings, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	raw, err := s.store.GetWorkspaceSettingsJSON(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var overrides workspaceSettingsOverrides
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return nil, fmt.Errorf("decode Azure workspace settings: %w", err)
	}
	settings := &WorkspaceSettings{WorkspaceID: workspaceID, WorkItemQueries: DefaultWorkItemQueryPresets(), PullRequestQueries: DefaultPullRequestQueryPresets(), WorkItemActions: DefaultWorkItemActionPresets(), PullRequestActions: DefaultPullRequestActionPresets()}
	decodeOverride(overrides, "workItemQueries", &settings.WorkItemQueries)
	decodeOverride(overrides, "pullRequestQueries", &settings.PullRequestQueries)
	decodeOverride(overrides, "workItemActions", &settings.WorkItemActions)
	decodeOverride(overrides, "pullRequestActions", &settings.PullRequestActions)
	return settings, nil
}

func decodeOverride[T any](overrides workspaceSettingsOverrides, key string, target *[]T) {
	raw, ok := overrides[key]
	if !ok || len(raw) == 0 {
		return
	}
	var value []T
	if json.Unmarshal(raw, &value) == nil && len(value) > 0 {
		*target = value
	}
}

func (s *Service) UpdateWorkspaceSettings(ctx context.Context, req *UpdateWorkspaceSettingsRequest) (*WorkspaceSettings, error) {
	if req == nil || req.WorkspaceID == "" {
		return nil, ErrInvalidWorkspaceID
	}
	if err := s.authorizeWorkspaceAccess(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < workspaceSettingsUpdateAttempts; attempt++ {
		snapshot, err := s.store.GetWorkspaceSettingsSnapshot(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}
		next, err := patchWorkspaceSettingsJSON(snapshot.JSON, req)
		if err != nil {
			return nil, err
		}
		updated, err := s.store.PutWorkspaceSettingsJSONIfVersion(ctx, req.WorkspaceID, next, snapshot.Version)
		if err != nil {
			return nil, err
		}
		if updated {
			return s.GetWorkspaceSettings(ctx, req.WorkspaceID)
		}
	}
	return nil, fmt.Errorf("update Azure workspace settings: concurrent update")
}

func patchWorkspaceSettingsJSON(raw string, req *UpdateWorkspaceSettingsRequest) (string, error) {
	overrides := workspaceSettingsOverrides{}
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return "", fmt.Errorf("decode Azure workspace settings: %w", err)
	}
	if err := patchPresetOverride(overrides, "workItemQueries", req.WorkItemQueriesSet, req.WorkItemQueries, normalizeQueryPresets); err != nil {
		return "", err
	}
	if err := patchPresetOverride(overrides, "pullRequestQueries", req.PullRequestQueriesSet, req.PullRequestQueries, normalizeQueryPresets); err != nil {
		return "", err
	}
	if err := patchPresetOverride(overrides, "workItemActions", req.WorkItemActionsSet, req.WorkItemActions, normalizeActionPresets); err != nil {
		return "", err
	}
	if err := patchPresetOverride(overrides, "pullRequestActions", req.PullRequestActionsSet, req.PullRequestActions, normalizeActionPresets); err != nil {
		return "", err
	}
	next, err := json.Marshal(overrides)
	if err != nil {
		return "", err
	}
	return string(next), nil
}

func patchPresetOverride[T any](overrides workspaceSettingsOverrides, key string, set bool, values *[]T, normalize func([]T) []T) error {
	if !set {
		return nil
	}
	if values == nil {
		delete(overrides, key)
		return nil
	}
	normalized := normalize(*values)
	if len(normalized) == 0 {
		return fmt.Errorf("%w: %s must include a valid preset", ErrInvalidConfig, key)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	overrides[key] = raw
	return nil
}

func normalizeQueryPresets(values []QueryPreset) []QueryPreset {
	result := make([]QueryPreset, 0, len(values))
	for _, value := range values {
		value.ID, value.Label, value.Group = strings.TrimSpace(value.ID), strings.TrimSpace(value.Label), strings.TrimSpace(value.Group)
		if value.ID == "" {
			value.ID = uuid.NewString()
		}
		if value.Label == "" || len(value.Filters) == 0 {
			continue
		}
		result = append(result, value)
	}
	return result
}
func normalizeActionPresets(values []ActionPreset) []ActionPreset {
	result := make([]ActionPreset, 0, len(values))
	for _, value := range values {
		value.ID, value.Label, value.Hint, value.Icon, value.PromptTemplate = strings.TrimSpace(value.ID), strings.TrimSpace(value.Label), strings.TrimSpace(value.Hint), strings.TrimSpace(value.Icon), strings.TrimSpace(value.PromptTemplate)
		if value.Label == "" || value.PromptTemplate == "" {
			continue
		}
		if value.ID == "" {
			value.ID = uuid.NewString()
		}
		result = append(result, value)
	}
	return result
}
