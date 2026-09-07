package sqlite

import "time"

// ClaimSafetyLimits carries the resolved launch-safety ceilings and
// budgets ClaimNextEligibleRun enforces (REQ-OFFICE-LAUNCH-SAFETY-001,
// REQ-OFFICE-LAUNCH-SAFETY-005). A zero field is replaced by the
// documented default wherever the limits are read, so a Repository
// built without SetClaimSafetyLimits still enforces the spec defaults
// rather than being unbounded.
type ClaimSafetyLimits struct {
	MaxConcurrentInstance  int
	MaxConcurrentWorkspace int
	WorkspaceBudgetPerHour int
	RoutineBudgetPerHour   int
	PromotionAge           time.Duration
}

// Documented defaults (docs/specs/office/system-design/unattended-launch-safety-01.md
// "Configuration"). BudgetWindow is the rolling window the workspace and
// routine launch budgets count against.
const (
	DefaultMaxConcurrentInstance  = 8
	DefaultMaxConcurrentWorkspace = 4
	DefaultWorkspaceBudgetPerHour = 120
	DefaultRoutineBudgetPerHour   = 20
	DefaultPromotionAge           = 15 * time.Minute
	BudgetWindow                  = time.Hour
)

// SetClaimSafetyLimits configures the ceilings and budgets the claim
// gate enforces. A value less than 1 (or, for PromotionAge, less than
// or equal to zero) is replaced by the documented default, matching the
// enqueue-side clamp idiom in runs/service.Service.SetLaunchSafetyLimits.
func (r *Repository) SetClaimSafetyLimits(limits ClaimSafetyLimits) {
	r.claimLimits = limits
}

// effectiveClaimLimits applies the clamp at read time, not only when
// SetClaimSafetyLimits is called, per AC-OFFICE-LAUNCH-SAFETY-001.5: a
// runtime override that resolves to 0 must not reach the gate as
// "unlimited".
func (r *Repository) effectiveClaimLimits() ClaimSafetyLimits {
	limits := r.claimLimits
	limits.MaxConcurrentInstance = clampToDefaultInt(limits.MaxConcurrentInstance, DefaultMaxConcurrentInstance)
	limits.MaxConcurrentWorkspace = clampToDefaultInt(limits.MaxConcurrentWorkspace, DefaultMaxConcurrentWorkspace)
	limits.WorkspaceBudgetPerHour = clampToDefaultInt(limits.WorkspaceBudgetPerHour, DefaultWorkspaceBudgetPerHour)
	limits.RoutineBudgetPerHour = clampToDefaultInt(limits.RoutineBudgetPerHour, DefaultRoutineBudgetPerHour)
	if limits.PromotionAge <= 0 {
		limits.PromotionAge = DefaultPromotionAge
	}
	return limits
}

func clampToDefaultInt(value, def int) int {
	if value < 1 {
		return def
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
