package process

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	comparisonTargetErrorPending         = "comparison_target_pending"
	comparisonTargetErrorInvalid         = "comparison_target_invalid"
	comparisonTargetErrorRemoteCollision = "comparison_remote_collision"
	comparisonTargetErrorRemoteSetup     = "comparison_remote_setup_failed"
	comparisonTargetErrorFetch           = "comparison_target_fetch_failed"
	comparisonTargetErrorRefUnavailable  = "comparison_target_ref_unavailable"
	comparisonTargetErrorMergeBase       = "comparison_merge_base_unavailable"
	comparisonTargetStatusReady          = "ready"
	comparisonTargetStatusUnavailable    = "unavailable"
)

type comparisonTargetMaterialization struct {
	RemoteName string
	Ref        string
}

type comparisonTargetMaterializationError struct {
	code string
	err  error
}

func (e *comparisonTargetMaterializationError) Error() string {
	if e == nil || e.err == nil {
		return e.code
	}
	return e.err.Error()
}

func (e *comparisonTargetMaterializationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func comparisonTargetFailure(code string, err error) error {
	return &comparisonTargetMaterializationError{code: code, err: err}
}

// ComparisonResolution is the bounded state consumed by every comparison
// surface. Explicit=true means callers must use Ref when Status is ready and
// must return unavailable when it is not.
type ComparisonResolution struct {
	Explicit  bool
	Ref       string
	Display   string
	Status    string
	ErrorCode string
}

// Ready reports whether an explicit comparison target has been fully
// materialized and is safe for Git consumers to use.
func (r ComparisonResolution) Ready() bool {
	return r.Explicit && r.Status == comparisonTargetStatusReady && r.Ref != ""
}

// SetComparisonTarget installs the desired target before tracker polling
// begins. It starts unavailable so an unmaterialized target cannot fall back
// to a same-named origin branch.
func (wt *WorkspaceTracker) SetComparisonTarget(target *models.ComparisonTarget) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if target == nil {
		wt.comparisonTarget = nil
		wt.comparisonTargetRef = ""
		wt.comparisonTargetStatus = ""
		wt.comparisonTargetErrorCode = ""
		return
	}
	copy := *target
	wt.comparisonTarget = &copy
	wt.comparisonTargetRef = ""
	wt.comparisonTargetStatus = comparisonTargetStatusUnavailable
	wt.comparisonTargetErrorCode = comparisonTargetErrorPending
	if err := copy.Validate(); err != nil {
		wt.comparisonTargetErrorCode = comparisonTargetErrorInvalid
	}
}

// SetComparisonTargetReady publishes a fully materialized target ref.
func (wt *WorkspaceTracker) SetComparisonTargetReady(target *models.ComparisonTarget, ref string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if target == nil || target.Validate() != nil || ref != target.ComparisonRef() {
		wt.setComparisonTargetUnavailableLocked(target, comparisonTargetErrorInvalid)
		return
	}
	copy := *target
	wt.comparisonTarget = &copy
	wt.comparisonTargetRef = ref
	wt.comparisonTargetStatus = comparisonTargetStatusReady
	wt.comparisonTargetErrorCode = ""
}

// SetComparisonTargetUnavailable keeps the desired target visible while
// publishing only a bounded error code. Raw provider/Git errors stay in logs.
func (wt *WorkspaceTracker) SetComparisonTargetUnavailable(target *models.ComparisonTarget, code string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	wt.setComparisonTargetUnavailableLocked(target, code)
}

func (wt *WorkspaceTracker) setComparisonTargetUnavailableLocked(target *models.ComparisonTarget, code string) {
	if target == nil {
		wt.comparisonTarget = nil
		wt.comparisonTargetRef = ""
		wt.comparisonTargetStatus = ""
		wt.comparisonTargetErrorCode = ""
		return
	}
	copy := *target
	wt.comparisonTarget = &copy
	wt.comparisonTargetRef = ""
	wt.comparisonTargetStatus = comparisonTargetStatusUnavailable
	if code == "" {
		code = comparisonTargetErrorInvalid
	}
	wt.comparisonTargetErrorCode = code
}

// ComparisonResolution returns a race-safe snapshot of the authoritative
// comparison state for this tracker.
func (wt *WorkspaceTracker) ComparisonResolution() ComparisonResolution {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	if wt.comparisonTarget == nil {
		return ComparisonResolution{}
	}
	return ComparisonResolution{
		Explicit:  true,
		Ref:       wt.comparisonTargetRef,
		Display:   wt.comparisonTarget.DisplayIdentity(),
		Status:    wt.comparisonTargetStatus,
		ErrorCode: wt.comparisonTargetErrorCode,
	}
}

func (wt *WorkspaceTracker) ComparisonTargetSnapshot() *models.ComparisonTarget {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	if wt.comparisonTarget == nil {
		return nil
	}
	copy := *wt.comparisonTarget
	return &copy
}

func isExplicitComparisonRef(ref string) bool {
	return strings.HasPrefix(ref, "refs/remotes/compare-")
}

type comparisonTargetGitRunner func(context.Context, ...string) (string, error)

func materializeComparisonTarget(
	ctx context.Context,
	run comparisonTargetGitRunner,
	target models.ComparisonTarget,
) (comparisonTargetMaterialization, error) {
	if err := target.Validate(); err != nil {
		return comparisonTargetMaterialization{}, comparisonTargetFailure(
			comparisonTargetErrorInvalid,
			fmt.Errorf("comparison target invalid: %w", err),
		)
	}
	remoteName := target.ComparisonRemoteName()
	configuredURL, remoteErr := run(ctx, "remote", "get-url", remoteName)
	if remoteErr == nil {
		if strings.TrimSpace(configuredURL) != target.TargetRepository.RemoteURL {
			return comparisonTargetMaterialization{}, comparisonTargetFailure(
				comparisonTargetErrorRemoteCollision,
				fmt.Errorf("comparison remote collision for %s", remoteName),
			)
		}
	} else {
		if _, err := run(ctx, "remote", "add", "--no-tags", remoteName, target.TargetRepository.RemoteURL); err != nil {
			return comparisonTargetMaterialization{}, comparisonTargetFailure(
				comparisonTargetErrorRemoteSetup,
				fmt.Errorf("comparison remote setup failed: %w", err),
			)
		}
	}
	if _, err := run(ctx, "config", "remote."+remoteName+".pushurl", "DISABLED"); err != nil {
		return comparisonTargetMaterialization{}, comparisonTargetFailure(
			comparisonTargetErrorRemoteSetup,
			fmt.Errorf("comparison remote push protection failed: %w", err),
		)
	}
	refspec := "refs/heads/" + target.TargetBranch + ":" + target.ComparisonRef()
	if _, err := run(ctx, "fetch", "--no-tags", remoteName, refspec); err != nil {
		return comparisonTargetMaterialization{}, comparisonTargetFailure(
			comparisonTargetErrorFetch,
			fmt.Errorf("comparison target fetch failed: %w", err),
		)
	}
	if _, err := run(ctx, "rev-parse", "--verify", target.ComparisonRef()+"^{commit}"); err != nil {
		return comparisonTargetMaterialization{}, comparisonTargetFailure(
			comparisonTargetErrorRefUnavailable,
			fmt.Errorf("comparison target ref unavailable: %w", err),
		)
	}
	return comparisonTargetMaterialization{RemoteName: remoteName, Ref: target.ComparisonRef()}, nil
}
