package dynamic

import (
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// Account-block routing lifecycle events. They live under the routing.metric.*
// namespace, the routing metrics convention, so a log rule can match the
// transition without scanning free-form text.
const (
	eventAccountBlockHold    = "routing.metric.account_block_hold"
	eventAccountBlockRelease = "routing.metric.account_block_release"
)

func accountBlockKey(executionProfileID string) string {
	if executionProfileID == "" {
		return ""
	}
	return ResourceKey(ScopeAccountBlock, executionProfileID)
}

// HoldAccountBlock keeps one execution profile out of every later selection
// until its owner releases it. Credential, model, and other profiles'
// circuits are never read or changed. The routing event is emitted by the
// caller that wins the registry's atomic hold transition, so concurrent holds
// for one profile report exactly one event; an empty profile id holds nothing
// and stays silent.
func (e *Engine) HoldAccountBlock(executionProfileID string, code routingerr.Code) error {
	if e == nil || e.circuits == nil {
		return nil
	}
	established, err := e.circuits.Hold(accountBlockKey(executionProfileID), code)
	if established {
		e.logger.Warn(eventAccountBlockHold,
			zap.String("execution_profile_id", executionProfileID),
			zap.String("code", string(code)))
	}
	return err
}

// ReleaseAccountBlock closes the owner-release hold of one execution profile
// and reports whether a hold was closed. Closing a hold is reported as a
// routing event; a profile without a hold is a silent no-op.
func (e *Engine) ReleaseAccountBlock(executionProfileID string) (bool, error) {
	if e == nil || e.circuits == nil {
		return false, nil
	}
	released, err := e.circuits.ReleaseHold(accountBlockKey(executionProfileID))
	if released && err == nil {
		e.logger.Warn(eventAccountBlockRelease,
			zap.String("execution_profile_id", executionProfileID))
	}
	return released, err
}
