package models

import "fmt"

// NormaliseCatchUpPolicy maps a raw stored or request value onto the
// canonical RoutineCatchUpPolicy set. The deprecated alias, an empty
// value, and any unrecognized value all normalize to the summarizing
// policy — the same "fail open toward reporting" default the write
// funnel and the read-side Scan hook below share.
func NormaliseCatchUpPolicy(raw string) RoutineCatchUpPolicy {
	if RoutineCatchUpPolicy(raw) == CatchUpPolicySkipMissed {
		return CatchUpPolicySkipMissed
	}
	return CatchUpPolicySummarizeMissed
}

// Scan implements sql.Scanner so every SELECT into a RoutineCatchUpPolicy
// field normalizes the deprecated alias and any unrecognized stored
// value to the canonical one on the way out of the database. This is
// the single read-side chokepoint: GetRoutine and ListRoutines are
// SELECT * through sqlx StructScan with no other post-scan hook.
func (p *RoutineCatchUpPolicy) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*p = NormaliseCatchUpPolicy("")
	case string:
		*p = NormaliseCatchUpPolicy(v)
	case []byte:
		*p = NormaliseCatchUpPolicy(string(v))
	default:
		return fmt.Errorf("RoutineCatchUpPolicy: unsupported scan type %T", value)
	}
	return nil
}

// Bounds a stored or requested catch_up_max is clamped to.
// CatchUpMaxDefault is the floor a value below 1 is raised to;
// CatchUpMaxCeiling is the ceiling a value above it is lowered to.
// Named constants so both bounds exist in exactly one place in Go.
const (
	CatchUpMaxDefault = 25
	CatchUpMaxCeiling = 1000
)

// NormaliseCatchUpMax clamps v to [1, CatchUpMaxCeiling], mapping a
// value below 1 to CatchUpMaxDefault. A fixpoint: normalizing an
// already-normalized value returns it unchanged, so applying it more
// than once (write funnel plus a belt-and-braces use-time clamp) is
// always safe.
func NormaliseCatchUpMax(v int) int {
	if v < 1 {
		return CatchUpMaxDefault
	}
	if v > CatchUpMaxCeiling {
		return CatchUpMaxCeiling
	}
	return v
}
