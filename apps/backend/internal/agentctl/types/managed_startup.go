package types

import (
	"encoding/json"
	"math"
)

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
	CacheEvidence         *ManagedRuntimeCacheEvidence  `json:"cache_evidence,omitempty"`
	DiagnosticReason      string                        `json:"diagnostic_reason,omitempty"`
	DiagnosticComplete    bool                          `json:"diagnostic_complete"`
}

// UnmarshalJSON decodes each optional field independently so one malformed
// peer value cannot hide the bounded process evidence around it.
func (e *ManagedStartupEvidence) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*e = ManagedStartupEvidence{}
	if err := json.Unmarshal(fields["process_generation"], &e.ProcessGeneration); err != nil {
		e.ProcessGeneration = 0
	}
	if err := json.Unmarshal(fields["exit_disposition"], &e.ExitDisposition); err != nil {
		e.ExitDisposition = ManagedStartupExitUnknown
	}
	switch e.ExitDisposition {
	case ManagedStartupExitOrdinary, ManagedStartupExitIntentional, ManagedStartupExitSignal, ManagedStartupExitUnknown:
	default:
		e.ExitDisposition = ManagedStartupExitUnknown
	}
	e.ExitCode = decodeManagedStartupExitCode(fields["exit_code"])
	npmComplete := decodeManagedStartupString(fields["npm_code"], &e.NPMCode)
	npmComplete = decodeManagedStartupBool(fields["collection_complete"], &e.CollectionComplete) && npmComplete
	npmComplete = decodeManagedStartupBool(fields["npm_diagnostic_present"], &e.NPMDiagnosticPresent) && npmComplete
	npmComplete = decodeManagedStartupBool(fields["npm_diagnostic_complete"], &e.NPMDiagnosticComplete) && npmComplete
	npmComplete = decodeManagedStartupBool(fields["unclassified_npm_code"], &e.UnclassifiedNPMCode) && npmComplete
	if !npmComplete {
		// A malformed npm witness cannot authorize the empty-stderr retry path.
		// Exit status and diagnostic attribution remain independently usable.
		e.NPMCode = ""
		e.NPMDiagnosticPresent = false
		e.NPMDiagnosticComplete = false
		e.UnclassifiedNPMCode = false
	}
	if raw, ok := fields["cache_evidence"]; ok {
		var cache *ManagedRuntimeCacheEvidence
		if err := json.Unmarshal(raw, &cache); err == nil {
			e.CacheEvidence = cache
		}
	}
	decodeManagedStartupString(fields["diagnostic_reason"], &e.DiagnosticReason)
	decodeManagedStartupBool(fields["diagnostic_complete"], &e.DiagnosticComplete)
	return nil
}

func decodeManagedStartupExitCode(raw json.RawMessage) *int {
	if len(raw) == 0 {
		return nil
	}
	var value *int
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil
	}
	if *value < 0 || *value > math.MaxUint32 {
		return nil
	}
	return value
}

// decodeManagedStartupString reports whether the value was absent or decodable.
// Null is not a valid supplied string.
func decodeManagedStartupString(raw json.RawMessage, target *string) bool {
	if len(raw) == 0 {
		return true
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return false
	}
	*target = *value
	return true
}

// decodeManagedStartupBool reports whether the value was absent or decodable.
// Null is not a valid supplied safety flag.
func decodeManagedStartupBool(raw json.RawMessage, target *bool) bool {
	if len(raw) == 0 {
		return true
	}
	var value *bool
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return false
	}
	*target = *value
	return true
}
