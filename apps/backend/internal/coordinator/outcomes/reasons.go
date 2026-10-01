// Package outcomes records what came of each decided coordinator proposal and
// the manager overrides that corrected the coordinator.
package outcomes

import "strings"

// Reason codes of a rejection; the set is closed.
const (
	ReasonDuplicate   = "duplicate"
	ReasonWrongTarget = "wrong_target"
	ReasonWrongTiming = "wrong_timing"
	ReasonNotWanted   = "not_wanted"
	ReasonTooBroad    = "too_broad"
	ReasonOther       = "other"
	ReasonNone        = "none"
)

var reasonSet = map[string]bool{
	ReasonDuplicate: true, ReasonWrongTarget: true, ReasonWrongTiming: true,
	ReasonNotWanted: true, ReasonTooBroad: true, ReasonOther: true, ReasonNone: true,
}

// Code maps a coded reason or free text to one reason code. A coded value in
// the set wins, text with no valid code is "other" and no reason is "none".
// The text is never returned or stored.
func Code(coded, text string) string {
	if reasonSet[coded] && coded != "" {
		return coded
	}
	if strings.TrimSpace(text) != "" {
		return ReasonOther
	}
	return ReasonNone
}
