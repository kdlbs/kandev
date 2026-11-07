package dynamic

import (
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// ProviderLimit is the operator-entered limit setting of one provider, keyed by
// the provider prefix of its model IDs.
type ProviderLimit struct {
	Provider string
	// MonthlyResetAt is any one occurrence of the provider's monthly reset. The
	// reset repeats on the same day of month and clock time in
	// MonthlyResetTimezone, clamped to the last day of shorter months.
	MonthlyResetAt       *time.Time
	MonthlyResetTimezone string
	// BlockUntil pauses the provider's models until that instant.
	BlockUntil *time.Time
	UpdatedAt  time.Time
}

// NormalizeProvider returns the stored form of a provider key.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(provider), "/")))
}

// ProviderOf returns the provider prefix of a model ID. A model ID without a
// prefix names its own provider.
func ProviderOf(modelID string) string {
	model := strings.TrimSpace(modelID)
	if index := strings.Index(model, "/"); index > 0 {
		return NormalizeProvider(model[:index])
	}
	return ""
}

// NextMonthlyReset returns the first monthly reset strictly after now.
func (l ProviderLimit) NextMonthlyReset(now time.Time) (time.Time, bool) {
	if l.MonthlyResetAt == nil || l.MonthlyResetAt.IsZero() {
		return time.Time{}, false
	}
	location := time.UTC
	if l.MonthlyResetTimezone != "" {
		if loaded, err := time.LoadLocation(l.MonthlyResetTimezone); err == nil {
			location = loaded
		}
	}
	anchor := l.MonthlyResetAt.In(location)
	local := now.In(location)
	offset := (local.Year()-anchor.Year())*12 + int(local.Month()-anchor.Month())
	next := monthlyOccurrence(anchor, offset)
	if !next.After(now) {
		next = monthlyOccurrence(anchor, offset+1)
	}
	return next, true
}

// BlockedUntil returns the operator block covering a model of this provider.
func (l ProviderLimit) BlockedUntil(modelID string, now time.Time) (time.Time, bool) {
	return l.BlockedUntilCandidate(Candidate{ModelID: modelID}, now)
}

// BlockedUntilCandidate returns the operator block covering one candidate.
func (l ProviderLimit) BlockedUntilCandidate(_ Candidate, now time.Time) (time.Time, bool) {
	if l.BlockUntil == nil || !l.BlockUntil.After(now) {
		return time.Time{}, false
	}
	return *l.BlockUntil, true
}

func monthlyOccurrence(anchor time.Time, monthOffset int) time.Time {
	firstOfMonth := time.Date(anchor.Year(), anchor.Month()+time.Month(monthOffset), 1, 0, 0, 0, 0, anchor.Location())
	lastDay := firstOfMonth.AddDate(0, 1, -1).Day()
	day := min(anchor.Day(), lastDay)
	return time.Date(firstOfMonth.Year(), firstOfMonth.Month(), day,
		anchor.Hour(), anchor.Minute(), anchor.Second(), 0, anchor.Location())
}

// IsFreeModel reports a model the provider publishes at zero cost. The name is
// the only classification available on this runtime; an owner-declared class
// remains authoritative elsewhere.
func IsFreeModel(modelID string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelID)), "free")
}

// ModelScoped reports whether the provider meters model allowances separately.
// Every current dynamic provider metered through this path is model-scoped; the
// account-level allowance is a separate tier check.
func ModelScoped(modelID string) bool {
	switch ProviderOf(modelID) {
	case "opencode-go", "opencode", "opencode-zen", "openrouter":
		return true
	default:
		return false
	}
}

// FailureCause is the provider-side reason a limit failure names.
type FailureCause string

const (
	CauseAccountMinuteLimit    FailureCause = "account_minute_limit"
	CauseFreeDailyLimit        FailureCause = "free_daily_limit"
	CauseFreeAccountDailyLimit FailureCause = "free_account_daily_limit"
)

// FailureCauseFor refines a classified limit failure from the provider wording.
// An unrecognized cause stays empty and the caller falls back to observations.
func FailureCauseFor(_ Candidate, failure *routingerr.Error) FailureCause {
	if failure == nil {
		return ""
	}
	text := strings.ToLower(failure.RawExcerpt)
	switch {
	case strings.Contains(text, "per minute"), strings.Contains(text, "requests per minute"), strings.Contains(text, "rpm"):
		return CauseAccountMinuteLimit
	case strings.Contains(text, "free-models-per-day"), strings.Contains(text, "free model") && strings.Contains(text, "day"):
		return CauseFreeAccountDailyLimit
	case strings.Contains(text, "daily"), strings.Contains(text, "per day"):
		return CauseFreeDailyLimit
	}
	return ""
}
