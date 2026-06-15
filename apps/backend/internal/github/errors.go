package github

import (
	"errors"
	"strings"
)

// ErrInvalidPRURL signals that a caller-supplied PR URL could not be parsed.
// Used by AssociateExistingPRByURL so HTTP callers can translate the failure
// into a 400 instead of a generic 500.
var ErrInvalidPRURL = errors.New("invalid PR URL")

// ErrTaskNotFound is the sentinel that cleanup paths check to distinguish
// "the task is already gone — fine, mop up the dedup row" from a real
// upstream failure. Adapter implementations of TaskDeleter wrap this when
// the task domain reports a missing row so the github layer can recognize
// the case without string-matching the underlying error message.
var ErrTaskNotFound = errors.New("github: task not found for cleanup")

// ErrSelfApprove is returned by SubmitReview when the authenticated user
// attempts to APPROVE their own PR. GitHub rejects this with a 422; we
// catch it server-side so the UI sees a clean, typed error rather than a
// generic upstream failure when the frontend's visibility guard is bypassed.
var ErrSelfApprove = errors.New("cannot approve your own pull request")

// ErrInvalidToken is returned by ConfigureToken when the supplied PAT fails
// validation against the GitHub API. Wrapped around the underlying client
// error so HTTP callers can distinguish a validation failure (HTTP 400)
// from a secret-store write failure (HTTP 500).
var ErrInvalidToken = errors.New("invalid token")

// ErrRepoNotResolvable signals that GitHub reported the repository as
// missing, deleted, or inaccessible to the authenticated principal. The PR
// watch flood storm this fixes (SyncWatchesBatched hammering the same dead
// repo on every 5s frontend poll) classifies upstream errors via
// isRepoNotResolvableErr and feeds the result into a 10-minute negative
// cache so subsequent watch syncs short-circuit before acquiring the gh
// throttle. The cache is also flushed on token swap/clear (see
// Service.ClearRepoErrorCache wired from ConfigureToken/ClearToken) so a
// re-auth doesn't have to wait out the TTL. Use errors.Is(err,
// ErrRepoNotResolvable) at boundaries (WS handler, poller) where the
// caller needs to differentiate a "stop retrying" failure from a
// transient upstream blip.
var ErrRepoNotResolvable = errors.New("github: repository not resolvable")

// errStoreUnavailable is returned from service methods when no Store is
// wired (Provide can return a Service with store == nil when the SQLite
// repos aren't configured). Returning a typed error instead of nil-
// dereferencing keeps the watch reset handlers from panicking under that
// degenerate config — the caller surfaces a 5xx, the process keeps running.
var errStoreUnavailable = errors.New("github: store not configured")

// repoNotResolvableSubstrings matches the wire-format substrings GitHub's
// GraphQL/REST APIs use for a deterministic "repository does not exist /
// not accessible" outcome:
//   - "Could not resolve to a Repository" — GraphQL response for a missing
//     or deleted repo (the dominant signal in the storm logs)
//   - "Resource not accessible by integration" — GitHub Apps / fine-grained
//     PAT scope mismatch. Treated as "dead end" here because retrying the
//     same call with the same token will never succeed; a token swap
//     evicts the cache via ClearRepoErrorCache.
//
// Raw "HTTP 404 / Not Found" is intentionally NOT matched here. A 404 on
// /repos/{owner}/{repo}/pulls/{N} can legitimately mean "the PR was
// deleted" while the repo itself is fine — a false-positive there would
// negative-cache the whole repo for 10 minutes off a single stale PR
// number. The GraphQL "Could not resolve to a Repository" string IS
// repo-scoped by construction and remains the canonical signal.
var repoNotResolvableSubstrings = []string{
	"could not resolve to a repository",
	"resource not accessible by integration",
}

// isRepoNotResolvableErr reports whether err carries a deterministic
// "repository missing / inaccessible" signal we should not keep retrying
// against in a tight loop. Errors flagged here are negative-cached for
// 10 minutes via Service.repoErrorCache; the cache is evicted on watch
// (re)creation and on token swap/clear so a freshly-linked repo or a
// re-auth is probed immediately. The match is case-insensitive so we
// don't depend on a stable error casing across gh CLI versions and
// GraphQL response variants.
//
// Bare HTTP 404 errors are NOT classified — too ambiguous (missing PR
// number vs missing repo) and a false positive poisons the whole repo
// for 10 minutes. Callers that know they're hitting a repo-root
// endpoint should wrap with ErrRepoNotResolvable explicitly.
func isRepoNotResolvableErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRepoNotResolvable) {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, sub := range repoNotResolvableSubstrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
