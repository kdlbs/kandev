package usage

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const defaultDailyReset = "00:00 UTC"

type BillingTier string

const (
	TierFree         BillingTier = "free"
	TierSubscription BillingTier = "subscription"
	TierPaid         BillingTier = "paid"
)

// TierLimits contains optional account allowances. Nil means unknown/inherit;
// an explicit zero disables that configured allowance.
type TierLimits struct {
	learnedFields        map[string]bool
	MaxConcurrentTurns   *float64       `json:"max_concurrent_turns,omitempty"`
	RequestsPerMinute    *float64       `json:"requests_per_minute,omitempty"`
	RequestsPerDay       *float64       `json:"requests_per_day,omitempty"`
	DailyReset           string         `json:"daily_reset,omitempty"`
	RequestsPerWindow    *float64       `json:"requests_per_window,omitempty"`
	RequestWindowSeconds int64          `json:"request_window_seconds,omitempty"`
	RequestsPerModel     bool           `json:"requests_per_model,omitempty"`
	Windows              []LimitBudget  `json:"windows,omitempty"`
	Scopes               []RequestScope `json:"scopes,omitempty"`
	// Provenance labels default values by JSON field name.
	Provenance map[string]string `json:"provenance,omitempty"`
	Learned    bool              `json:"learned,omitempty"`
}

type LimitBudget struct {
	Learned         bool       `json:"learned,omitempty"`
	WindowEstimated bool       `json:"window_estimated,omitempty"`
	Window          string     `json:"window"`
	Unit            string     `json:"unit"`
	Value           *float64   `json:"value,omitempty"`
	Reset           string     `json:"reset"`
	ResetAt         *time.Time `json:"reset_at,omitempty"`
	Timezone        string     `json:"timezone,omitempty"`
	Scope           string     `json:"scope,omitempty"`
}

type AccountTierConfig struct {
	SubscriptionPlan string                     `json:"subscription_plan,omitempty"`
	PurchasedCredits *bool                      `json:"purchased_credits,omitempty"`
	Tiers            map[BillingTier]TierLimits `json:"tiers"`
	ProfileTiers     map[string]BillingTier     `json:"profile_tiers,omitempty"`
}

// LimitUsage combines recorded lower bounds and provider-reported windows.
type LimitUsage struct {
	ActiveTurns    int64                `json:"active_turns"`
	RequestsMinute float64              `json:"requests_minute"`
	RequestsDay    float64              `json:"requests_day"`
	RequestsWindow float64              `json:"requests_window"`
	TurnsDay       int64                `json:"turns_day"`
	TokensDay      int64                `json:"tokens_day"`
	Windows        map[string]float64   `json:"windows"`
	Scopes         map[string]float64   `json:"scopes,omitempty"`
	Resets         map[string]time.Time `json:"resets,omitempty"`
	Estimated      bool                 `json:"estimated"`
}

type LimitAdmission struct {
	Reason  string    `json:"reason,omitempty"`
	RetryAt time.Time `json:"retry_at,omitempty"`
}

func EvaluateTierLimits(limits TierLimits, used LimitUsage, now time.Time) LimitAdmission {
	if positive(limits.MaxConcurrentTurns) && float64(used.ActiveTurns) >= *limits.MaxConcurrentTurns*limits.margin("concurrency") {
		return LimitAdmission{Reason: "limit_concurrency", RetryAt: now.Add(time.Second)}
	}
	if exceeds(used.RequestsMinute, limits.RequestsPerMinute, limits.margin("minute")) {
		return LimitAdmission{Reason: "limit_minute", RetryAt: now.Add(time.Minute)}
	}
	if exceeds(used.RequestsWindow, limits.RequestsPerWindow, limits.margin("short_window")) {
		return LimitAdmission{Reason: "limit_minute", RetryAt: now.Add(time.Duration(limits.RequestWindowSeconds) * time.Second)}
	}
	if exceeds(used.RequestsDay, limits.RequestsPerDay, limits.margin("daily")) {
		_, reset, _ := DailyLimitWindow(now, limits.DailyReset)
		return LimitAdmission{Reason: "limit_daily", RetryAt: reset}
	}
	for _, budget := range limits.Windows {
		amount, known := used.Windows[budget.Key()]
		if !known || !positive(budget.Value) || amount < *budget.Value*budgetMargin(budget, limits) {
			continue
		}
		reset := used.Resets[budget.Key()]
		if !reset.After(now) {
			_, reset, _ = LimitWindowBounds(now, budget)
		}
		if !reset.After(now) {
			reset = now.Add(time.Minute)
		}
		return LimitAdmission{Reason: "limit_window", RetryAt: reset}
	}
	return LimitAdmission{}
}

func (l TierLimits) margin(field string) float64 {
	if l.learnedFields != nil {
		if l.learnedFields[field] {
			return 0.95
		}
		return 1
	}
	if l.Learned {
		return 0.95
	}
	return 1
}

func budgetMargin(b LimitBudget, l TierLimits) float64 {
	if b.Learned || (l.learnedFields == nil && l.Learned) {
		return 0.95
	}
	return 1
}

func DefaultTierLimits(provider string, tier BillingTier, plan string, purchasedCredits bool) TierLimits {
	if limits, ok := catalogTierDefaults(provider, tier, plan, purchasedCredits); ok {
		return limits
	}
	return TierLimits{}
}

func LearnTierLimits(current TierLimits, dimension string, used LimitUsage) TierLimits {
	switch dimension {
	case "minute":
		current.RequestsPerMinute, current.Learned = learnAllowance(current.RequestsPerMinute, used.RequestsMinute, current.Learned)
	case "daily":
		current.RequestsPerDay, current.Learned = learnAllowance(current.RequestsPerDay, used.RequestsDay, current.Learned)
		if current.Learned {
			current.DailyReset = defaultDailyReset
		}
	case "concurrency":
		current.MaxConcurrentTurns, current.Learned = learnAllowance(current.MaxConcurrentTurns, float64(used.ActiveTurns), current.Learned)
	case "unknown_subscription":
		current = learnUnknownSubscription(current, used)
	default:
		current = learnWindowAllowance(current, dimension, used)
	}
	return current
}

func learnAllowance(current *float64, observed float64, learned bool) (*float64, bool) {
	if current == nil && observed > 0 {
		return limitNumber(observed), true
	}
	return current, learned
}

func learnWindowAllowance(current TierLimits, dimension string, used LimitUsage) TierLimits {
	if dimension != "5h" && dimension != WindowPeriodWeek && dimension != WindowPeriodMonth {
		return current
	}
	amount, ok := used.Windows[dimension]
	if !ok || amount <= 0 {
		return current
	}
	for _, budget := range current.Windows {
		if budget.Window == dimension && budget.Value != nil {
			return current
		}
	}
	current.Windows = append(current.Windows, LimitBudget{Window: dimension, Unit: "tokens", Value: limitNumber(amount), Reset: "rolling", Learned: true})
	current.Learned = true
	return current
}

func learnUnknownSubscription(current TierLimits, used LimitUsage) TierLimits {
	before := len(current.Windows)
	current = learnWindowAllowance(current, "5h", used)
	if len(current.Windows) > before {
		current.Windows[len(current.Windows)-1].WindowEstimated = true
	}
	return current
}

func positive(value *float64) bool { return value != nil && *value > 0 }
func exceeds(used float64, limit *float64, margin float64) bool {
	return positive(limit) && used+1 > *limit*margin
}
func limitNumber(value float64) *float64 { return &value }
func (b LimitBudget) Key() string {
	if b.Scope != "" {
		return b.Window + ":" + b.Scope
	}
	return b.Window
}

// DailyLimitWindow accepts an explicit clock and IANA time zone.
func DailyLimitWindow(now time.Time, rule string) (time.Time, time.Time, error) {
	if rule == "" {
		rule = defaultDailyReset
	}
	parts := strings.Fields(rule)
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("daily reset must be HH:MM timezone")
	}
	clock, err := time.Parse("15:04", parts[0])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid daily reset clock")
	}
	zone, err := time.LoadLocation(parts[1])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid daily reset timezone")
	}
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, zone)
	if now.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start, start.AddDate(0, 0, 1), nil
}

func LimitWindowBounds(now time.Time, b LimitBudget) (time.Time, time.Time, bool) {
	span := map[string]time.Duration{"5h": 5 * time.Hour, WindowPeriodDay: 24 * time.Hour, WindowPeriodWeek: 7 * 24 * time.Hour, WindowPeriodMonth: 30 * 24 * time.Hour}[b.Window]
	if span == 0 {
		return time.Time{}, time.Time{}, false
	}
	if b.Reset == "provider" && b.ResetAt == nil {
		return time.Time{}, time.Time{}, false
	}
	if b.Reset == "anchor" || (b.Reset == "provider" && b.ResetAt != nil) {
		return anchoredLimitWindow(now, b)
	}
	if b.Reset == "utc" {
		return utcLimitWindow(now, b.Window)
	}
	return now.Add(-span), now.Add(span), true
}

func anchoredLimitWindow(now time.Time, b LimitBudget) (time.Time, time.Time, bool) {
	if b.ResetAt == nil {
		return time.Time{}, time.Time{}, false
	}
	zone := time.UTC
	if b.Timezone != "" {
		loaded, err := time.LoadLocation(b.Timezone)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		zone = loaded
	}
	if b.Window == "5h" {
		resolved, ok := fiveHourWindowFrom(now, *b.ResetAt, zone)
		return resolved.Start, resolved.Reset, ok
	}
	local := b.ResetAt.In(zone)
	anchor := ResetAnchor{Hour: local.Hour(), Minute: local.Minute(), Weekday: local.Weekday(), HasWeekday: true, DayOfMonth: local.Day()}
	resolved, ok := ResolveResetWindow(now, b.Window, anchor, zone)
	return resolved.Start, resolved.Reset, ok
}

func utcLimitWindow(now time.Time, period string) (time.Time, time.Time, bool) {
	utc := now.UTC()
	start := utc.Truncate(24 * time.Hour)
	switch period {
	case WindowPeriodDay:
		return start, start.AddDate(0, 0, 1), true
	case WindowPeriodMonth:
		start = time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), true
	case WindowPeriodWeek:
		start = start.AddDate(0, 0, -(int(utc.Weekday())+6)%7)
		return start, start.AddDate(0, 0, 7), true
	default:
		return time.Time{}, time.Time{}, false
	}
}

// MergeTierLimits keeps owner settings last and merges budgets by window/scope.
func MergeTierLimits(base, next TierLimits) TierLimits {
	base = withLearnedFields(base)
	if next.MaxConcurrentTurns != nil {
		base.MaxConcurrentTurns = next.MaxConcurrentTurns
		base.learnedFields["concurrency"] = next.Learned
	}
	if next.RequestsPerMinute != nil {
		base.RequestsPerMinute = next.RequestsPerMinute
		base.learnedFields["minute"] = next.Learned
	}
	if next.RequestsPerDay != nil {
		base.RequestsPerDay = next.RequestsPerDay
		base.learnedFields["daily"] = next.Learned
	}
	if next.DailyReset != "" {
		base.DailyReset = next.DailyReset
	}
	if next.RequestsPerWindow != nil {
		base.RequestsPerWindow = next.RequestsPerWindow
		base.learnedFields["short_window"] = next.Learned
		base.RequestWindowSeconds = next.RequestWindowSeconds
	}
	if next.RequestsPerModel {
		base.RequestsPerModel = true
	}
	base.Windows = append([]LimitBudget(nil), base.Windows...)
	for _, b := range next.Windows {
		b.Learned = b.Learned || next.Learned
		found := false
		for i, old := range base.Windows {
			if old.Key() == b.Key() {
				if b.Value == nil {
					b.Value = old.Value
					b.Learned = old.Learned
					b.WindowEstimated = old.WindowEstimated
				}
				base.Windows[i] = b
				found = true
				break
			}
		}
		if !found {
			base.Windows = append(base.Windows, b)
		}
	}
	base.Scopes = mergeRequestScopes(base.Scopes, next.Scopes, next.Learned)
	base.Learned = base.Learned || next.Learned
	return base
}

func withLearnedFields(base TierLimits) TierLimits {
	fields := make(map[string]bool)
	for key, value := range base.learnedFields {
		fields[key] = value
	}
	if base.learnedFields == nil && base.Learned {
		fields["minute"] = base.RequestsPerMinute != nil
		fields["daily"] = base.RequestsPerDay != nil
		fields["short_window"] = base.RequestsPerWindow != nil
		fields["concurrency"] = base.MaxConcurrentTurns != nil
	}
	base.learnedFields = fields
	return base
}

func ValidateTierLimits(limits TierLimits) error {
	for _, v := range []*float64{limits.MaxConcurrentTurns, limits.RequestsPerMinute, limits.RequestsPerDay, limits.RequestsPerWindow} {
		if !validLimitNumber(v) {
			return fmt.Errorf("limits must be finite non-negative numbers")
		}
	}
	if limits.DailyReset != "" {
		if _, _, err := DailyLimitWindow(time.Now(), limits.DailyReset); err != nil {
			return err
		}
	}
	if positive(limits.RequestsPerWindow) && limits.RequestWindowSeconds <= 0 {
		return fmt.Errorf("request window must have a positive duration")
	}
	for _, b := range limits.Windows {
		if err := validateLimitBudget(b); err != nil {
			return err
		}
	}
	for _, scope := range limits.Scopes {
		if err := validateRequestScope(scope); err != nil {
			return err
		}
	}
	return nil
}

func validLimitNumber(v *float64) bool {
	return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= 0)
}

func validateLimitBudget(b LimitBudget) error {
	if !validLimitNumber(b.Value) {
		return fmt.Errorf("invalid budget value")
	}
	if !oneOf(b.Window, "5h", WindowPeriodDay, WindowPeriodWeek, WindowPeriodMonth) {
		return fmt.Errorf("invalid usage window")
	}
	if !oneOf(b.Unit, "usd", "tokens", "percent", "requests", "turns") {
		return fmt.Errorf("invalid budget unit")
	}
	if !oneOf(b.Reset, "rolling", "utc", "provider", "anchor") {
		return fmt.Errorf("invalid reset rule")
	}
	if b.Window == "5h" && b.Reset == "utc" {
		return fmt.Errorf("a five-hour budget does not support a UTC calendar reset")
	}
	if b.Timezone != "" {
		if _, err := time.LoadLocation(b.Timezone); err != nil {
			return fmt.Errorf("invalid budget timezone")
		}
	}
	if b.Reset == "anchor" && b.ResetAt == nil {
		return fmt.Errorf("owner reset requires an anchor")
	}
	return nil
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
