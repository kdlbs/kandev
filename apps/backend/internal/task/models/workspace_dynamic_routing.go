package models

// Bounds of Workspace.DynamicFailedRouteAutoRetryMax, the number of
// automatic retries of a failed dynamic route between successful turns.
const (
	DefaultDynamicFailedRouteAutoRetryMax = 1
	MinDynamicFailedRouteAutoRetryMax     = 1
	MaxDynamicFailedRouteAutoRetryMax     = 3
)

// ValidDynamicFailedRouteAutoRetryMax reports whether limit is an accepted
// automatic failed-route retry limit.
func ValidDynamicFailedRouteAutoRetryMax(limit int) bool {
	return limit >= MinDynamicFailedRouteAutoRetryMax && limit <= MaxDynamicFailedRouteAutoRetryMax
}

// FailedRouteAutoRetryLimit returns the workspace's automatic failed-route
// retry limit when the opt-in is enabled, or zero when it is off. A stored
// value outside the accepted range falls back to the default.
func (w *Workspace) FailedRouteAutoRetryLimit() int {
	if w == nil || !w.DynamicFailedRouteAutoRetryEnabled {
		return 0
	}
	if !ValidDynamicFailedRouteAutoRetryMax(w.DynamicFailedRouteAutoRetryMax) {
		return DefaultDynamicFailedRouteAutoRetryMax
	}
	return w.DynamicFailedRouteAutoRetryMax
}
