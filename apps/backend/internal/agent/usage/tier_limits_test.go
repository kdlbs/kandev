package usage_test

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
)

func num(v float64) *float64 { return &v }

var tierNow = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestEvaluateTierLimitsMinuteBoundary(t *testing.T) {
	limits := usage.TierLimits{RequestsPerMinute: num(10)}
	cases := []struct {
		name string
		used float64
		deny bool
	}{
		{"just under", 8, false},
		{"last admitted request", 9, false},
		{"at limit", 10, true},
		{"over limit", 11, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := usage.EvaluateTierLimits(limits, usage.LimitUsage{RequestsMinute: tc.used}, tierNow)
			if (got.Reason != "") != tc.deny {
				t.Fatalf("used=%v admission=%+v, deny=%v", tc.used, got, tc.deny)
			}
			if tc.deny && (got.Reason != "limit_minute" || !got.RetryAt.Equal(tierNow.Add(time.Minute))) {
				t.Fatalf("admission = %+v", got)
			}
		})
	}
}

func TestEvaluateTierLimitsConcurrencyBoundary(t *testing.T) {
	limits := usage.TierLimits{MaxConcurrentTurns: num(2)}
	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{ActiveTurns: 1}, tierNow); got.Reason != "" {
		t.Fatalf("one active turn below limit 2: %+v", got)
	}
	got := usage.EvaluateTierLimits(limits, usage.LimitUsage{ActiveTurns: 2}, tierNow)
	if got.Reason != "limit_concurrency" || !got.RetryAt.Equal(tierNow.Add(time.Second)) {
		t.Fatalf("at limit admission = %+v", got)
	}
}

func TestEvaluateTierLimitsNilAndZeroAllowancesDoNotBlock(t *testing.T) {
	heavy := usage.LimitUsage{ActiveTurns: 99, RequestsMinute: 99, RequestsDay: 99, RequestsWindow: 99}
	if got := usage.EvaluateTierLimits(usage.TierLimits{}, heavy, tierNow); got.Reason != "" {
		t.Fatalf("unknown limits blocked: %+v", got)
	}
	zero := usage.TierLimits{MaxConcurrentTurns: num(0), RequestsPerMinute: num(0), RequestsPerDay: num(0), RequestsPerWindow: num(0)}
	if got := usage.EvaluateTierLimits(zero, heavy, tierNow); got.Reason != "" {
		t.Fatalf("explicit zero allowances blocked: %+v", got)
	}
}

func TestEvaluateTierLimitsShortWindowUsesConfiguredDuration(t *testing.T) {
	limits := usage.TierLimits{RequestsPerWindow: num(5), RequestWindowSeconds: 30}
	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{RequestsWindow: 4}, tierNow); got.Reason != "" {
		t.Fatalf("under short window limit: %+v", got)
	}
	got := usage.EvaluateTierLimits(limits, usage.LimitUsage{RequestsWindow: 5}, tierNow)
	if got.Reason != "limit_minute" || !got.RetryAt.Equal(tierNow.Add(30*time.Second)) {
		t.Fatalf("admission = %+v", got)
	}
}

func TestEvaluateTierLimitsDailyBoundaryAndReset(t *testing.T) {
	limits := usage.TierLimits{RequestsPerDay: num(100)}
	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{RequestsDay: 99}, tierNow); got.Reason != "" {
		t.Fatalf("99 of 100: %+v", got)
	}
	got := usage.EvaluateTierLimits(limits, usage.LimitUsage{RequestsDay: 100}, tierNow)
	wantReset := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	if got.Reason != "limit_daily" || !got.RetryAt.Equal(wantReset) {
		t.Fatalf("default reset admission = %+v, want retry %v", got, wantReset)
	}

	limits.DailyReset = "09:30 Asia/Seoul"
	got = usage.EvaluateTierLimits(limits, usage.LimitUsage{RequestsDay: 100}, tierNow)
	wantReset = time.Date(2026, 3, 11, 0, 30, 0, 0, time.UTC) // 09:30 KST next day
	if got.Reason != "limit_daily" || !got.RetryAt.Equal(wantReset) {
		t.Fatalf("zoned reset admission = %+v, want retry %v", got, wantReset)
	}
}

func TestEvaluateTierLimitsLearnedAllowanceKeepsFivePercentMargin(t *testing.T) {
	// Merging a learned value marks only that field as learned.
	learned := usage.MergeTierLimits(usage.TierLimits{}, usage.TierLimits{RequestsPerMinute: num(20), Learned: true})
	if got := usage.EvaluateTierLimits(learned, usage.LimitUsage{RequestsMinute: 18}, tierNow); got.Reason != "" {
		t.Fatalf("18 of learned 20 (cap 19): %+v", got)
	}
	if got := usage.EvaluateTierLimits(learned, usage.LimitUsage{RequestsMinute: 19}, tierNow); got.Reason != "limit_minute" {
		t.Fatalf("19 of learned 20 should hit the 95 percent margin: %+v", got)
	}
	owner := usage.MergeTierLimits(usage.TierLimits{}, usage.TierLimits{RequestsPerMinute: num(20)})
	if got := usage.EvaluateTierLimits(owner, usage.LimitUsage{RequestsMinute: 19}, tierNow); got.Reason != "" {
		t.Fatalf("owner value has no margin: %+v", got)
	}
}

func TestEvaluateTierLimitsWindowBudgets(t *testing.T) {
	week := usage.LimitBudget{Window: usage.WindowPeriodWeek, Unit: "tokens", Value: num(1000), Reset: "rolling"}
	limits := usage.TierLimits{Windows: []usage.LimitBudget{week}}
	key := week.Key()

	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{Windows: map[string]float64{key: 999}}, tierNow); got.Reason != "" {
		t.Fatalf("just under budget: %+v", got)
	}
	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{}, tierNow); got.Reason != "" {
		t.Fatalf("unknown usage must not block: %+v", got)
	}

	at := usage.EvaluateTierLimits(limits, usage.LimitUsage{Windows: map[string]float64{key: 1000}}, tierNow)
	if at.Reason != "limit_window" || !at.RetryAt.Equal(tierNow.Add(7*24*time.Hour)) {
		t.Fatalf("at budget with rolling reset = %+v", at)
	}

	provider := tierNow.Add(3 * time.Hour)
	used := usage.LimitUsage{Windows: map[string]float64{key: 1500}, Resets: map[string]time.Time{key: provider}}
	if got := usage.EvaluateTierLimits(limits, used, tierNow); got.Reason != "limit_window" || !got.RetryAt.Equal(provider) {
		t.Fatalf("future provider reset must win: %+v", got)
	}
	used.Resets[key] = tierNow.Add(-time.Minute)
	if got := usage.EvaluateTierLimits(limits, used, tierNow); !got.RetryAt.Equal(tierNow.Add(7 * 24 * time.Hour)) {
		t.Fatalf("stale reset must fall back to the computed window: %+v", got)
	}

	zero := usage.LimitBudget{Window: usage.WindowPeriodWeek, Unit: "tokens", Value: num(0), Reset: "rolling"}
	if got := usage.EvaluateTierLimits(usage.TierLimits{Windows: []usage.LimitBudget{zero}}, usage.LimitUsage{Windows: map[string]float64{key: 5}}, tierNow); got.Reason != "" {
		t.Fatalf("zero budget must not block: %+v", got)
	}
}

func TestEvaluateTierLimitsWindowWithoutResetRetriesInOneMinute(t *testing.T) {
	b := usage.LimitBudget{Window: usage.WindowPeriodDay, Unit: "tokens", Value: num(10), Reset: "provider"}
	limits := usage.TierLimits{Windows: []usage.LimitBudget{b}}
	got := usage.EvaluateTierLimits(limits, usage.LimitUsage{Windows: map[string]float64{b.Key(): 10}}, tierNow)
	if got.Reason != "limit_window" || !got.RetryAt.Equal(tierNow.Add(time.Minute)) {
		t.Fatalf("admission = %+v", got)
	}
}

func TestEvaluateTierLimitsLearnedBudgetMargin(t *testing.T) {
	b := usage.LimitBudget{Window: "5h", Unit: "tokens", Value: num(100), Reset: "rolling", Learned: true}
	limits := usage.TierLimits{Windows: []usage.LimitBudget{b}}
	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{Windows: map[string]float64{"5h": 94}}, tierNow); got.Reason != "" {
		t.Fatalf("94 of learned 100: %+v", got)
	}
	if got := usage.EvaluateTierLimits(limits, usage.LimitUsage{Windows: map[string]float64{"5h": 95}}, tierNow); got.Reason != "limit_window" {
		t.Fatalf("95 of learned 100: %+v", got)
	}
}

func TestEvaluateTierLimitsCheckOrder(t *testing.T) {
	limits := usage.TierLimits{MaxConcurrentTurns: num(1), RequestsPerMinute: num(1), RequestsPerDay: num(1)}
	used := usage.LimitUsage{ActiveTurns: 1, RequestsMinute: 1, RequestsDay: 1}
	if got := usage.EvaluateTierLimits(limits, used, tierNow); got.Reason != "limit_concurrency" {
		t.Fatalf("concurrency reported first: %+v", got)
	}
	used.ActiveTurns = 0
	if got := usage.EvaluateTierLimits(limits, used, tierNow); got.Reason != "limit_minute" {
		t.Fatalf("minute before daily: %+v", got)
	}
}

func TestMergeTierLimitsOwnerOverridesBaseAndNilInherits(t *testing.T) {
	base := usage.TierLimits{
		MaxConcurrentTurns: num(2), RequestsPerMinute: num(10), RequestsPerDay: num(100),
		DailyReset: "00:00 UTC", RequestsPerWindow: num(5), RequestWindowSeconds: 60,
	}
	merged := usage.MergeTierLimits(base, usage.TierLimits{RequestsPerMinute: num(0), DailyReset: "06:00 UTC"})
	if *merged.RequestsPerMinute != 0 {
		t.Fatalf("explicit zero must override: %v", *merged.RequestsPerMinute)
	}
	if *merged.MaxConcurrentTurns != 2 || *merged.RequestsPerDay != 100 || *merged.RequestsPerWindow != 5 || merged.RequestWindowSeconds != 60 {
		t.Fatalf("nil fields must inherit: %+v", merged)
	}
	if merged.DailyReset != "06:00 UTC" {
		t.Fatalf("daily reset = %q", merged.DailyReset)
	}
	if *base.RequestsPerMinute != 10 {
		t.Fatal("merge mutated base")
	}

	replaced := usage.MergeTierLimits(base, usage.TierLimits{RequestsPerWindow: num(7), RequestWindowSeconds: 120})
	if *replaced.RequestsPerWindow != 7 || replaced.RequestWindowSeconds != 120 {
		t.Fatalf("window allowance and duration move together: %+v", replaced)
	}
}

func TestMergeTierLimitsPrecedenceAcrossThreeLayers(t *testing.T) {
	defaults := usage.TierLimits{RequestsPerDay: num(50), RequestsPerMinute: num(10)}
	learned := usage.TierLimits{RequestsPerDay: num(40), Learned: true}
	owner := usage.TierLimits{RequestsPerDay: num(30)}
	merged := usage.MergeTierLimits(usage.MergeTierLimits(defaults, learned), owner)
	if *merged.RequestsPerDay != 30 || *merged.RequestsPerMinute != 10 {
		t.Fatalf("merged = %+v", merged)
	}
	// The owner value replaced the learned one, so it carries no learned margin.
	if got := usage.EvaluateTierLimits(merged, usage.LimitUsage{RequestsDay: 29}, tierNow); got.Reason != "" {
		t.Fatalf("owner day value 30 uses the full allowance: %+v", got)
	}
	if got := usage.EvaluateTierLimits(merged, usage.LimitUsage{RequestsDay: 30}, tierNow); got.Reason != "limit_daily" {
		t.Fatalf("at owner day value: %+v", got)
	}
	// A field the owner left unset keeps its learned status.
	learnedMinute := usage.MergeTierLimits(usage.MergeTierLimits(defaults, usage.TierLimits{RequestsPerMinute: num(20), Learned: true}), owner)
	if got := usage.EvaluateTierLimits(learnedMinute, usage.LimitUsage{RequestsMinute: 19}, tierNow); got.Reason != "limit_minute" {
		t.Fatalf("learned minute margin lost: %+v", got)
	}
}

func TestMergeTierLimitsBudgetsMergeByWindowAndScope(t *testing.T) {
	base := usage.TierLimits{Windows: []usage.LimitBudget{
		{Window: "5h", Unit: "tokens", Value: num(100), Reset: "rolling", Learned: true, WindowEstimated: true},
		{Window: usage.WindowPeriodWeek, Unit: "tokens", Value: num(1000), Reset: "rolling"},
		{Window: usage.WindowPeriodWeek, Unit: "tokens", Value: num(500), Reset: "rolling", Scope: string(usage.WindowScopeFreeModels)},
	}}
	next := usage.TierLimits{Windows: []usage.LimitBudget{
		{Window: "5h", Unit: "tokens", Reset: "anchor"},
		{Window: usage.WindowPeriodWeek, Unit: "tokens", Value: num(2000), Reset: "rolling", Scope: string(usage.WindowScopeFreeModels)},
		{Window: usage.WindowPeriodMonth, Unit: "tokens", Value: num(9000), Reset: "rolling"},
	}}
	merged := usage.MergeTierLimits(base, next)
	if len(merged.Windows) != 4 {
		t.Fatalf("windows = %+v", merged.Windows)
	}
	five := merged.Windows[0]
	if five.Value == nil || *five.Value != 100 || !five.Learned || !five.WindowEstimated || five.Reset != "anchor" {
		t.Fatalf("value-less owner budget must keep value and learned flags: %+v", five)
	}
	if *merged.Windows[1].Value != 1000 {
		t.Fatalf("account-wide week changed by a scoped budget: %+v", merged.Windows[1])
	}
	if *merged.Windows[2].Value != 2000 || merged.Windows[2].Scope != string(usage.WindowScopeFreeModels) {
		t.Fatalf("scoped week not replaced: %+v", merged.Windows[2])
	}
	if merged.Windows[3].Window != usage.WindowPeriodMonth {
		t.Fatalf("new budget not appended: %+v", merged.Windows[3])
	}
	if len(base.Windows) != 3 || *base.Windows[2].Value != 500 {
		t.Fatal("merge mutated base windows")
	}
}

func TestMergeTierLimitsLearnedNextMarksBudgetsAndRequestsPerModelSticks(t *testing.T) {
	merged := usage.MergeTierLimits(usage.TierLimits{RequestsPerModel: true}, usage.TierLimits{
		Learned: true,
		Windows: []usage.LimitBudget{{Window: "5h", Unit: "tokens", Value: num(10), Reset: "rolling"}},
	})
	if !merged.Learned || !merged.Windows[0].Learned {
		t.Fatalf("learned flags = %+v", merged)
	}
	if !merged.RequestsPerModel {
		t.Fatal("RequestsPerModel lost when the next layer leaves it unset")
	}
	if got := usage.MergeTierLimits(usage.TierLimits{}, usage.TierLimits{RequestsPerModel: true}); !got.RequestsPerModel {
		t.Fatal("RequestsPerModel not set by next layer")
	}
}

func TestMergeTierLimitsScopesMergeByID(t *testing.T) {
	base := usage.TierLimits{Scopes: []usage.RequestScope{
		{ID: usage.ModelScopeID("a"), Kind: usage.ScopeKindModel, Models: []string{"a"}, RequestsPerDay: num(10), DailyReset: "00:00 UTC"},
	}}
	next := usage.TierLimits{Scopes: []usage.RequestScope{
		{ID: usage.ModelScopeID("a"), RequestsPerDay: num(5)},
		{ID: "group:b+c", Kind: usage.ScopeKindGroup, Models: []string{"b", "c"}, RequestsPerDay: num(20)},
	}}
	merged := usage.MergeTierLimits(base, next)
	if len(merged.Scopes) != 2 {
		t.Fatalf("scopes = %+v", merged.Scopes)
	}
	a := merged.Scopes[0]
	if *a.RequestsPerDay != 5 || a.Kind != usage.ScopeKindModel || len(a.Models) != 1 || a.DailyReset != "00:00 UTC" {
		t.Fatalf("same-ID scope must update the value and keep its definition: %+v", a)
	}
	if merged.Scopes[1].ID != "group:b+c" {
		t.Fatalf("new scope not appended: %+v", merged.Scopes[1])
	}
	if *base.Scopes[0].RequestsPerDay != 10 {
		t.Fatal("merge mutated base scopes")
	}
}

func TestEvaluateScopeLimitsModelAndGroupCounts(t *testing.T) {
	model := usage.RequestScope{ID: usage.ModelScopeID("fast"), Kind: usage.ScopeKindModel, Models: []string{"fast"}, RequestsPerDay: num(10)}
	group := usage.RequestScope{ID: "group:a+b", Kind: usage.ScopeKindGroup, Models: []string{"a", "b"}, RequestsPerDay: num(20)}
	scopes := []usage.RequestScope{model, group}
	counts := usage.LimitUsage{Scopes: map[string]float64{model.ID: 10, group.ID: 19}}

	if got := usage.EvaluateScopeLimits(scopes, "provider/fast", counts, tierNow); got.Reason != "limit_daily" {
		t.Fatalf("exhausted model scope (provider prefix stripped): %+v", got)
	}
	if got := usage.EvaluateScopeLimits(scopes, "provider/other", counts, tierNow); got.Reason != "" {
		t.Fatalf("scope must not cover an unlisted model: %+v", got)
	}
	if got := usage.EvaluateScopeLimits(scopes, "p/A", counts, tierNow); got.Reason != "" {
		t.Fatalf("group at 19 of 20: %+v", got)
	}
	counts.Scopes[group.ID] = 20
	for _, m := range []string{"p/a", "p/b"} {
		got := usage.EvaluateScopeLimits(scopes, m, counts, tierNow)
		if got.Reason != "limit_daily" || !got.RetryAt.Equal(time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("group member %s: %+v", m, got)
		}
	}
	scopes[1].RequestsPerDay = num(0)
	if got := usage.EvaluateScopeLimits(scopes, "p/a", counts, tierNow); got.Reason != "" {
		t.Fatalf("zero scope allowance must not block: %+v", got)
	}
}

func TestEvaluateScopeLimitsLearnedMarginAndOwnReset(t *testing.T) {
	scope := usage.RequestScope{ID: usage.ModelScopeID("m"), Models: []string{"m"}, RequestsPerDay: num(20), Learned: true, DailyReset: "12:00 UTC"}
	counts := usage.LimitUsage{Scopes: map[string]float64{scope.ID: 19}}
	got := usage.EvaluateScopeLimits([]usage.RequestScope{scope}, "m", counts, tierNow)
	if got.Reason != "limit_daily" || !got.RetryAt.Equal(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("learned scope admission = %+v", got)
	}
	counts.Scopes[scope.ID] = 18
	if got := usage.EvaluateScopeLimits([]usage.RequestScope{scope}, "m", counts, tierNow); got.Reason != "" {
		t.Fatalf("18 of learned 20: %+v", got)
	}
}

func TestDailyLimitWindowResetBoundary(t *testing.T) {
	start, reset, err := usage.DailyLimitWindow(tierNow, "")
	if err != nil || !start.Equal(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)) || !reset.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("default window = %v %v %v", start, reset, err)
	}
	beforeReset := time.Date(2026, 3, 10, 5, 59, 59, 0, time.UTC)
	start, reset, _ = usage.DailyLimitWindow(beforeReset, "06:00 UTC")
	if !start.Equal(time.Date(2026, 3, 9, 6, 0, 0, 0, time.UTC)) || !reset.Equal(time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("just before reset = %v %v", start, reset)
	}
	start, reset, _ = usage.DailyLimitWindow(time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC), "06:00 UTC")
	if !start.Equal(time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC)) || !reset.Equal(time.Date(2026, 3, 11, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("at reset instant = %v %v", start, reset)
	}
	for _, bad := range []string{"06:00", "25:00 UTC", "06:00 Not/AZone"} {
		if _, _, err := usage.DailyLimitWindow(tierNow, bad); err == nil {
			t.Fatalf("rule %q accepted", bad)
		}
	}
}

func TestLimitWindowBoundsResetModes(t *testing.T) {
	cases := []struct {
		name       string
		budget     usage.LimitBudget
		ok         bool
		start, end time.Time
	}{
		{"rolling day", usage.LimitBudget{Window: usage.WindowPeriodDay, Reset: "rolling"}, true, tierNow.Add(-24 * time.Hour), tierNow.Add(24 * time.Hour)},
		{"rolling 5h", usage.LimitBudget{Window: "5h", Reset: "rolling"}, true, tierNow.Add(-5 * time.Hour), tierNow.Add(5 * time.Hour)},
		{"utc day", usage.LimitBudget{Window: usage.WindowPeriodDay, Reset: "utc"}, true, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)},
		{"utc month", usage.LimitBudget{Window: usage.WindowPeriodMonth, Reset: "utc"}, true, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		// 2026-03-10 is a Tuesday; the UTC week starts on Monday.
		{"utc week", usage.LimitBudget{Window: usage.WindowPeriodWeek, Reset: "utc"}, true, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"provider without reset time", usage.LimitBudget{Window: usage.WindowPeriodDay, Reset: "provider"}, false, time.Time{}, time.Time{}},
		{"anchor without reset time", usage.LimitBudget{Window: usage.WindowPeriodDay, Reset: "anchor"}, false, time.Time{}, time.Time{}},
		{"unknown window", usage.LimitBudget{Window: "year", Reset: "rolling"}, false, time.Time{}, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := usage.LimitWindowBounds(tierNow, tc.budget)
			if ok != tc.ok || !start.Equal(tc.start) || !end.Equal(tc.end) {
				t.Fatalf("bounds = %v %v %v, want %v %v %v", start, end, ok, tc.start, tc.end, tc.ok)
			}
		})
	}
}

func TestLimitWindowBoundsAnchoredDayFollowsOwnerAnchor(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	b := usage.LimitBudget{Window: usage.WindowPeriodDay, Reset: "anchor", ResetAt: &anchor}
	start, end, ok := usage.LimitWindowBounds(tierNow, b)
	if !ok || !start.Equal(time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC)) || !end.Equal(time.Date(2026, 3, 11, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("anchored day = %v %v %v", start, end, ok)
	}
	b.Timezone = "Not/AZone"
	if _, _, ok := usage.LimitWindowBounds(tierNow, b); ok {
		t.Fatal("invalid anchor timezone accepted")
	}
}
