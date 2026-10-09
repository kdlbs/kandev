package types

// ManagedStartupExitDisposition identifies how a managed agent process ended.
type ManagedStartupExitDisposition string

const (
	ManagedStartupExitOrdinary    ManagedStartupExitDisposition = "ordinary_exit"
	ManagedStartupExitIntentional ManagedStartupExitDisposition = "intentional_stop"
	ManagedStartupExitSignal      ManagedStartupExitDisposition = "signal_exit"
	ManagedStartupExitUnknown     ManagedStartupExitDisposition = "unknown"
)

// ManagedStartupEvidence is the bounded process evidence attached to an ACP
// initialize failure. It contains no raw stderr or host paths.
type ManagedStartupEvidence struct {
	ProcessGeneration     uint64                        `json:"process_generation"`
	ExitDisposition       ManagedStartupExitDisposition `json:"exit_disposition"`
	ExitCode              *int                          `json:"exit_code,omitempty"`
	NPMCode               string                        `json:"npm_code,omitempty"`
	CollectionComplete    bool                          `json:"collection_complete"`
	NPMDiagnosticPresent  bool                          `json:"npm_diagnostic_present"`
	NPMDiagnosticComplete bool                          `json:"npm_diagnostic_complete"`
	UnclassifiedNPMCode   bool                          `json:"unclassified_npm_code"`
}
