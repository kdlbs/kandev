package process

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/common/gitbase"
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
	materialized, err := gitbase.Materialize(ctx, gitbase.GitRunner(run), models.PRBase{Target: target})
	if err != nil {
		code := comparisonTargetErrorFetch
		switch gitbase.ErrorCode(err) {
		case gitbase.ErrorInvalidTarget:
			code = comparisonTargetErrorInvalid
		case gitbase.ErrorRemoteCollision:
			code = comparisonTargetErrorRemoteCollision
		case gitbase.ErrorRemoteSetup:
			code = comparisonTargetErrorRemoteSetup
		case gitbase.ErrorRefUnavailable:
			code = comparisonTargetErrorRefUnavailable
		}
		return comparisonTargetMaterialization{}, comparisonTargetFailure(code, err)
	}
	return comparisonTargetMaterialization{RemoteName: materialized.RemoteName, Ref: materialized.Ref}, nil
}
