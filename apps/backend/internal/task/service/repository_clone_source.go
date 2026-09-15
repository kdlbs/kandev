package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// LocalRepositoryCloneSource is the server-owned result of inspecting a host
// checkout before it is offered to a remote executor. The origin is always
// credential-free and branches come from the origin, not local-only refs.
type LocalRepositoryCloneSource struct {
	Ready         bool
	Origin        string
	Reason        string
	CurrentBranch string
	DefaultBranch string
	Branches      []Branch
}

const checkoutSourceRemoteOrigin = "remote_origin"

// resolveRemoteOriginInput turns a host checkout into the credential-free
// provider descriptor used by a remote executor. The browser may provide the
// previously inspected origin only as an equality check; the checkout itself
// is always read again here.
func (s *Service) resolveRemoteOriginInput(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	if input.CheckoutSource == "" {
		return input, nil
	}
	if err := validateRemoteOriginInput(input); err != nil {
		return input, err
	}
	localPath, err := s.resolveRemoteOriginLocalPath(ctx, workspaceID, input)
	if err != nil {
		return input, err
	}
	inspection, err := inspectLocalRepositoryCloneSourcePath(ctx, localPath)
	if err != nil {
		return input, fmt.Errorf("%w: inspect local repository: %v", ErrInvalidWorkspaceSource, err)
	}
	if err := validateRemoteOriginInspection(input, inspection); err != nil {
		return input, err
	}
	baseBranch, checkoutBranch, err := resolveRemoteOriginBranches(input, inspection)
	if err != nil {
		return input, err
	}
	input.RepositoryID = ""
	input.LocalPath = ""
	input.RemoteURL = inspection.Origin
	input.BaseBranch = baseBranch
	input.CheckoutBranch = checkoutBranch
	return input, nil
}

func validateRemoteOriginInput(input TaskRepositoryInput) error {
	if input.CheckoutSource != checkoutSourceRemoteOrigin {
		return fmt.Errorf("%w: unsupported checkout_source %q", ErrInvalidWorkspaceSource, input.CheckoutSource)
	}
	if strings.TrimSpace(input.RemoteURL) != "" || strings.TrimSpace(input.GitHubURL) != "" ||
		input.Provider != "" || input.ProviderHost != "" || input.ProviderScope != "" ||
		input.ProviderRepoID != "" || input.ProviderOwner != "" || input.ProviderName != "" {
		return fmt.Errorf("%w: remote_origin cannot include a client-supplied remote descriptor", ErrInvalidWorkspaceSource)
	}
	return nil
}

func (s *Service) resolveRemoteOriginLocalPath(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) (string, error) {
	localPath := strings.TrimSpace(input.LocalPath)
	if input.RepositoryID == "" {
		if localPath == "" {
			return "", fmt.Errorf("%w: remote_origin requires a host checkout", ErrInvalidWorkspaceSource)
		}
		return localPath, nil
	}
	repository, err := s.repoEntities.GetRepository(ctx, input.RepositoryID)
	if err != nil {
		return "", err
	}
	if repository == nil || repository.WorkspaceID != workspaceID {
		return "", fmt.Errorf("%w: repository %q", taskrepository.ErrRepositoryNotFound, input.RepositoryID)
	}
	if localPath == "" {
		localPath = repository.LocalPath
	}
	if localPath == "" {
		return "", fmt.Errorf("%w: remote_origin requires a host checkout", ErrInvalidWorkspaceSource)
	}
	return localPath, nil
}

func validateRemoteOriginInspection(
	input TaskRepositoryInput, inspection LocalRepositoryCloneSource,
) error {
	if !inspection.Ready {
		reason := inspection.Reason
		if reason == "" {
			reason = "origin_unavailable"
		}
		return fmt.Errorf("%w: local repository origin is unavailable (%s)", ErrUnsupportedWorkspaceSource, reason)
	}
	expected := strings.TrimSpace(input.ExpectedOrigin)
	if expected == "" {
		return fmt.Errorf("%w: remote_origin requires expected_origin", ErrInvalidWorkspaceSource)
	}
	expectedOrigin, err := credentialFreeCloneOrigin(expected)
	if err != nil || expectedOrigin != inspection.Origin {
		return fmt.Errorf("%w: local repository origin changed; refresh the repository", ErrInvalidWorkspaceSource)
	}
	return nil
}

func resolveRemoteOriginBranches(
	input TaskRepositoryInput, inspection LocalRepositoryCloneSource,
) (string, string, error) {
	baseBranch, err := normalizeRemoteOriginBranch(input.BaseBranch)
	if err != nil {
		return "", "", err
	}
	checkoutBranch, err := normalizeRemoteOriginBranch(input.CheckoutBranch)
	if err != nil {
		return "", "", err
	}
	if baseBranch != "" && !remoteOriginHasBranch(inspection.Branches, baseBranch) {
		return "", "", fmt.Errorf("%w: remote branch %q is unavailable", ErrInvalidWorkspaceSource, input.BaseBranch)
	}
	if checkoutBranch != "" && !remoteOriginHasBranch(inspection.Branches, checkoutBranch) {
		return "", "", fmt.Errorf("%w: remote branch %q is unavailable", ErrInvalidWorkspaceSource, input.CheckoutBranch)
	}
	if baseBranch == "" && inspection.CurrentBranch != "" && remoteOriginHasBranch(inspection.Branches, inspection.CurrentBranch) {
		baseBranch = inspection.CurrentBranch
	}
	return baseBranch, checkoutBranch, nil
}

func normalizeRemoteOriginBranch(branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	branch = strings.TrimPrefix(branch, "origin/")
	if branch != "" && !securityutil.IsValidBranchName(branch) {
		return "", fmt.Errorf("%w: remote branch %q is not a safe git branch", ErrInvalidWorkspaceSource, branch)
	}
	return branch, nil
}

func remoteOriginHasBranch(branches []Branch, branch string) bool {
	for _, candidate := range branches {
		if candidate.Name == branch {
			return true
		}
	}
	return false
}

// InspectLocalRepositoryCloneSource validates a workspace repository or an
// explicitly selected checkout and reads its origin and remote branches. A
// missing or unreachable origin is represented as a non-ready result so the
// picker can keep the row visible and recover without losing the selection.
func (s *Service) InspectLocalRepositoryCloneSource(
	ctx context.Context,
	workspaceID, repositoryID, localPath string,
) (LocalRepositoryCloneSource, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return LocalRepositoryCloneSource{}, err
	}
	if (repositoryID == "") == (strings.TrimSpace(localPath) == "") {
		return LocalRepositoryCloneSource{}, fmt.Errorf("repository_id or local_path is required")
	}
	if repositoryID != "" {
		repository, err := s.repoEntities.GetRepository(ctx, repositoryID)
		if err != nil {
			return LocalRepositoryCloneSource{}, err
		}
		if repository == nil || repository.WorkspaceID != workspaceID {
			return LocalRepositoryCloneSource{}, repoerrors.ErrRepositoryNotFound
		}
		localPath = repository.LocalPath
	}
	return inspectLocalRepositoryCloneSourcePath(ctx, localPath)
}

func inspectLocalRepositoryCloneSourcePath(ctx context.Context, localPath string) (LocalRepositoryCloneSource, error) {
	resolved, _, err := resolveExplicitLocalRepositoryPath(strings.TrimSpace(localPath))
	if err != nil {
		return LocalRepositoryCloneSource{}, err
	}
	result := LocalRepositoryCloneSource{CurrentBranch: readExplicitGitCurrentBranch(resolved)}
	rawOrigin, err := readGitRemoteOriginURL(resolved)
	if err != nil || strings.TrimSpace(rawOrigin) == "" {
		result.Reason = "missing_origin"
		return result, nil
	}
	origin, err := credentialFreeCloneOrigin(rawOrigin)
	if err != nil {
		result.Reason = err.Error()
		return result, nil
	}
	result.Origin = origin
	branches, err := listRemoteOriginBranches(ctx, resolved)
	if err != nil {
		result.Reason = "origin_unavailable"
		return result, nil
	}
	result.Ready = len(branches) > 0
	result.Branches = branches
	result.DefaultBranch = remoteOriginDefaultBranch(branches, result.CurrentBranch)
	if !result.Ready {
		result.Reason = "origin_has_no_branches"
	}
	return result, nil
}

func remoteOriginDefaultBranch(branches []Branch, current string) string {
	if remoteOriginHasBranch(branches, current) {
		return current
	}
	for _, preferred := range []string{"main", "master", "develop"} {
		if remoteOriginHasBranch(branches, preferred) {
			return preferred
		}
	}
	if len(branches) > 0 {
		return branches[0].Name
	}
	return ""
}

func credentialFreeCloneOrigin(raw string) (string, error) {
	parsed, scpStyle, err := normalizeRemoteRepositoryURL(raw)
	if err != nil {
		return "", errors.New("unsupported_origin")
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return "", errors.New("origin_credentials")
		}
		if !scpStyle && (parsed.Scheme == protocolHTTP || parsed.Scheme == protocolHTTPS) {
			return "", errors.New("origin_credentials")
		}
	}
	if parsed.Scheme != protocolHTTP && parsed.Scheme != protocolHTTPS && parsed.Scheme != "ssh" && parsed.Scheme != "git" {
		return "", errors.New("unsupported_origin")
	}
	if parsed.Hostname() == "" || parsed.Path == "" || strings.Contains(parsed.RawQuery, "@") {
		return "", errors.New("unsupported_origin")
	}
	if scpStyle {
		return strings.TrimSpace(raw), nil
	}
	return parsed.String(), nil
}

func listRemoteOriginBranches(ctx context.Context, repoPath string) ([]Branch, error) {
	cmd := subproc.NewGitCommand(ctx, "ls-remote", "--heads", "origin")
	cmd.Dir = repoPath
	out, err := subproc.RunGitOutputClass(ctx, subproc.GitInteractive, cmd)
	if err != nil {
		return nil, err
	}
	branches := make([]Branch, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(fields[1], "refs/heads/")
		if !securityutil.IsValidBranchName(name) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		branches = append(branches, Branch{Name: name, Type: "remote", Remote: "origin"})
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].Name < branches[j].Name })
	return branches, nil
}
