// Package admission contains internal data passed across task admission
// boundaries. It has no repository implementation dependencies so both the
// service and concrete repositories can share the same write contract.
package admission

// InitialTaskBriefCandidate is the server-owned candidate for a prepared
// session's first direct prompt. The repository validates the description
// snapshot and sets Selected only when this candidate owns the first prompt
// slot.
type InitialTaskBriefCandidate struct {
	DescriptionSnapshot    string
	Content                string
	PromptReferenceContext string
	// PromptReferencesPrepared distinguishes an accepted empty expansion from
	// content that never passed the server-owned prompt preparer.
	PromptReferencesPrepared bool
	Selected                 bool
}
