package lifecycle

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/offlinebudget"
)

const (
	// toolTimeoutHeadroom is added to the offline budget for the managed
	// default, so the budget error reaches the agent before the harness gives up.
	toolTimeoutHeadroom = 10 * time.Minute
	// toolTimeoutMinimumHeadroom is the least a configured value may exceed the
	// budget by.
	toolTimeoutMinimumHeadroom = time.Minute
)

// ErrToolTimeoutBelowOfflineBudget is returned when a configured harness tool
// timeout would end a waiting call before the offline budget does.
var ErrToolTimeoutBelowOfflineBudget = errors.New("tool timeout is below the offline budget")

// ErrToolTimeoutInvalid is returned when a configured harness tool timeout is
// not a positive whole number of the harness unit.
var ErrToolTimeoutInvalid = errors.New("tool timeout is not a positive whole number")

// applyToolTimeoutFloor raises the managed default harness tool timeout to the
// offline budget plus headroom, and rejects a configured value that would end
// a waiting call before the budget does. A value equal to the managed default
// is the managed default.
func applyToolTimeoutFloor(env map[string]string, agentConfig agents.Agent, metadata map[string]interface{}) error {
	if agentConfig == nil || env == nil {
		return nil
	}
	rt := agentConfig.Runtime()
	if rt == nil || rt.ToolTimeoutEnvKey == "" || rt.ToolTimeoutUnit <= 0 {
		return nil
	}
	minutes, err := offlinebudget.Parse(getMetadataString(metadata, MetadataKeyOfflineBudgetMinutes))
	if err != nil {
		return err
	}
	budget := offlinebudget.Duration(minutes)
	key, unit := rt.ToolTimeoutEnvKey, rt.ToolTimeoutUnit
	managed := rt.Env[key]
	value, set := env[key]
	if !set || value == managed {
		return setManagedToolTimeout(env, key, managed, unit, budget)
	}
	n, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if parseErr != nil || n <= 0 || strings.ContainsAny(strings.TrimSpace(value), "+-") {
		return fmt.Errorf("%w: %s with an offline budget of %d minutes", ErrToolTimeoutInvalid, key, minutes)
	}
	if time.Duration(n)*unit < budget+toolTimeoutMinimumHeadroom {
		return fmt.Errorf("%w: %s=%s with an offline budget of %d minutes", ErrToolTimeoutBelowOfflineBudget, key, value, minutes)
	}
	return nil
}

func setManagedToolTimeout(env map[string]string, key, managed string, unit, budget time.Duration) error {
	floor := int64((budget + toolTimeoutHeadroom) / unit)
	current, _ := strconv.ParseInt(strings.TrimSpace(managed), 10, 64)
	if current < floor {
		current = floor
	}
	if current <= 0 {
		return nil
	}
	env[key] = strconv.FormatInt(current, 10)
	return nil
}
