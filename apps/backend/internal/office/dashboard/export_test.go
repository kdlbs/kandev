package dashboard

// SetSessionCapacityReadHook installs a yield point between a guarded
// termination path's own commit and the retained-capacity determination's
// read, and returns a function restoring the previous value. This file is
// compiled only under `go test`, so the hook has no setter at all in a
// production build.
func SetSessionCapacityReadHook(fn func(taskID, agentProfileID string)) func() {
	prev := sessionCapacityReadHook
	sessionCapacityReadHook = fn
	return func() { sessionCapacityReadHook = prev }
}
