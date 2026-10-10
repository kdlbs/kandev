package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type pluginSourceIssueController struct {
	tasks  pluginTaskReader
	jira   pluginJiraSourceService
	linear pluginLinearSourceService
}

type pluginTaskReader interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
}

type pluginJiraSourceService interface {
	GetTaskIssueLink(context.Context, string) (*jira.TaskIssueLink, error)
	GetTicketForWorkspace(context.Context, string, string) (*jira.JiraTicket, error)
	AddCommentForWorkspace(context.Context, string, string, string) (string, error)
	DoTransitionForWorkspace(context.Context, string, string, string) error
}

type pluginLinearSourceService interface {
	GetTaskIssueLink(context.Context, string) (*linear.TaskIssueLink, error)
	GetIssueForWorkspace(context.Context, string, string) (*linear.LinearIssue, error)
	AddCommentForWorkspace(context.Context, string, string, string) (string, error)
	SetIssueStateForWorkspace(context.Context, string, string, string) error
}

type pluginTaskSourceLink struct {
	provider string
	identity string
	url      string
}

const (
	pluginSourceProviderJira        = "jira"
	pluginSourceProviderLinear      = "linear"
	pluginSourceOperationComment    = "comment"
	pluginSourceOperationTransition = "transition"
)

func (c pluginSourceIssueController) GetCapabilities(ctx context.Context, workspaceID, taskID string) (*pluginsdk.SourceIssueCapabilities, error) {
	task, link, err := c.resolveTaskSourceLink(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	base := &pluginsdk.SourceIssueCapabilities{
		WorkspaceID: workspaceID, TaskID: taskID,
		TaskResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Provider:            link.provider, Identifier: link.identity, URL: link.url, Title: task.Title,
	}
	switch link.provider {
	case pluginSourceProviderJira:
		return c.jiraCapabilities(ctx, base, link)
	case pluginSourceProviderLinear:
		return c.linearCapabilities(ctx, base, link)
	default:
		return nil, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnsupported}
	}
}

func (c pluginSourceIssueController) jiraCapabilities(ctx context.Context, result *pluginsdk.SourceIssueCapabilities, link pluginTaskSourceLink) (*pluginsdk.SourceIssueCapabilities, error) {
	result.SourceID = link.identity
	if c.jira == nil {
		result.ResourceVersion = pluginSourceIssueVersion(result, "credentials_unavailable")
		return result, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorMissingCredentials}
	}
	ticket, err := c.jira.GetTicketForWorkspace(ctx, result.WorkspaceID, link.identity)
	if err != nil {
		result.ResourceVersion = pluginSourceIssueVersion(result, "provider_unavailable")
		return result, pluginJiraSourceError(err)
	}
	if ticket == nil || ticket.Key != link.identity {
		return nil, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorNotFound}
	}
	result.SourceID = ticket.ID
	if result.SourceID == "" {
		result.SourceID = link.identity
	}
	result.Title, result.StatusID, result.StatusName = ticket.Summary, ticket.StatusID, ticket.StatusName
	result.Transitions = jiraSourceTransitions(ticket.Transitions)
	result.ResourceVersion = pluginSourceIssueVersion(result, ticket.Updated)
	return result, nil
}

func jiraSourceTransitions(transitions []jira.JiraTransition) []pluginsdk.SourceIssueTransition {
	result := make([]pluginsdk.SourceIssueTransition, 0, len(transitions))
	for _, transition := range transitions {
		if transition.ID == "" {
			continue
		}
		name := transition.ToStatusName
		if name == "" {
			name = transition.Name
		}
		result = append(result, pluginsdk.SourceIssueTransition{TargetID: transition.ID, TargetName: name})
	}
	return result
}

func (c pluginSourceIssueController) linearCapabilities(ctx context.Context, result *pluginsdk.SourceIssueCapabilities, link pluginTaskSourceLink) (*pluginsdk.SourceIssueCapabilities, error) {
	if c.linear == nil {
		result.SourceID = link.identity
		result.ResourceVersion = pluginSourceIssueVersion(result, "credentials_unavailable")
		return result, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorMissingCredentials}
	}
	issue, err := c.linear.GetIssueForWorkspace(ctx, result.WorkspaceID, link.identity)
	if err != nil {
		result.SourceID = link.identity
		result.ResourceVersion = pluginSourceIssueVersion(result, "provider_unavailable")
		return result, pluginLinearSourceError(err)
	}
	if issue == nil || issue.Identifier != link.identity {
		return nil, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorNotFound}
	}
	result.SourceID, result.Title = issue.ID, issue.Title
	result.StatusID, result.StatusName = issue.StateID, issue.StateName
	result.Transitions = linearSourceTransitions(issue.States)
	result.ResourceVersion = pluginSourceIssueVersion(result, issue.Updated)
	return result, nil
}

func linearSourceTransitions(states []linear.LinearWorkflowState) []pluginsdk.SourceIssueTransition {
	result := make([]pluginsdk.SourceIssueTransition, 0, len(states))
	for _, state := range states {
		if state.ID != "" {
			result = append(result, pluginsdk.SourceIssueTransition{TargetID: state.ID, TargetName: state.Name})
		}
	}
	return result
}

func (c pluginSourceIssueController) ApplyWriteback(ctx context.Context, input plugins.SourceIssueWritebackInput, beforeSend func() error) (string, error) {
	if input.Operation != pluginSourceOperationComment && input.Operation != pluginSourceOperationTransition {
		return "", &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnsupported}
	}
	if beforeSend == nil {
		return "", &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnavailable}
	}
	current, err := c.GetCapabilities(ctx, input.WorkspaceID, input.TaskID)
	if err != nil {
		return "", err
	}
	if !sourceIssueVersionMatches(current, input) {
		return "", &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorStale}
	}
	if input.Operation == pluginSourceOperationTransition && !sourceIssueTransitionAllowed(current.Transitions, input.TargetID) {
		return "", &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnsupported}
	}
	if err := beforeSend(); err != nil {
		return "", err
	}
	switch input.Provider {
	case pluginSourceProviderJira:
		return c.applyJiraWriteback(ctx, current, input)
	case pluginSourceProviderLinear:
		return c.applyLinearWriteback(ctx, current, input)
	default:
		return "", &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnsupported}
	}
}

func sourceIssueVersionMatches(current *pluginsdk.SourceIssueCapabilities, input plugins.SourceIssueWritebackInput) bool {
	return current != nil && current.Provider == input.Provider && current.SourceID == input.SourceID &&
		current.TaskResourceVersion == input.ExpectedTaskResourceVersion &&
		current.ResourceVersion == input.ExpectedSourceResourceVersion
}

func sourceIssueTransitionAllowed(transitions []pluginsdk.SourceIssueTransition, targetID string) bool {
	for _, transition := range transitions {
		if transition.TargetID == targetID {
			return true
		}
	}
	return false
}

func (c pluginSourceIssueController) applyJiraWriteback(ctx context.Context, current *pluginsdk.SourceIssueCapabilities, input plugins.SourceIssueWritebackInput) (string, error) {
	if input.Operation == pluginSourceOperationComment {
		commentID, err := c.jira.AddCommentForWorkspace(ctx, input.WorkspaceID, current.Identifier, input.Body)
		if err != nil {
			return "", pluginJiraWriteError(err)
		}
		return commentID, nil
	}
	err := c.jira.DoTransitionForWorkspace(ctx, input.WorkspaceID, current.Identifier, input.TargetID)
	return "", pluginJiraWriteErrorIfNeeded(err)
}

func pluginJiraWriteErrorIfNeeded(err error) error {
	if err == nil {
		return nil
	}
	return pluginJiraWriteError(err)
}

func (c pluginSourceIssueController) applyLinearWriteback(ctx context.Context, current *pluginsdk.SourceIssueCapabilities, input plugins.SourceIssueWritebackInput) (string, error) {
	if input.Operation == pluginSourceOperationComment {
		commentID, err := c.linear.AddCommentForWorkspace(ctx, input.WorkspaceID, current.SourceID, input.Body)
		if err != nil {
			return "", pluginLinearWriteError(err)
		}
		return commentID, nil
	}
	err := c.linear.SetIssueStateForWorkspace(ctx, input.WorkspaceID, current.SourceID, input.TargetID)
	return "", pluginLinearWriteErrorIfNeeded(err)
}

func pluginLinearWriteErrorIfNeeded(err error) error {
	if err == nil {
		return nil
	}
	return pluginLinearWriteError(err)
}

func (c pluginSourceIssueController) resolveTaskSourceLink(ctx context.Context, workspaceID, taskID string) (*taskmodels.Task, pluginTaskSourceLink, error) {
	if c.tasks == nil {
		return nil, pluginTaskSourceLink{}, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnavailable}
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID != workspaceID {
		return nil, pluginTaskSourceLink{}, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorNotFound, Cause: err}
	}
	links := make([]pluginTaskSourceLink, 0, 2)
	jiraLink, found, linkErr := c.resolveJiraTaskSourceLink(ctx, task, workspaceID, taskID)
	if linkErr != nil {
		return nil, pluginTaskSourceLink{}, linkErr
	}
	if found {
		links = append(links, jiraLink)
	}
	linearLink, found, linkErr := c.resolveLinearTaskSourceLink(ctx, task, workspaceID, taskID)
	if linkErr != nil {
		return nil, pluginTaskSourceLink{}, linkErr
	}
	if found {
		links = append(links, linearLink)
	}
	if len(links) == 0 {
		return nil, pluginTaskSourceLink{}, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorNotFound}
	}
	if len(links) != 1 {
		return nil, pluginTaskSourceLink{}, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorStale, Cause: errors.New("task has multiple linked issue providers")}
	}
	return task, links[0], nil
}

//nolint:dupl // Provider-specific link identity and error types stay explicit at this adapter.
func (c pluginSourceIssueController) resolveJiraTaskSourceLink(ctx context.Context, task *taskmodels.Task, workspaceID, taskID string) (pluginTaskSourceLink, bool, error) {
	if c.jira == nil {
		return pluginTaskSourceLink{}, false, nil
	}
	link, err := c.jira.GetTaskIssueLink(ctx, taskID)
	if err != nil {
		if errors.Is(err, jira.ErrSourceLinkNotFound) {
			return pluginTaskSourceLink{}, false, nil
		}
		if errors.Is(err, jira.ErrSourceLinkAmbiguous) {
			return pluginTaskSourceLink{}, false, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorStale, Cause: err}
		}
		return pluginTaskSourceLink{}, false, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnavailable, Cause: err}
	}
	if link == nil || link.WorkspaceID != workspaceID || metadataString(task.Metadata, "jira_issue_key") != link.IssueKey ||
		metadataString(task.Metadata, "jira_issue_url") != link.IssueURL {
		return pluginTaskSourceLink{}, false, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorStale}
	}
	return pluginTaskSourceLink{provider: pluginSourceProviderJira, identity: link.IssueKey, url: link.IssueURL}, true, nil
}

//nolint:dupl // Provider-specific link identity and error types stay explicit at this adapter.
func (c pluginSourceIssueController) resolveLinearTaskSourceLink(ctx context.Context, task *taskmodels.Task, workspaceID, taskID string) (pluginTaskSourceLink, bool, error) {
	if c.linear == nil {
		return pluginTaskSourceLink{}, false, nil
	}
	link, err := c.linear.GetTaskIssueLink(ctx, taskID)
	if err != nil {
		if errors.Is(err, linear.ErrSourceLinkNotFound) {
			return pluginTaskSourceLink{}, false, nil
		}
		if errors.Is(err, linear.ErrSourceLinkAmbiguous) {
			return pluginTaskSourceLink{}, false, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorStale, Cause: err}
		}
		return pluginTaskSourceLink{}, false, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnavailable, Cause: err}
	}
	if link == nil || link.WorkspaceID != workspaceID || metadataString(task.Metadata, "linear_issue_identifier") != link.IssueIdentifier ||
		metadataString(task.Metadata, "linear_issue_url") != link.IssueURL {
		return pluginTaskSourceLink{}, false, &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorStale}
	}
	return pluginTaskSourceLink{provider: pluginSourceProviderLinear, identity: link.IssueIdentifier, url: link.IssueURL}, true, nil
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}

func pluginSourceIssueVersion(item *pluginsdk.SourceIssueCapabilities, providerVersion string) string {
	transitions := append([]pluginsdk.SourceIssueTransition(nil), item.Transitions...)
	sort.Slice(transitions, func(i, j int) bool {
		if transitions[i].TargetID == transitions[j].TargetID {
			return transitions[i].TargetName < transitions[j].TargetName
		}
		return transitions[i].TargetID < transitions[j].TargetID
	})
	value := struct {
		WorkspaceID string
		TaskID      string
		TaskVersion string
		Provider    string
		SourceID    string
		Identifier  string
		URL         string
		ProviderVer string
		StatusID    string
		Transitions []pluginsdk.SourceIssueTransition
	}{item.WorkspaceID, item.TaskID, item.TaskResourceVersion, item.Provider, item.SourceID,
		item.Identifier, item.URL, providerVersion, item.StatusID, transitions}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func pluginJiraSourceError(err error) error {
	return pluginProviderReadError(err, func(status int) plugins.SourceIssueErrorKind {
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return plugins.SourceIssueErrorMissingCredentials
		}
		if status == http.StatusNotFound {
			return plugins.SourceIssueErrorNotFound
		}
		if status == http.StatusTooManyRequests {
			return plugins.SourceIssueErrorRateLimited
		}
		return plugins.SourceIssueErrorUnavailable
	}, func() bool { return errors.Is(err, jira.ErrNotConfigured) })
}

func pluginLinearSourceError(err error) error {
	return pluginProviderReadError(err, func(status int) plugins.SourceIssueErrorKind {
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return plugins.SourceIssueErrorMissingCredentials
		}
		if status == http.StatusNotFound {
			return plugins.SourceIssueErrorNotFound
		}
		if status == http.StatusTooManyRequests {
			return plugins.SourceIssueErrorRateLimited
		}
		return plugins.SourceIssueErrorUnavailable
	}, func() bool { return errors.Is(err, linear.ErrNotConfigured) })
}

func pluginJiraWriteError(err error) error {
	return pluginProviderWriteError(err, func(status int) plugins.SourceIssueErrorKind {
		if status == http.StatusTooManyRequests {
			return plugins.SourceIssueErrorRateLimited
		}
		if status == http.StatusNotFound {
			return plugins.SourceIssueErrorNotFound
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return plugins.SourceIssueErrorMissingCredentials
		}
		if status >= 400 && status < 500 {
			return plugins.SourceIssueErrorUnsupported
		}
		return plugins.SourceIssueErrorUncertain
	}, func() bool { return errors.Is(err, jira.ErrNotConfigured) })
}

func pluginLinearWriteError(err error) error {
	return pluginProviderWriteError(err, func(status int) plugins.SourceIssueErrorKind {
		if status == http.StatusTooManyRequests {
			return plugins.SourceIssueErrorRateLimited
		}
		if status == http.StatusNotFound {
			return plugins.SourceIssueErrorNotFound
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return plugins.SourceIssueErrorMissingCredentials
		}
		if status >= 400 && status < 500 {
			return plugins.SourceIssueErrorUnsupported
		}
		return plugins.SourceIssueErrorUncertain
	}, func() bool { return errors.Is(err, linear.ErrNotConfigured) })
}

func pluginProviderReadError(err error, classify func(int) plugins.SourceIssueErrorKind, missingCredentials func() bool) error {
	if missingCredentials() {
		return &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorMissingCredentials, Cause: err}
	}
	var jiraErr *jira.APIError
	if errors.As(err, &jiraErr) {
		return &plugins.SourceIssueError{Kind: classify(jiraErr.StatusCode), Cause: err}
	}
	var linearErr *linear.APIError
	if errors.As(err, &linearErr) {
		return &plugins.SourceIssueError{Kind: classify(linearErr.StatusCode), Cause: err}
	}
	return &plugins.SourceIssueError{Kind: plugins.SourceIssueErrorUnavailable, Cause: err}
}

func pluginProviderWriteError(err error, classify func(int) plugins.SourceIssueErrorKind, missingCredentials func() bool) error {
	return pluginProviderReadError(err, classify, missingCredentials)
}

var _ plugins.SourceIssueController = pluginSourceIssueController{}
