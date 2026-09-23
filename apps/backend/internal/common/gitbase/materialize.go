package gitbase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	ErrorInvalidTarget   = "invalid_target"
	ErrorRemoteCollision = "remote_collision"
	ErrorRemoteSetup     = "remote_setup_failed"
	ErrorFetch           = "fetch_failed"
	ErrorRefUnavailable  = "ref_unavailable"
	ErrorOIDMismatch     = "oid_mismatch"
)

// GitRunner executes one Git command in the caller's repository and credential
// scope. Callers provide the repository directory through their runner.
type GitRunner func(context.Context, ...string) (string, error)

// Materialization identifies the exact remote-tracking commit fetched for a
// qualified PR base.
type Materialization struct {
	RemoteName string
	Ref        string
	OID        string
}

// Error retains a bounded failure category while preserving cancellation and
// the underlying command error for the owning subsystem.
type Error struct {
	Code string
	err  error
}

func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return e.Code
	}
	return e.err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func ErrorCode(err error) string {
	var targetErr *Error
	if errors.As(err, &targetErr) {
		return targetErr.Code
	}
	return ""
}

func failure(code string, err error) error {
	return &Error{Code: code, err: err}
}

// Materialize fetches only the validated target branch into its deterministic
// comparison ref. It never updates origin or a user branch. An observed OID is
// checked against the fetched commit before the ref is returned.
func Materialize(ctx context.Context, run GitRunner, base models.PRBase) (Materialization, error) {
	if run == nil {
		return Materialization{}, failure(ErrorInvalidTarget, errors.New("git runner is required"))
	}
	if err := base.Validate(); err != nil {
		return Materialization{}, failure(ErrorInvalidTarget, fmt.Errorf("qualified PR base is invalid: %w", err))
	}
	target := base.Target
	remoteName := target.ComparisonRemoteName()
	configuredURL, remoteErr := run(ctx, "config", "--get", "remote."+remoteName+".url")
	if remoteErr == nil {
		if strings.TrimSpace(configuredURL) != target.TargetRepository.RemoteURL {
			return Materialization{}, failure(ErrorRemoteCollision, fmt.Errorf("comparison remote collision for %s", remoteName))
		}
	} else if _, err := run(ctx, "remote", "add", "--no-tags", remoteName, target.TargetRepository.RemoteURL); err != nil {
		return Materialization{}, failure(ErrorRemoteSetup, fmt.Errorf("comparison remote setup failed: %w", err))
	}
	if _, err := run(ctx, "config", "remote."+remoteName+".pushurl", "DISABLED"); err != nil {
		return Materialization{}, failure(ErrorRemoteSetup, fmt.Errorf("comparison remote push protection failed: %w", err))
	}
	ref := target.ComparisonRef()
	refspec := "+refs/heads/" + target.TargetBranch + ":" + ref
	if _, err := run(ctx, "fetch", "--no-tags", remoteName, refspec); err != nil {
		return Materialization{}, failure(ErrorFetch, fmt.Errorf("comparison target fetch failed: %w", err))
	}
	output, err := run(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return Materialization{}, failure(ErrorRefUnavailable, fmt.Errorf("comparison target ref unavailable: %w", err))
	}
	actualOID := strings.TrimSpace(output)
	if actualOID == "" {
		return Materialization{}, failure(ErrorRefUnavailable, errors.New("comparison target ref resolved to an empty object ID"))
	}
	if base.OID != "" && !strings.EqualFold(actualOID, base.OID) {
		return Materialization{}, failure(ErrorOIDMismatch, fmt.Errorf("comparison target OID changed from %s to %s", base.OID, actualOID))
	}
	return Materialization{RemoteName: remoteName, Ref: ref, OID: actualOID}, nil
}

// FetchPullRequestHead fetches GitHub's pull-request head snapshot from the
// validated base repository. That keeps it distinct from a same-named branch
// on origin or from the target base branch.
func FetchPullRequestHead(ctx context.Context, run GitRunner, target models.ComparisonTarget) (string, error) {
	if run == nil {
		return "", failure(ErrorInvalidTarget, errors.New("git runner is required"))
	}
	if err := target.Validate(); err != nil {
		return "", failure(ErrorInvalidTarget, fmt.Errorf("comparison target is invalid: %w", err))
	}
	if target.Provider != models.ComparisonTargetProviderGitHub || target.Kind != models.ComparisonTargetKindPullRequest {
		return "", failure(ErrorInvalidTarget, errors.New("target is not a GitHub pull request"))
	}
	remoteName := target.ComparisonRemoteName()
	ref := fmt.Sprintf("refs/remotes/%s/pull/%d/head", remoteName, target.Number)
	refspec := fmt.Sprintf("+refs/pull/%d/head:%s", target.Number, ref)
	if _, err := run(ctx, "fetch", "--no-tags", remoteName, refspec); err != nil {
		return "", failure(ErrorFetch, fmt.Errorf("pull request head fetch failed: %w", err))
	}
	if _, err := run(ctx, "rev-parse", "--verify", ref+"^{commit}"); err != nil {
		return "", failure(ErrorRefUnavailable, fmt.Errorf("pull request head ref unavailable: %w", err))
	}
	return ref, nil
}
