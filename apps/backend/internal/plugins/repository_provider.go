package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	repositoryBranchesActionKey     = "repositories.branches"
	repositoryActionBodyKey         = "repository"
	maxRepositoryBranchResponseSize = 1 << 20
	maxRepositoryBranchCount        = 10_000
	repositoryBranchActionTimeout   = 15 * time.Second
)

// RepositoryProviderSource is the host-verified identity sent to a plugin
// that owns the repository's provider. Browser input never populates it.
type RepositoryProviderSource struct {
	Provider             string
	ProviderHost         string
	ProviderScope        string
	ProviderRepositoryID string
	OwnerOrProject       string
	Name                 string
	CloneURL             string
	DefaultBranch        string
}

type RepositoryProviderBranch struct {
	Name      string `json:"name"`
	Commit    string `json:"commit,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type repositoryActionInvoker func(
	context.Context,
	string,
	pluginDispatchGeneration,
	*pluginsdk.PluginActionRequest,
) (*pluginsdk.PluginActionResponse, error)

// ListRepositoryProviderBranches invokes the standardized workspace-scoped
// branch action on the active plugin that owns provider.
func (s *Service) ListRepositoryProviderBranches(
	ctx context.Context,
	workspaceID string,
	source RepositoryProviderSource,
) ([]RepositoryProviderBranch, error) {
	return s.listRepositoryProviderBranches(ctx, workspaceID, source, s.InvokeAction)
}

func (s *Service) listRepositoryProviderBranches(
	ctx context.Context,
	workspaceID string,
	source RepositoryProviderSource,
	invoke repositoryActionInvoker,
) ([]RepositoryProviderBranch, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(source.Provider) == "" {
		return nil, fmt.Errorf("plugins: workspace and repository provider are required")
	}
	record, action, err := s.repositoryProviderAction(source.Provider, repositoryBranchesActionKey)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{repositoryActionBodyKey: map[string]any{
		"provider_id":            source.Provider,
		"provider_host":          source.ProviderHost,
		"provider_scope":         source.ProviderScope,
		"provider_repository_id": source.ProviderRepositoryID,
		"owner_or_project":       source.OwnerOrProject,
		"name":                   source.Name,
		"clone_url":              source.CloneURL,
		"default_branch":         source.DefaultBranch,
	}})
	if err != nil {
		return nil, fmt.Errorf("plugins: encode repository branch request: %w", err)
	}
	if len(body) > action.MaxBodyBytes {
		return nil, fmt.Errorf("plugins: repository branch request exceeds declared action limit")
	}
	invokeCtx, cancel := context.WithTimeout(ctx, repositoryBranchActionTimeout)
	defer cancel()
	response, err := invoke(invokeCtx, record.ID, dispatchGeneration(record), &pluginsdk.PluginActionRequest{
		ActionKey: repositoryBranchesActionKey,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: workspaceID},
		Body:      body,
	})
	if err != nil {
		return nil, fmt.Errorf("plugins: list %s repository branches: %w", source.Provider, err)
	}
	return parseRepositoryProviderBranches(response)
}

func (s *Service) repositoryProviderAction(provider, key string) (*store.Record, manifest.Action, error) {
	owner, found := s.registry.activeRepositoryProviderOwner(provider, "")
	if !found {
		return nil, manifest.Action{}, fmt.Errorf("plugins: no active plugin owns repository provider %q", provider)
	}
	record, err := s.Get(owner)
	if err != nil {
		return nil, manifest.Action{}, err
	}
	for _, action := range record.Actions {
		if action.Key == key && action.ResourceScope == manifest.ActionScopeWorkspace {
			return record, action, nil
		}
	}
	return nil, manifest.Action{}, fmt.Errorf("plugins: repository provider %q does not declare %s", provider, key)
}

func parseRepositoryProviderBranches(response *pluginsdk.PluginActionResponse) ([]RepositoryProviderBranch, error) {
	if response == nil || len(response.Body) == 0 {
		return nil, fmt.Errorf("plugins: repository branch action returned an empty response")
	}
	if len(response.Body) > maxRepositoryBranchResponseSize {
		return nil, fmt.Errorf("plugins: repository branch action response exceeds maximum size")
	}
	var payload struct {
		Branches []RepositoryProviderBranch `json:"branches"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, fmt.Errorf("plugins: decode repository branch response: %w", err)
	}
	if len(payload.Branches) > maxRepositoryBranchCount {
		return nil, fmt.Errorf("plugins: repository branch action returned too many branches")
	}
	seen := make(map[string]struct{}, len(payload.Branches))
	branches := make([]RepositoryProviderBranch, 0, len(payload.Branches))
	for _, branch := range payload.Branches {
		branch.Name = strings.TrimSpace(branch.Name)
		if branch.Name == "" {
			return nil, fmt.Errorf("plugins: repository branch action returned an empty branch name")
		}
		if _, found := seen[branch.Name]; found {
			continue
		}
		seen[branch.Name] = struct{}{}
		branches = append(branches, branch)
	}
	return branches, nil
}
