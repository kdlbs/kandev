package models

// WorkspaceFieldUpdate preserves omission separately from supplied values.
// For nullable defaults, a nil outer pointer omits the field and a nil inner
// pointer clears it. UnitID contains only an admitted placement change.
type WorkspaceFieldUpdate struct {
	Name                        *string
	Description                 *string
	DefaultExecutorID           **string
	DefaultEnvironmentID        **string
	DefaultAgentProfileID       **string
	DefaultConfigAgentProfileID **string
	ACPIdleSuspensionEnabled    *bool
	ACPIdleTimeoutMinutes       *int
	UnitID                      *string
}
