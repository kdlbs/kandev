package dynamic

import (
	"math"
	"sort"
	"time"
)

const (
	tierModePace            = "pace"
	tierModeHeadroom        = "headroom"
	tierModeConsumptionRate = "consumption_rate"
)

// UsageBucket contains existing, proved provider/account/window references.
// Empty Model identifies a shared account window, not a per-model allowance.
type UsageBucket struct{ Provider, OrganizationRef, AccountRef, Window, Model string }

// CandidateUsage is an internal observation projection. Profile IDs, circuit
// fingerprints and host cache keys cannot establish its account identity.
type CandidateUsage struct {
	Bucket                UsageBucket
	FetchedAt             time.Time
	Limited               bool
	Known                 bool
	RemainingPct          float64
	ConsumptionRateKnown  bool
	ConsumptionPctPerHour float64
	// ResetAt is the proved provider reset of the same window the remaining
	// reading came from. It never implies a window start or span.
	ResetAt      time.Time
	ResetAtKnown bool
}

func finiteUsageValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func normalizedCandidateUsage(value CandidateUsage, now time.Time) CandidateUsage {
	if value.Bucket.Provider == "" || value.Bucket.AccountRef == "" || value.Bucket.Window == "" || value.FetchedAt.IsZero() || value.FetchedAt.After(now) || now.Sub(value.FetchedAt) > 5*time.Minute {
		return CandidateUsage{}
	}
	value.Known = value.Known && finiteUsageValue(value.RemainingPct) && value.RemainingPct <= 100
	value.ConsumptionRateKnown = value.Known && value.ConsumptionRateKnown && finiteUsageValue(value.ConsumptionPctPerHour)
	// A stale or absent reset is not a fact about the time left. Dropping it
	// keeps the remaining reading; it only stops the candidate from claiming a
	// runway that already ended.
	value.ResetAtKnown = value.Known && value.ResetAtKnown && !value.ResetAt.IsZero() && value.ResetAt.After(now)
	return value
}

func sharedUsage(left, right CandidateUsage) CandidateUsage {
	left.Known = left.Known && right.Known && left.RemainingPct == right.RemainingPct
	left.ConsumptionRateKnown = left.Known && left.ConsumptionRateKnown && right.ConsumptionRateKnown && left.ConsumptionPctPerHour == right.ConsumptionPctPerHour
	// Two projections of one bucket that disagree about the reset cannot claim
	// either deadline, but the readings they agree on stay usable.
	left.ResetAtKnown = left.Known && left.ResetAtKnown && right.ResetAtKnown && left.ResetAt.Equal(right.ResetAt)
	return left
}

// A shared bucket is one observation. Conflicting projections become unknown
// rather than giving one of its profiles an artificial ranking advantage.
func applyTierUsage(rows []rankedTierRow, now time.Time) {
	groups := make(map[UsageBucket]CandidateUsage)
	for index := range rows {
		value := normalizedCandidateUsage(rows[index].candidate.Usage, now)
		if prior, exists := groups[value.Bucket]; exists {
			value = sharedUsage(prior, value)
		}
		groups[value.Bucket] = value
	}
	for index := range rows {
		value := groups[normalizedCandidateUsage(rows[index].candidate.Usage, now).Bucket]
		entry := &rows[index].entry
		entry.UsageKnown, entry.ConsumptionRateKnown = value.Known, value.ConsumptionRateKnown
		if value.Known {
			remaining := value.RemainingPct
			entry.RemainingPct = &remaining
		}
		if value.ConsumptionRateKnown {
			rate := value.ConsumptionPctPerHour
			entry.ConsumptionPctPerHour = &rate
		}
		if value.ResetAtKnown {
			entry.resetAt, entry.resetAtKnown = value.ResetAt, true
		}
	}
}

const (
	usageGroupAvailable = iota
	usageGroupUnknown
	usageGroupExhausted
)

// usageGroupOf separates capacity that is known to remain from capacity that is
// merely unmeasured and from a measured allowance that is already used up. An
// unknown reading is not worse than an exhausted one: the unknown candidate may
// still have room, while the exhausted candidate is known not to.
func usageGroupOf(entry TierPreviewEntry) int {
	if !entry.UsageKnown || entry.RemainingPct == nil {
		return usageGroupUnknown
	}
	if *entry.RemainingPct <= 0 {
		return usageGroupExhausted
	}
	return usageGroupAvailable
}

// usageScore ranks candidates inside the available group. Only pace divides the
// remaining allowance by the measured burn until the proved reset, so a
// candidate with more runway left outranks one that must reset soon. Without a
// rate or a proved reset the score is the absolute remaining percentage.
func usageScore(entry TierPreviewEntry, mode string, now time.Time) float64 {
	remaining := *entry.RemainingPct
	if mode != tierModePace || !entry.ConsumptionRateKnown || entry.ConsumptionPctPerHour == nil ||
		*entry.ConsumptionPctPerHour <= 0 || !entry.resetAtKnown {
		return remaining
	}
	burn := *entry.ConsumptionPctPerHour * entry.resetAt.Sub(now).Hours()
	if burn <= 0 {
		return remaining
	}
	return remaining / burn
}

func usageRowLess(left, right TierPreviewEntry, mode string, now time.Time) bool {
	leftGroup, rightGroup := usageGroupOf(left), usageGroupOf(right)
	if leftGroup != rightGroup {
		return leftGroup < rightGroup
	}
	if leftGroup == usageGroupUnknown {
		return unknownUsageLess(left, right)
	}
	leftScore, rightScore := usageScore(left, mode, now), usageScore(right, mode, now)
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	return *left.RemainingPct > *right.RemainingPct
}

// A rate without a remaining reading is a lower bound on consumption, so the
// slower burner is the safer unknown candidate. Stored order breaks ties.
func unknownUsageLess(left, right TierPreviewEntry) bool {
	if left.ConsumptionRateKnown != right.ConsumptionRateKnown {
		return left.ConsumptionRateKnown
	}
	if !left.ConsumptionRateKnown {
		return false
	}
	return left.ConsumptionPctPerHour != nil && right.ConsumptionPctPerHour != nil &&
		*left.ConsumptionPctPerHour < *right.ConsumptionPctPerHour
}

func rankConsumptionRows(rows []rankedTierRow) []rankedTierRow {
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i].entry, rows[j].entry
		if left.ConsumptionRateKnown != right.ConsumptionRateKnown {
			return left.ConsumptionRateKnown
		}
		if !left.ConsumptionRateKnown || left.ConsumptionPctPerHour == nil || right.ConsumptionPctPerHour == nil {
			return false
		}
		return *left.ConsumptionPctPerHour < *right.ConsumptionPctPerHour
	})
	return rows
}

func rankUsageRows(rows []rankedTierRow, mode string, now time.Time) []rankedTierRow {
	if mode == tierModeConsumptionRate {
		return rankConsumptionRows(rows)
	}
	if mode != tierModeHeadroom && mode != tierModePace {
		return rows
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return usageRowLess(rows[i].entry, rows[j].entry, mode, now)
	})
	return rows
}
