package engine

// MapRegistry is a simple callback registry for action callbacks.
type MapRegistry map[ActionKind]ActionCallback

// Get resolves a callback by action kind.
func (r MapRegistry) Get(kind ActionKind) (ActionCallback, bool) {
	cb, ok := r[kind]
	return cb, ok
}
