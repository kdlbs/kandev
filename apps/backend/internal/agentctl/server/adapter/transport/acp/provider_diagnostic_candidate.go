package acp

import "github.com/kandev/kandev/internal/agent/runtime/routingerr"

// isProviderDiagnosticCandidate reports whether a message chunk classifies as
// a high-confidence provider failure that permits fallback. MayClassify skips
// the rule engine for text that no rule can match, which is the common case
// for assistant prose on the per-chunk path.
func (a *Adapter) isProviderDiagnosticCandidate(text string) bool {
	input := routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: a.agentID, Stderr: text}
	if !routingerr.MayClassify(input) {
		return false
	}
	classified := routingerr.Classify(input)
	return classified.Confidence == routingerr.ConfHigh && classified.FallbackAllowed
}
