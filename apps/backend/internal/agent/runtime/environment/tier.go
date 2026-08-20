package environment

import (
	"sort"
	"strings"
)

// Tier ranks an origin. LOWER IS STRONGER: Tier 1 beats Tier 2 beats Tier 3.
type Tier int

const (
	TierAuthoritative       Tier = 1
	TierProfileDefault      Tier = 2
	TierAgentRuntimeDefault Tier = 3
)

// Origin constants shared by both assembly sites so tier classification
// cannot drift between two independent spellings of the same origin.
const (
	OriginManagedRuntime       = "managed runtime"
	OriginManagedCredentials   = "managed credentials"
	OriginManagedAgentDefaults = "managed agent defaults"
	OriginAgentProfile         = "agent profile"
	OriginExecutorProfile      = "executor profile"

	// OriginRepositoryPrefix is a CONSTRUCTION constant used to build a
	// repository origin string (OriginRepositoryPrefix + name). It is not a
	// matching rule — classification always goes through IsRepositoryOrigin.
	OriginRepositoryPrefix = "repository "
)

// originTiers is the single source of truth for tier membership. It stays
// unexported: an exported map could be mutated by any importer, defeating
// the single-source-of-truth guarantee this table exists to provide.
// TierForOrigin is the only reader.
var originTiers = map[string]Tier{
	OriginManagedRuntime:       TierAuthoritative,
	OriginManagedCredentials:   TierAuthoritative,
	OriginExecutorProfile:      TierAuthoritative,
	OriginAgentProfile:         TierProfileDefault,
	OriginManagedAgentDefaults: TierAgentRuntimeDefault,
}

// TierForOrigin classifies one origin string into its tier. It is the ONLY
// place the tier table is read. An origin the table does not name classifies
// as TierAuthoritative — fail closed, so an unrecognised origin never
// silently loses to a known one.
func TierForOrigin(origin string) Tier {
	if IsRepositoryOrigin(origin) {
		return TierAuthoritative
	}
	if tier, ok := originTiers[origin]; ok {
		return tier
	}
	return TierAuthoritative
}

// IsRepositoryOrigin reports whether origin is a repository origin, per the
// single normative rule: strings.Fields(origin)[0] == "repository".
// TierForOrigin and NormalizeOriginLabel both call it; neither restates it.
func IsRepositoryOrigin(origin string) bool {
	fields := strings.Fields(origin)
	return len(fields) > 0 && fields[0] == "repository"
}

// RepositoryOrigin builds a repository origin string from a repository name,
// falling back to the bare prefix when name is blank. Both assembly sites
// (orchestrator and lifecycle) construct repository origins through this
// helper so the two spellings cannot drift.
func RepositoryOrigin(name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return OriginRepositoryPrefix + trimmed
	}
	return strings.TrimSpace(OriginRepositoryPrefix)
}

// NormalizeOriginLabel derives the metric-label form of one origin: a
// repository origin collapses to the bare value "repository" so per-name
// cardinality cannot leak into a metric label; every other origin is used
// verbatim.
func NormalizeOriginLabel(origin string) string {
	if IsRepositoryOrigin(origin) {
		return "repository"
	}
	return origin
}

// JoinOriginLabels normalises, de-duplicates, sorts ascending, and joins with
// "+" — the label form for a WinningOrigins list that may span several
// origins. Returns "" for a nil or empty slice.
func JoinOriginLabels(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(origins))
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		label := NormalizeOriginLabel(origin)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "+")
}

// OriginTier pairs an origin string with the tier it classified into.
type OriginTier struct {
	Origin string
	Tier   Tier
}

// OverrideRecord describes one environment key whose value was chosen by
// tier precedence instead of blocking the launch. It carries identity and
// rank only — never a literal value, a SecretID, or a decrypted secret.
type OverrideRecord struct {
	// Key is the environment key that was overridden.
	Key string
	// WinningOrigins is a de-duplicated SET: each distinct contributing
	// origin of the winning identity, listed once, sorted ascending. May
	// span tiers when a merged identity was contributed by origins of
	// different tiers carrying the same value.
	WinningOrigins []string
	// WinningTier is the STRONGEST tier among WinningOrigins — the tier the
	// winning identity was classified into.
	WinningTier Tier
	// LosingOrigins is a de-duplicated SET: each distinct origin appearing
	// among the discarded identities, paired with its own tier, sorted by
	// Origin ascending. One entry per ORIGIN, never one per discarded
	// identity.
	LosingOrigins []OriginTier
}
