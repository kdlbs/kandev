package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrMergeHeadMoved is matched by a merge rejection that reports the expected
// head is no longer the pull request head.
var ErrMergeHeadMoved = errors.New("github: pull request head moved")

// headMovedError keeps the provider message while matching ErrMergeHeadMoved.
type headMovedError struct{ err error }

func (e *headMovedError) Error() string        { return e.err.Error() }
func (e *headMovedError) Unwrap() error        { return e.err }
func (e *headMovedError) Is(target error) bool { return target == ErrMergeHeadMoved }

// ErrMergeRejected is matched by a definite provider refusal to merge: the
// request was answered and nothing was merged.
var ErrMergeRejected = errors.New("github: merge rejected")

// mergeRejectedError keeps the provider message while matching ErrMergeRejected.
type mergeRejectedError struct{ err error }

func (e *mergeRejectedError) Error() string        { return e.err.Error() }
func (e *mergeRejectedError) Unwrap() error        { return e.err }
func (e *mergeRejectedError) Is(target error) bool { return target == ErrMergeRejected }

func isHeadMovedMessage(message string) bool {
	m := strings.ToLower(message)
	return strings.Contains(m, "head branch was modified") || strings.Contains(m, "expected head")
}

// classifyMergeError marks a head-mismatch rejection as ErrMergeHeadMoved and
// any other answered refusal (400, 405, 409, 422) as ErrMergeRejected. A status
// that may be a rate limit or an outage stays unclassified.
func classifyMergeError(err error) error {
	if err == nil || errors.Is(err, ErrMergeHeadMoved) || errors.Is(err, ErrMergeRejected) {
		return err
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.StatusCode {
	case http.StatusConflict, http.StatusUnprocessableEntity:
		if isHeadMovedMessage(apiErr.Body) {
			return &headMovedError{err: err}
		}
		return &mergeRejectedError{err: err}
	case http.StatusBadRequest, http.StatusMethodNotAllowed:
		return &mergeRejectedError{err: err}
	}
	return err
}

// AutomationMergeResult is the provider answer to an automation merge: merged
// or queued, plus the merge commit when it could be read.
type AutomationMergeResult struct {
	Outcome     MergeOutcome
	MergeCommit string
}

func normalizeAutomationHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// requireAutomationHost fails when the resolved client does not talk to the
// expected host; an empty expectation accepts the workspace's host.
func requireAutomationHost(resolved *resolvedServiceClient, expected string) error {
	if normalizeAutomationHost(expected) == "" {
		return nil
	}
	if normalizeAutomationHost(resolved.host().String()) != normalizeAutomationHost(expected) {
		return ErrGitHubHostMismatch
	}
	return nil
}

// MergePRForAutomationOutcome merges like MergePRForAutomation and returns the
// provider outcome. The request always carries the expected head; a rejection
// that reports a moved head matches ErrMergeHeadMoved. It never enables
// auto-merge or a merge queue: a queued answer is returned as MergeOutcomeQueued.
func (s *Service) MergePRForAutomationOutcome(
	ctx context.Context,
	workspaceID, host, owner, repo string,
	number int,
	mergeMethod, expectedHeadSHA string,
) (AutomationMergeResult, error) {
	if strings.TrimSpace(expectedHeadSHA) == "" {
		return AutomationMergeResult{}, fmt.Errorf("automatic merge requires an expected head SHA")
	}
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return AutomationMergeResult{}, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return AutomationMergeResult{}, err
	}
	if err := requireAutomationHost(resolved, host); err != nil {
		return AutomationMergeResult{}, err
	}
	if err := requireGitHubCapability(resolved, CapabilityPullRequestWrite); err != nil {
		return AutomationMergeResult{}, err
	}
	outcome, err := s.mergePRWithClient(
		ctx, resolved.Client, resolved.CacheScope, owner, repo, number, MergePRRequest{
			MergeMethod: mergeMethod, ExpectedHeadSHA: expectedHeadSHA,
		},
	)
	if err != nil {
		return AutomationMergeResult{}, classifyMergeError(err)
	}
	result := AutomationMergeResult{Outcome: outcome}
	if outcome == MergeOutcomeMerged {
		result.MergeCommit = pullRequestMergeCommit(ctx, resolved.Client, owner, repo, number)
	}
	return result, nil
}

// pullRequestMergeCommit reads the merge commit of a merged pull request; it
// is empty when the client cannot read it.
func pullRequestMergeCommit(ctx context.Context, client Client, owner, repo string, number int) string {
	exec, err := graphQLExecutorFor(client)
	if err != nil {
		return ""
	}
	query := fmt.Sprintf(
		`query { repository(owner: %q, name: %q) { pullRequest(number: %d) { mergeCommit { oid } } } }`,
		owner, repo, number)
	var resp struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					MergeCommit *struct {
						OID string `json:"oid"`
					} `json:"mergeCommit"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := exec.ExecuteGraphQL(ctx, query, nil, &resp); err != nil {
		return ""
	}
	if c := resp.Data.Repository.PullRequest.MergeCommit; c != nil {
		return c.OID
	}
	return ""
}

// PRLiveRead is an uncached read of a pull request.
type PRLiveRead struct {
	Status      *PRStatus
	MergeCommit string
	// HasMergeConflicts is nil when the provider did not say.
	HasMergeConflicts *bool
}

// ReadPRLiveForAutomation reads the pull request from the provider, bypassing
// every service cache. Review threads are complete when the client speaks
// GraphQL; otherwise Status.UnresolvedReviewThreadsPopulated is false.
func (s *Service) ReadPRLiveForAutomation(
	ctx context.Context, workspaceID, host, owner, repo string, number int,
) (*PRLiveRead, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	if err := requireAutomationHost(resolved, host); err != nil {
		return nil, err
	}
	status, err := readLivePRStatus(ctx, resolved.Client, owner, repo, number)
	if err != nil {
		return nil, err
	}
	if status == nil || status.PR == nil {
		return nil, fmt.Errorf("pull request %s/%s#%d: provider returned no pull request", owner, repo, number)
	}
	read := &PRLiveRead{
		Status:            status,
		HasMergeConflicts: observedPRMergeConflict(nil, effectivePRMergeableState(status.PR)),
	}
	if status.PR.State == prStateMerged {
		read.MergeCommit = pullRequestMergeCommit(ctx, resolved.Client, owner, repo, number)
	}
	return read, nil
}

func readLivePRStatus(ctx context.Context, client Client, owner, repo string, number int) (*PRStatus, error) {
	exec, err := graphQLExecutorFor(client)
	if err != nil {
		return client.GetPRStatus(ctx, owner, repo, number)
	}
	statuses, err := runBatchedPRQuery(ctx, exec, []graphQLPRRef{{Owner: owner, Repo: repo, Number: number}})
	if err != nil {
		return nil, err
	}
	status := statuses[prStatusCacheKey(owner, repo, number)]
	if status == nil {
		return nil, fmt.Errorf("pull request %s/%s#%d: not in provider answer", owner, repo, number)
	}
	return status, nil
}

// RepoMergeMethodsForAutomation reads the repository's allowed merge methods
// without the service cache.
func (s *Service) RepoMergeMethodsForAutomation(
	ctx context.Context, workspaceID, host, owner, repo string,
) (RepoMergeMethods, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return RepoMergeMethods{}, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return RepoMergeMethods{}, err
	}
	if err := requireAutomationHost(resolved, host); err != nil {
		return RepoMergeMethods{}, err
	}
	return resolved.Client.GetRepoMergeMethods(ctx, owner, repo)
}

// DefaultMergeMethod is the method the service picks when none is requested.
func (m RepoMergeMethods) DefaultMergeMethod() string { return pickDefaultMergeMethod(m) }
