//nolint:goconst // Provider resource types are stable protocol identifiers kept readable in switches.
package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/settingscatalog"
)

func isIntegrationResource(resourceType string) bool {
	switch resourceType {
	case "jira_settings", "linear_settings", "sentry_instance", "jira_issue_watch", "linear_issue_watch", "sentry_issue_watch",
		"github_settings", "gitlab_settings", "azure_devops_settings", "github_review_watch", "github_issue_watch",
		"gitlab_review_watch", "gitlab_issue_watch", "azure_devops_work_item_watch", "azure_devops_pull_request_watch":
		return true
	default:
		return false
	}
}

//nolint:cyclop // Provider reads stay in one adapter so target validation and redaction are shared.
func (s *settingsOperations) readIntegrationSettings(ctx context.Context, target settingscatalog.ResourceTarget, keys []string) (any, error) {
	if err := s.requireIntegrationDependency(target.ResourceType); err != nil {
		return nil, err
	}
	var value any
	var err error
	switch target.ResourceType {
	case "jira_settings":
		value, err = s.deps.jira.GetConfigForWorkspace(ctx, requireWorkspaceTarget(target))
	case "linear_settings":
		value, err = s.deps.linear.GetConfigForWorkspace(ctx, requireWorkspaceTarget(target))
	case "sentry_instance":
		value, err = s.deps.sentry.GetInstance(ctx, requireWorkspaceTarget(target), requireResourceTarget(target))
	case "jira_issue_watch":
		value, err = s.deps.jira.GetIssueWatch(ctx, requireResourceTarget(target))
	case "linear_issue_watch":
		value, err = s.deps.linear.GetIssueWatch(ctx, requireResourceTarget(target))
	case "sentry_issue_watch":
		value, err = s.deps.sentry.GetIssueWatch(ctx, requireResourceTarget(target))
	case "github_settings":
		value, err = s.deps.github.GetWorkspaceSettings(ctx, requireWorkspaceTarget(target))
	case "gitlab_settings":
		value, err = s.deps.gitlab.GetConfigForWorkspace(ctx, requireWorkspaceTarget(target))
	case "azure_devops_settings":
		value, err = s.deps.azureDevOps.GetWorkspaceSettings(ctx, requireWorkspaceTarget(target))
	case "github_review_watch":
		value, err = s.deps.github.GetReviewWatch(ctx, requireResourceTarget(target))
	case "github_issue_watch":
		value, err = s.deps.github.GetIssueWatch(ctx, requireResourceTarget(target))
	case "gitlab_review_watch":
		value, err = s.deps.gitlab.GetReviewWatch(ctx, requireResourceTarget(target))
	case "gitlab_issue_watch":
		value, err = s.deps.gitlab.GetIssueWatch(ctx, requireResourceTarget(target))
	case "azure_devops_work_item_watch":
		value, err = s.findAzureWorkItemWatch(ctx, requireWorkspaceTarget(target), requireResourceTarget(target))
	case "azure_devops_pull_request_watch":
		value, err = s.findAzurePullRequestWatch(ctx, requireWorkspaceTarget(target), requireResourceTarget(target))
	default:
		return nil, fmt.Errorf("integration settings read is not implemented for resource type %q", target.ResourceType)
	}
	if err != nil {
		return nil, err
	}
	if err := validateIntegrationTargetValue(target, value); err != nil {
		return nil, err
	}
	values := projectJSON(value, s.registry, target.ResourceType, keys)
	redactSettingsValues(values, s.registry, target.ResourceType)
	return map[string]any{
		"target": target,
		"values": values,
		"source": target.ResourceType,
	}, nil
}

//nolint:cyclop // Provider updates stay in one adapter so normalization and projection are shared.
func (s *settingsOperations) updateIntegrationSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage, options map[string]json.RawMessage) (any, error) {
	if err := s.requireIntegrationDependency(target.ResourceType); err != nil {
		return nil, err
	}
	if len(options) != 0 {
		return nil, fmt.Errorf("options are not supported for %q", target.ResourceType)
	}
	normalized, err := normalizeChangeMap(changes, target.ResourceType)
	if err != nil {
		return nil, err
	}
	var updated any
	switch target.ResourceType {
	case "jira_settings":
		updated, err = s.updateJiraSettings(ctx, target, normalized)
	case "linear_settings":
		updated, err = s.updateLinearSettings(ctx, target, normalized)
	case "sentry_instance":
		updated, err = s.updateSentryInstance(ctx, target, normalized)
	case "jira_issue_watch":
		updated, err = s.updateJiraIssueWatch(ctx, target, normalized)
	case "linear_issue_watch":
		updated, err = s.updateLinearIssueWatch(ctx, target, normalized)
	case "sentry_issue_watch":
		updated, err = s.updateSentryIssueWatch(ctx, target, normalized)
	case "github_settings":
		updated, err = s.updateGitHubSettings(ctx, target, normalized)
	case "gitlab_settings":
		return nil, fmt.Errorf("gitlab connection settings are interactive-only")
	case "azure_devops_settings":
		updated, err = s.updateAzureDevOpsSettings(ctx, target, normalized)
	case "github_review_watch":
		updated, err = s.updateGitHubReviewWatch(ctx, target, normalized)
	case "github_issue_watch":
		updated, err = s.updateGitHubIssueWatch(ctx, target, normalized)
	case "gitlab_review_watch":
		updated, err = s.updateGitLabReviewWatch(ctx, target, normalized)
	case "gitlab_issue_watch":
		updated, err = s.updateGitLabIssueWatch(ctx, target, normalized)
	case "azure_devops_work_item_watch":
		updated, err = s.updateAzureWorkItemWatch(ctx, target, normalized)
	case "azure_devops_pull_request_watch":
		updated, err = s.updateAzurePullRequestWatch(ctx, target, normalized)
	default:
		return nil, fmt.Errorf("integration settings update is not implemented for resource type %q", target.ResourceType)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"target": target, "accepted_fields": changePaths(changes), "settings": s.sanitizeSettingsValue(target.ResourceType, updated), "source": target.ResourceType}, nil
}

func (s *settingsOperations) updateJiraSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID := requireWorkspaceTarget(target)
	current, err := s.deps.jira.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("jira settings are not configured")
	}
	request := &jira.SetConfigRequest{
		SiteURL: current.SiteURL, Email: current.Email, AuthMethod: current.AuthMethod,
		InstanceType: current.InstanceType, DefaultProjectKey: current.DefaultProjectKey, ClientID: current.ClientID,
	}
	if err := decodeOptionalChange(changes, "defaultProjectKey", &request.DefaultProjectKey); err != nil {
		return nil, err
	}
	for key := range changes {
		if key != "defaultProjectKey" {
			return nil, fmt.Errorf("setting %q is not writable", key)
		}
	}
	return s.deps.jira.SetConfigForWorkspace(ctx, workspaceID, request)
}

func (s *settingsOperations) updateLinearSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID := requireWorkspaceTarget(target)
	current, err := s.deps.linear.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("linear settings are not configured")
	}
	request := &linear.SetConfigRequest{AuthMethod: current.AuthMethod, DefaultTeamKey: current.DefaultTeamKey}
	if err := decodeOptionalChange(changes, "defaultTeamKey", &request.DefaultTeamKey); err != nil {
		return nil, err
	}
	for key := range changes {
		if key != "defaultTeamKey" {
			return nil, fmt.Errorf("setting %q is not writable", key)
		}
	}
	return s.deps.linear.SetConfigForWorkspace(ctx, workspaceID, request)
}

func (s *settingsOperations) updateSentryInstance(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID, instanceID := requireWorkspaceTarget(target), requireResourceTarget(target)
	current, err := s.deps.sentry.GetInstance(ctx, workspaceID, instanceID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("sentry instance is not configured")
	}
	request := &sentry.UpdateConfigRequest{Name: current.Name, AuthMethod: current.AuthMethod, URL: current.URL}
	if err := decodeOptionalChange(changes, "name", &request.Name); err != nil {
		return nil, err
	}
	for key := range changes {
		if key != "name" {
			return nil, fmt.Errorf("setting %q is not writable", key)
		}
	}
	return s.deps.sentry.UpdateInstance(ctx, workspaceID, instanceID, request)
}

func (s *settingsOperations) updateJiraIssueWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.jira.GetIssueWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request jira.UpdateIssueWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	return s.deps.jira.UpdateIssueWatch(ctx, watch.ID, &request)
}

func (s *settingsOperations) updateLinearIssueWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.linear.GetIssueWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request linear.UpdateIssueWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	return s.deps.linear.UpdateIssueWatch(ctx, watch.ID, &request)
}

func (s *settingsOperations) updateSentryIssueWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.sentry.GetIssueWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request sentry.UpdateIssueWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	return s.deps.sentry.UpdateIssueWatch(ctx, watch.ID, &request)
}

func (s *settingsOperations) updateGitHubSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID := requireWorkspaceTarget(target)
	current, err := s.deps.github.GetWorkspaceSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	request := &github.UpdateWorkspaceSettingsRequest{WorkspaceID: workspaceID}
	if err := decodeOptionalChange(changes, "task_git_credentials_mode", &request.TaskGitCredentialsMode); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "repo_scope_mode", &request.RepoScopeMode); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "repo_scope_orgs", &request.RepoScopeOrgs); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "repo_scope_repos", &request.RepoScopeRepos); err != nil {
		return nil, err
	}
	if err := decodeGitHubRawChange(changes, "saved_presets", &request.SavedPresets, &request.SavedPresetsSet); err != nil {
		return nil, err
	}
	if err := decodeGitHubRawChange(changes, "default_query_presets", &request.DefaultQueryPresets, &request.DefaultQueriesSet); err != nil {
		return nil, err
	}
	if current != nil {
		if request.TaskGitCredentialsMode == nil && request.RepoScopeMode == nil && request.RepoScopeOrgs == nil && request.RepoScopeRepos == nil && !request.SavedPresetsSet && !request.DefaultQueriesSet {
			return current, nil
		}
	}
	return s.deps.github.UpdateWorkspaceSettings(ctx, request)
}

func decodeGitHubRawChange(changes map[string]json.RawMessage, key string, target **json.RawMessage, set *bool) error {
	raw, ok := changes[key]
	if !ok {
		return nil
	}
	if string(raw) == "null" {
		*target = nil
		*set = true
		return nil
	}
	copyValue := append(json.RawMessage(nil), raw...)
	*target, *set = &copyValue, true
	return nil
}

func (s *settingsOperations) updateAzureDevOpsSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID := requireWorkspaceTarget(target)
	request := &azuredevops.UpdateWorkspaceSettingsRequest{WorkspaceID: workspaceID}
	if err := decodeOptionalChange(changes, "workItemQueries", &request.WorkItemQueries); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "pullRequestQueries", &request.PullRequestQueries); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "workItemActions", &request.WorkItemActions); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "pullRequestActions", &request.PullRequestActions); err != nil {
		return nil, err
	}
	request.WorkItemQueriesSet = request.WorkItemQueries != nil
	request.PullRequestQueriesSet = request.PullRequestQueries != nil
	request.WorkItemActionsSet = request.WorkItemActions != nil
	request.PullRequestActionsSet = request.PullRequestActions != nil
	return s.deps.azureDevOps.UpdateWorkspaceSettings(ctx, request)
}

func (s *settingsOperations) updateGitHubReviewWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.github.GetReviewWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request github.UpdateReviewWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	if err := s.deps.github.UpdateReviewWatch(ctx, watch.ID, &request); err != nil {
		return nil, err
	}
	return s.deps.github.GetReviewWatch(ctx, watch.ID)
}

func (s *settingsOperations) updateGitHubIssueWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.github.GetIssueWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request github.UpdateIssueWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	if err := s.deps.github.UpdateIssueWatch(ctx, watch.ID, &request); err != nil {
		return nil, err
	}
	return s.deps.github.GetIssueWatch(ctx, watch.ID)
}

func (s *settingsOperations) updateGitLabReviewWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.gitlab.GetReviewWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request gitlab.UpdateReviewWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	if err := s.deps.gitlab.UpdateReviewWatch(ctx, watch.ID, &request); err != nil {
		return nil, err
	}
	return s.deps.gitlab.GetReviewWatch(ctx, watch.ID)
}

func (s *settingsOperations) updateGitLabIssueWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	watch, err := s.deps.gitlab.GetIssueWatch(ctx, requireResourceTarget(target))
	if err != nil {
		return nil, err
	}
	if err := ensureWatchWorkspace(target, watch.WorkspaceID); err != nil {
		return nil, err
	}
	var request gitlab.UpdateIssueWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	if err := s.deps.gitlab.UpdateIssueWatch(ctx, watch.ID, &request); err != nil {
		return nil, err
	}
	return s.deps.gitlab.GetIssueWatch(ctx, watch.ID)
}

func (s *settingsOperations) updateAzureWorkItemWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID, watchID := requireWorkspaceTarget(target), requireResourceTarget(target)
	watch, err := s.findAzureWorkItemWatch(ctx, workspaceID, watchID)
	if err != nil {
		return nil, err
	}
	var request azuredevops.UpdateWorkItemWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	return s.deps.azureDevOps.UpdateWorkItemWatch(ctx, workspaceID, watch.ID, &request)
}

func (s *settingsOperations) updateAzurePullRequestWatch(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	workspaceID, watchID := requireWorkspaceTarget(target), requireResourceTarget(target)
	watch, err := s.findAzurePullRequestWatch(ctx, workspaceID, watchID)
	if err != nil {
		return nil, err
	}
	var request azuredevops.UpdatePullRequestWatchRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	return s.deps.azureDevOps.UpdatePullRequestWatch(ctx, workspaceID, watch.ID, &request)
}

func requireWorkspaceTarget(target settingscatalog.ResourceTarget) string {
	if target.WorkspaceID == nil {
		return ""
	}
	return strings.TrimSpace(*target.WorkspaceID)
}

func requireResourceTarget(target settingscatalog.ResourceTarget) string {
	if target.ResourceID == nil {
		return ""
	}
	return strings.TrimSpace(*target.ResourceID)
}

func ensureWatchWorkspace(target settingscatalog.ResourceTarget, actual string) error {
	requested := requireWorkspaceTarget(target)
	if requested == "" || actual != requested {
		return fmt.Errorf("resource is not visible in the requested workspace")
	}
	return nil
}

func validateIntegrationTargetValue(target settingscatalog.ResourceTarget, value any) error {
	if value == nil {
		return fmt.Errorf("resource %q was not found", requireResourceTarget(target))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return err
	}
	if workspace, ok := payload["workspaceId"].(string); ok {
		return ensureWatchWorkspace(target, workspace)
	}
	if workspace, ok := payload["workspace_id"].(string); ok {
		return ensureWatchWorkspace(target, workspace)
	}
	return nil
}

func (s *settingsOperations) findAzureWorkItemWatch(ctx context.Context, workspaceID, id string) (*azuredevops.WorkItemWatch, error) {
	items, err := s.deps.azureDevOps.ListWorkItemWatches(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("azure work item watch not found")
}

func (s *settingsOperations) findAzurePullRequestWatch(ctx context.Context, workspaceID, id string) (*azuredevops.PullRequestWatch, error) {
	items, err := s.deps.azureDevOps.ListPullRequestWatches(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("azure pull request watch not found")
}

//nolint:cyclop,gocognit,funlen // Provider lookup keeps each provider's list and display label close to its service call.
func (s *settingsOperations) listIntegrationSettingsResources(ctx context.Context, resourceType string, workspaceID *string, query string, limit int, cursor string) (any, error) {
	if err := s.requireIntegrationDependency(resourceType); err != nil {
		return nil, err
	}
	workspaces, err := s.settingsWorkspaces(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	resources := make([]map[string]any, 0)
	needle := strings.ToLower(strings.TrimSpace(query))
	add := func(id, label, workspace string) {
		if id == "" && workspace == "" {
			return
		}
		if needle != "" && !strings.Contains(strings.ToLower(id), needle) && !strings.Contains(strings.ToLower(label), needle) {
			return
		}
		target := settingscatalog.ResourceTarget{ResourceType: resourceType, WorkspaceID: nonEmptyPointer(workspace)}
		if id != "" {
			target.ResourceID = settingsStringPointer(id)
		}
		resources = append(resources, map[string]any{"target": target, "label": label})
	}
	for _, workspace := range workspaces {
		switch resourceType {
		case "jira_settings":
			add("", "Jira settings ("+workspace.name+")", workspace.id)
		case "linear_settings":
			add("", "Linear settings ("+workspace.name+")", workspace.id)
		case "github_settings":
			add("", "GitHub settings ("+workspace.name+")", workspace.id)
		case "gitlab_settings":
			add("", "GitLab settings ("+workspace.name+")", workspace.id)
		case "azure_devops_settings":
			add("", "Azure DevOps settings ("+workspace.name+")", workspace.id)
		case "sentry_instance":
			instances, listErr := s.deps.sentry.ListInstances(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, instance := range instances {
				if instance != nil {
					add(instance.ID, instance.Name, workspace.id)
				}
			}
		case "jira_issue_watch":
			watches, listErr := s.deps.jira.ListIssueWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.JQL, workspace.id)
				}
			}
		case "linear_issue_watch":
			watches, listErr := s.deps.linear.ListIssueWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.ID, workspace.id)
				}
			}
		case "sentry_issue_watch":
			watches, listErr := s.deps.sentry.ListIssueWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.SentryInstanceID, workspace.id)
				}
			}
		case "github_review_watch":
			watches, listErr := s.deps.github.ListReviewWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.CustomQuery, workspace.id)
				}
			}
		case "github_issue_watch":
			watches, listErr := s.deps.github.ListIssueWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.CustomQuery, workspace.id)
				}
			}
		case "gitlab_review_watch":
			watches, listErr := s.deps.gitlab.ListReviewWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.CustomQuery, workspace.id)
				}
			}
		case "gitlab_issue_watch":
			watches, listErr := s.deps.gitlab.ListIssueWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.CustomQuery, workspace.id)
				}
			}
		case "azure_devops_work_item_watch":
			watches, listErr := s.deps.azureDevOps.ListWorkItemWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.WIQL, workspace.id)
				}
			}
		case "azure_devops_pull_request_watch":
			watches, listErr := s.deps.azureDevOps.ListPullRequestWatches(ctx, workspace.id)
			if listErr != nil {
				return nil, listErr
			}
			for _, watch := range watches {
				if watch != nil {
					add(watch.ID, watch.Status, workspace.id)
				}
			}
		}
	}
	return pageSettingsResources(resources, limit, cursor), nil
}

func (s *settingsOperations) requireIntegrationDependency(resourceType string) error {
	var available bool
	switch resourceType {
	case "jira_settings", "jira_issue_watch":
		available = s.deps.jira != nil
	case "linear_settings", "linear_issue_watch":
		available = s.deps.linear != nil
	case "sentry_instance", "sentry_issue_watch":
		available = s.deps.sentry != nil
	case "github_settings", "github_review_watch", "github_issue_watch":
		available = s.deps.github != nil
	case "gitlab_settings", "gitlab_review_watch", "gitlab_issue_watch":
		available = s.deps.gitlab != nil
	case "azure_devops_settings", "azure_devops_work_item_watch", "azure_devops_pull_request_watch":
		available = s.deps.azureDevOps != nil
	default:
		return fmt.Errorf("unknown integration resource type %q", resourceType)
	}
	if !available {
		return fmt.Errorf("integration settings for %q are unavailable", resourceType)
	}
	return nil
}

type settingsWorkspace struct {
	id   string
	name string
}

func (s *settingsOperations) settingsWorkspaces(ctx context.Context, requested *string) ([]settingsWorkspace, error) {
	if s.deps.task == nil {
		return nil, fmt.Errorf("workspace lookup is unavailable")
	}
	items, err := s.deps.task.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]settingsWorkspace, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if requested != nil && strings.TrimSpace(*requested) != "" && item.ID != strings.TrimSpace(*requested) {
			continue
		}
		result = append(result, settingsWorkspace{id: item.ID, name: item.Name})
	}
	if requested != nil && strings.TrimSpace(*requested) != "" && len(result) == 0 {
		return nil, fmt.Errorf("workspace was not found")
	}
	return result, nil
}
