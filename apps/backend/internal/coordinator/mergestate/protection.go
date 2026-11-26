package mergestate

// Enforcement says whether the provider enforces one protection.
type Enforcement string

// Enforcement values. Unknown is never read as enforced.
const (
	Enforced    Enforcement = "enforced"
	NotEnforced Enforcement = "not_enforced"
	UnknownEnf  Enforcement = "unknown"
)

// Protection is what a provider enforces on merging into one branch.
type Protection struct {
	Pipeline  Enforcement
	Approvals Enforcement
	Threads   Enforcement
}

// UnknownProtection is the record of a failed or unsupported read.
func UnknownProtection() Protection {
	return Protection{Pipeline: UnknownEnf, Approvals: UnknownEnf, Threads: UnknownEnf}
}

// ProtectionRequirements are the protections one source (classic rule or
// ruleset) requires.
type ProtectionRequirements struct {
	Pipeline, Approvals, Threads bool
}

// ClassicState is the outcome of reading GitHub classic branch protection.
type ClassicState int

const (
	// ClassicFailed is any failed, rate-limited or unauthorized read.
	ClassicFailed ClassicState = iota
	// ClassicFound is a rule that was read.
	ClassicFound
	// ClassicAbsent is a 404. It means "no rule" only when admin access is
	// confirmed, because GitHub also answers 404 without that access.
	ClassicAbsent
)

// ClassicRead is a classic branch protection read.
type ClassicRead struct {
	State          ClassicState
	AdminConfirmed bool
	Requires       ProtectionRequirements
}

// RulesetsRead is a read of the rulesets that apply to the branch.
type RulesetsRead struct {
	OK       bool
	Requires ProtectionRequirements
}

func combineOne(classicEnforces, rulesEnforces, classicKnown, rulesKnown bool) Enforcement {
	switch {
	case classicEnforces || rulesEnforces:
		return Enforced
	case classicKnown && rulesKnown:
		return NotEnforced
	}
	return UnknownEnf
}

// GitHubProtection merges classic protection and rulesets: a protection is
// enforced when either source requires it, not enforced only when both
// sources were read and neither requires it, and unknown otherwise.
func GitHubProtection(classic ClassicRead, rules RulesetsRead) Protection {
	classicKnown := classic.State == ClassicFound || (classic.State == ClassicAbsent && classic.AdminConfirmed)
	c, r := classic.Requires, rules.Requires
	if classic.State != ClassicFound {
		c = ProtectionRequirements{}
	}
	return Protection{
		Pipeline:  combineOne(c.Pipeline, r.Pipeline, classicKnown, rules.OK),
		Approvals: combineOne(c.Approvals, r.Approvals, classicKnown, rules.OK),
		Threads:   combineOne(c.Threads, r.Threads, classicKnown, rules.OK),
	}
}
