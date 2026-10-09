package shared

// SessionRestoreCapabilities describes the native restore guarantees known
// by an adapter. A false value means unsupported, while an unadvertised
// capability remains conservative at the lifecycle policy boundary.
type SessionRestoreCapabilities struct {
	SupportsNativeLoad    bool
	SupportsNativeResume  bool
	SupportsDirectoryMove bool
	RequiresNativeState   bool
}
