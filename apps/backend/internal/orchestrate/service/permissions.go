package service

import (
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// Permission key constants.
const (
	PermCanCreateTasks     = "can_create_tasks"
	PermCanAssignTasks     = "can_assign_tasks"
	PermCanCreateAgents    = "can_create_agents"
	PermCanApprove         = "can_approve"
	PermCanManageOwnSkills = "can_manage_own_skills"
	PermMaxSubtaskDepth    = "max_subtask_depth"
)

// AllPermissionKeys returns all known permission keys in display order.
func AllPermissionKeys() []string {
	return []string{
		PermCanCreateTasks,
		PermCanAssignTasks,
		PermCanCreateAgents,
		PermCanApprove,
		PermCanManageOwnSkills,
		PermMaxSubtaskDepth,
	}
}

// ErrForbidden is returned when an agent lacks the required permission.
var ErrForbidden = fmt.Errorf("forbidden: insufficient permissions")

// ResolvePermissions merges role defaults with agent-specific overrides.
// If overrides is empty, the role defaults are returned unchanged.
func ResolvePermissions(role models.AgentRole, overrides string) map[string]interface{} {
	defaults := defaultPermsForRole(role)
	if overrides == "" || overrides == "{}" {
		return defaults
	}
	var custom map[string]interface{}
	if err := json.Unmarshal([]byte(overrides), &custom); err != nil {
		return defaults
	}
	for k, v := range custom {
		defaults[k] = v
	}
	return defaults
}

// HasPermission checks whether a boolean permission is granted.
func HasPermission(perms map[string]interface{}, key string) bool {
	v, ok := perms[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// ValidateNoEscalation ensures the requested permissions do not grant
// anything the caller itself does not have.
func ValidateNoEscalation(callerPerms map[string]interface{}, requestedJSON string) error {
	if requestedJSON == "" || requestedJSON == "{}" {
		return nil
	}
	var requested map[string]interface{}
	if err := json.Unmarshal([]byte(requestedJSON), &requested); err != nil {
		return fmt.Errorf("invalid permissions JSON: %w", err)
	}
	for key, val := range requested {
		if !callerHasAtLeast(callerPerms, key, val) {
			return fmt.Errorf("%w: cannot grant %q", ErrForbidden, key)
		}
	}
	return nil
}

// callerHasAtLeast checks whether the caller's value for a key is at least
// as permissive as the requested value.
func callerHasAtLeast(callerPerms map[string]interface{}, key string, requested interface{}) bool {
	callerVal, ok := callerPerms[key]
	if !ok {
		// Caller doesn't have this permission at all.
		// Requested false/0 is safe; anything truthy is escalation.
		return isZeroValue(requested)
	}
	// Bool comparison: caller must have true to grant true.
	if reqBool, ok := requested.(bool); ok {
		if !reqBool {
			return true
		}
		callerBool, ok := callerVal.(bool)
		return ok && callerBool
	}
	// Numeric comparison: caller's value must be >= requested.
	reqNum := toFloat(requested)
	callerNum := toFloat(callerVal)
	return callerNum >= reqNum
}

func isZeroValue(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return !val
	case float64:
		return val == 0
	case int:
		return val == 0
	default:
		return false
	}
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case bool:
		if val {
			return 1
		}
		return 0
	default:
		return 0
	}
}
