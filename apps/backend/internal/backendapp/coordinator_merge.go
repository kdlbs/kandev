package backendapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/githubhost"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	mergeProviderGitHub = "github"
	mergeProviderGitLab = "gitlab"
	// mergeReasonLimit bounds the provider reason kept on a rejection.
	mergeReasonLimit = 200

	providerStateMerged  = "merged"
	providerStateClosed  = "closed"
	providerStatePending = "pending"
)

// mergeRepositories resolves a Kandev repository ID to its provider identity.
type mergeRepositories interface {
	GetRepository(ctx context.Context, id string) (*taskmodels.Repository, error)
}

// mergeTarget is a provider repository resolved from a Kandev repository.
type mergeTarget struct {
	// owner and name are the GitHub identity; path is the GitLab project path.
	owner, name, path string
	// host is the canonical bare host; callHost is the host argument the
	// provider service validates against the workspace configuration.
	host, callHost string
}

// providerMerge is one provider's merge and protection surface. Every method
// is bound to the workspace credentials and reads without caches.
type providerMerge interface {
	readLive(ctx context.Context, ws string, t mergeTarget, number int) (mergestate.Snapshot, error)
	methods(ctx context.Context, ws string, t mergeTarget) (coordinator.MethodSet, error)
	merge(ctx context.Context, ws string, t mergeTarget, number int, headSHA, method string) (coordinator.MergeResult, error)
	// commits lists a page of commits of the change's target branch, newest first.
	commits(ctx context.Context, ws string, t mergeTarget, number, page int) (commits []branchCommit, more bool, err error)
	protection(ctx context.Context, ws string, t mergeTarget, branch string) (mergestate.Protection, error)
}

type branchCommit struct{ sha, message string }

// coordinatorMergeProviders implements coordinator.ChangeMerger and
// coordinator.ProtectionReader over the GitHub and GitLab services.
type coordinatorMergeProviders struct {
	repos     mergeRepositories
	providers map[string]providerMerge
}

var (
	_ coordinator.ChangeMerger     = (*coordinatorMergeProviders)(nil)
	_ coordinator.ProtectionReader = (*coordinatorMergeProviders)(nil)
)

func newCoordinatorMergeProviders(repos mergeRepositories, github, gitlab providerMerge) *coordinatorMergeProviders {
	p := &coordinatorMergeProviders{repos: repos, providers: map[string]providerMerge{}}
	if github != nil {
		p.providers[mergeProviderGitHub] = github
	}
	if gitlab != nil {
		p.providers[mergeProviderGitLab] = gitlab
	}
	return p
}

// canonicalMergeHost reduces a host or origin to a lowercase bare host.
func canonicalMergeHost(provider, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if provider == mergeProviderGitHub {
		h, err := githubhost.Parse(raw)
		if raw == "" {
			return githubhost.Public().String(), nil
		}
		if err != nil {
			return "", fmt.Errorf("repository host %q: %w", raw, err)
		}
		return h.String(), nil
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	raw = strings.TrimSuffix(strings.ToLower(strings.TrimRight(raw, "/")), ".")
	if raw == "" {
		return "", errors.New("repository has no provider host")
	}
	return raw, nil
}

func gitlabCallHost(providerHost, canonical string) string {
	if strings.Contains(providerHost, "://") {
		return strings.TrimRight(strings.TrimSpace(providerHost), "/")
	}
	return "https://" + canonical
}

// lookup reads the repository; a missing row is an error.
func (p *coordinatorMergeProviders) lookup(ctx context.Context, repoID string) (*taskmodels.Repository, error) {
	repo, err := p.repos.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("resolve repository %s: %w", repoID, err)
	}
	if repo == nil {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}
	return repo, nil
}

// resolve finds the repository and checks it belongs to the workspace, the
// provider and the host; any mismatch fails closed. An empty host skips the
// host comparison.
func (p *coordinatorMergeProviders) resolve(ctx context.Context, ws, provider, host, repoID string) (providerMerge, mergeTarget, error) {
	repo, err := p.lookup(ctx, repoID)
	if err != nil {
		return nil, mergeTarget{}, err
	}
	return p.target(repo, ws, provider, host)
}

func (p *coordinatorMergeProviders) target(repo *taskmodels.Repository, ws, provider, host string) (providerMerge, mergeTarget, error) {
	impl, ok := p.providers[provider]
	if !ok {
		return nil, mergeTarget{}, fmt.Errorf("merge provider %q is not available", provider)
	}
	if repo.WorkspaceID != ws || repo.Provider != provider {
		return nil, mergeTarget{}, fmt.Errorf("repository %s is not a %s repository of the workspace", repo.ID, provider)
	}
	if repo.ProviderOwner == "" || repo.ProviderName == "" {
		return nil, mergeTarget{}, fmt.Errorf("repository %s has no provider identity", repo.ID)
	}
	canonical, err := canonicalMergeHost(provider, repo.ProviderHost)
	if err != nil {
		return nil, mergeTarget{}, err
	}
	if host != "" {
		want, err := canonicalMergeHost(provider, host)
		if err != nil || want != canonical {
			return nil, mergeTarget{}, fmt.Errorf("repository %s host does not match the change host", repo.ID)
		}
	}
	t := mergeTarget{
		owner: repo.ProviderOwner, name: repo.ProviderName, path: repo.ProviderOwner + "/" + repo.ProviderName,
		host: canonical, callHost: canonical,
	}
	if provider == mergeProviderGitLab {
		t.callHost = gitlabCallHost(repo.ProviderHost, canonical)
	}
	return impl, t, nil
}

// ReadLive implements coordinator.ChangeMerger.
func (p *coordinatorMergeProviders) ReadLive(ctx context.Context, ws string, key mergestate.ChangeKey) (mergestate.Snapshot, error) {
	impl, t, err := p.resolve(ctx, ws, key.Provider, key.Host, key.RepoID)
	if err != nil {
		return mergestate.Snapshot{}, err
	}
	snap, err := impl.readLive(ctx, ws, t, key.Number)
	if err != nil {
		return mergestate.Snapshot{}, err
	}
	snap.Key = key
	return snap, nil
}

// Methods implements coordinator.ChangeMerger.
func (p *coordinatorMergeProviders) Methods(ctx context.Context, ws string, key mergestate.ChangeKey) (coordinator.MethodSet, error) {
	impl, t, err := p.resolve(ctx, ws, key.Provider, key.Host, key.RepoID)
	if err != nil {
		return coordinator.MethodSet{}, err
	}
	return impl.methods(ctx, ws, t)
}

// Merge implements coordinator.ChangeMerger. A request that gave no answer is
// an error; a definite refusal is a MergeRejected or MergeStaleHead result.
func (p *coordinatorMergeProviders) Merge(ctx context.Context, ws string, key mergestate.ChangeKey, headSHA, method string) (coordinator.MergeResult, error) {
	if strings.TrimSpace(headSHA) == "" {
		return coordinator.MergeResult{}, errors.New("merge requires the expected head SHA")
	}
	impl, t, err := p.resolve(ctx, ws, key.Provider, key.Host, key.RepoID)
	if err != nil {
		return coordinator.MergeResult{}, err
	}
	return impl.merge(ctx, ws, t, key.Number, headSHA, method)
}

// CommitMessages implements coordinator.ChangeMerger: the messages of the
// commits after afterSHA on the change's target branch, oldest first.
func (p *coordinatorMergeProviders) CommitMessages(ctx context.Context, ws string, key mergestate.ChangeKey, afterSHA string, page int) ([]string, bool, error) {
	impl, t, err := p.resolve(ctx, ws, key.Provider, key.Host, key.RepoID)
	if err != nil {
		return nil, false, err
	}
	commits, full, err := impl.commits(ctx, ws, t, key.Number, page)
	if err != nil {
		return nil, false, err
	}
	msgs, more := messagesAfter(commits, afterSHA, full)
	return msgs, more, nil
}

// messagesAfter keeps the commits newer than afterSHA from a newest-first page
// and returns their messages oldest first. more is set while afterSHA has not
// been reached and the page was full.
func messagesAfter(commits []branchCommit, afterSHA string, full bool) (msgs []string, more bool) {
	reached := false
	var newer []branchCommit
	for _, c := range commits {
		if strings.EqualFold(c.sha, afterSHA) {
			reached = true
			break
		}
		newer = append(newer, c)
	}
	for i := len(newer) - 1; i >= 0; i-- {
		msgs = append(msgs, newer[i].message)
	}
	return msgs, full && !reached
}

// ReadProtection implements coordinator.ProtectionReader. Any failed or
// insufficient read returns an error, which the coordinator records as unknown.
func (p *coordinatorMergeProviders) ReadProtection(ctx context.Context, ws, repositoryID, branch string) (mergestate.Protection, error) {
	repo, err := p.lookup(ctx, repositoryID)
	if err != nil {
		return mergestate.UnknownProtection(), err
	}
	impl, t, err := p.target(repo, ws, repo.Provider, "")
	if err != nil {
		return mergestate.UnknownProtection(), err
	}
	got, err := impl.protection(ctx, ws, t, branch)
	if err != nil {
		return mergestate.UnknownProtection(), err
	}
	return got, nil
}

// boundedReason reduces a provider message to one bounded line.
func boundedReason(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if r := []rune(s); len(r) > mergeReasonLimit {
		s = string(r[:mergeReasonLimit])
	}
	return s
}

func changeStateOf(s string) mergestate.ChangeState {
	switch s {
	case providerStateMerged:
		return mergestate.StateMerged
	case providerStateClosed:
		return mergestate.StateClosed
	}
	return mergestate.StateOpen
}

func triOfMet(met bool) mergestate.Tri {
	if met {
		return mergestate.Met
	}
	return mergestate.Unmet
}

// wireCoordinatorMergeProviders connects the provider seams of merge
// stewardship. It does nothing unless the merge flag stack is effective, so a
// disabled install never reaches a provider.
func wireCoordinatorMergeProviders(svc *coordinator.Service, repos mergeRepositories, gh *github.Service, gl *gitlab.Service) {
	if svc == nil || repos == nil || !svc.MergeEnabled() {
		return
	}
	var ghImpl, glImpl providerMerge
	if gh != nil {
		ghImpl = githubMerge{api: gh}
	}
	if gl != nil {
		glImpl = gitlabMerge{api: gl}
	}
	adapter := newCoordinatorMergeProviders(repos, ghImpl, glImpl)
	svc.SetChangeMerger(adapter)
	svc.SetProtectionReader(adapter)
}
