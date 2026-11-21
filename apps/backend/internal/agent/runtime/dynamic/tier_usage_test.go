package dynamic

import (
	"testing"
	"time"
)

func usageCandidateAt(now time.Time, id string, value CandidateUsage) Candidate {
	value.FetchedAt = now
	if value.Bucket.Provider == "" {
		value.Bucket = UsageBucket{Provider: "anthropic", AccountRef: "account-" + id, Window: "7-day"}
	}
	return Candidate{ID: id, Enabled: true, BindingKey: id, Usage: value}
}

func rankedUsage(now time.Time, mode string, candidates ...Candidate) []rankedTierRow {
	return rankTierRows(orderedTier{mode: mode, rows: candidates}, 1, nil, "", nil, nil, nil, now)
}

func usageIDs(rows []rankedTierRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.candidate.ID)
	}
	return ids
}

func floatPtr(value float64) *float64 { return &value }

// A measured burn that would exhaust the allowance before a distant reset must
// not outrank a lower allowance whose reset arrives first.
func TestPaceRanksByTheResetDeadline(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	soon := usageCandidateAt(now, "soon", CandidateUsage{
		Known: true, RemainingPct: 20,
		ConsumptionRateKnown: true, ConsumptionPctPerHour: 1,
		ResetAtKnown: true, ResetAt: now.Add(time.Hour),
	})
	late := usageCandidateAt(now, "late", CandidateUsage{
		Known: true, RemainingPct: 30,
		ConsumptionRateKnown: true, ConsumptionPctPerHour: 10,
		ResetAtKnown: true, ResetAt: now.Add(10 * time.Hour),
	})

	rows := rankedUsage(now, tierModePace, late, soon)
	if got := usageIDs(rows); len(got) != 2 || got[0] != "soon" {
		t.Fatalf("pace order = %v, want the imminent reset first", got)
	}
	headroom := rankedUsage(now, tierModeHeadroom, late, soon)
	if got := usageIDs(headroom); len(got) != 2 || got[0] != "late" {
		t.Fatalf("headroom order = %v, want the higher remaining first", got)
	}
}

func TestPaceWithoutARateFallsBackToRemaining(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	low := usageCandidateAt(now, "low", CandidateUsage{
		Known: true, RemainingPct: 20, ResetAtKnown: true, ResetAt: now.Add(time.Hour),
	})
	high := usageCandidateAt(now, "high", CandidateUsage{
		Known: true, RemainingPct: 30, ResetAtKnown: true, ResetAt: now.Add(10 * time.Hour),
	})

	rows := rankedUsage(now, tierModePace, low, high)
	if got := usageIDs(rows); len(got) != 2 || got[0] != "high" {
		t.Fatalf("pace order without a rate = %v, want remaining order", got)
	}
}

func TestHeadroomIgnoresTheResetDeadline(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	a := usageCandidateAt(now, "a", CandidateUsage{
		Known: true, RemainingPct: 40,
		ConsumptionRateKnown: true, ConsumptionPctPerHour: 4,
		ResetAtKnown: true, ResetAt: now.Add(time.Hour),
	})
	b := usageCandidateAt(now, "b", CandidateUsage{
		Known: true, RemainingPct: 40,
		ConsumptionRateKnown: true, ConsumptionPctPerHour: 4,
		ResetAtKnown: true, ResetAt: now.Add(20 * time.Hour),
	})
	rows := rankedUsage(now, tierModeHeadroom, a, b)
	if got := usageIDs(rows); len(got) != 2 || got[0] != "a" {
		t.Fatalf("headroom order = %v, want stored order on equal remaining", got)
	}
}

func TestUsageRankingGroupsKnownUnknownAndExhausted(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	exhausted := usageCandidateAt(now, "exhausted", CandidateUsage{Known: true, RemainingPct: 0})
	unknown := usageCandidateAt(now, "unknown", CandidateUsage{})
	available := usageCandidateAt(now, "available", CandidateUsage{Known: true, RemainingPct: 50})

	rows := rankedUsage(now, tierModePace, exhausted, unknown, available)
	got := usageIDs(rows)
	want := []string{"available", "unknown", "exhausted"}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestUnknownUsageOrdersByKnownRate(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	rows := []rankedTierRow{
		{entry: TierPreviewEntry{CandidateID: "plain", Mode: tierModePace}},
		{entry: TierPreviewEntry{
			CandidateID: "fast", Mode: tierModePace,
			ConsumptionRateKnown: true, ConsumptionPctPerHour: floatPtr(9),
		}},
		{entry: TierPreviewEntry{
			CandidateID: "slow", Mode: tierModePace,
			ConsumptionRateKnown: true, ConsumptionPctPerHour: floatPtr(1),
		}},
	}
	ranked := rankUsageRows(rows, tierModePace, now)
	got := make([]string, 0, len(ranked))
	for _, row := range ranked {
		got = append(got, row.entry.CandidateID)
	}
	want := []string{"slow", "fast", "plain"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unknown order = %v, want %v", got, want)
		}
	}
}

func TestSharedBucketWithConflictingResetsKeepsTheRemainingReading(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	bucket := UsageBucket{Provider: "anthropic", AccountRef: "shared", Window: "7-day"}
	first := usageCandidateAt(now, "first", CandidateUsage{
		Bucket: bucket, Known: true, RemainingPct: 40,
		ResetAtKnown: true, ResetAt: now.Add(time.Hour),
	})
	second := usageCandidateAt(now, "second", CandidateUsage{
		Bucket: bucket, Known: true, RemainingPct: 40,
		ResetAtKnown: true, ResetAt: now.Add(3 * time.Hour),
	})
	rows := []rankedTierRow{{candidate: first}, {candidate: second}}
	applyTierUsage(rows, now)
	for _, row := range rows {
		if !row.entry.UsageKnown || row.entry.RemainingPct == nil || *row.entry.RemainingPct != 40 {
			t.Fatalf("%s lost its agreed remaining reading: %#v", row.candidate.ID, row.entry)
		}
		if row.entry.resetAtKnown {
			t.Fatalf("%s claimed one of the conflicting resets", row.candidate.ID)
		}
	}
}

func TestNormalizedUsageRejectsStaleOrAbsentReset(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	base := CandidateUsage{
		Bucket:    UsageBucket{Provider: "anthropic", AccountRef: "account", Window: "7-day"},
		FetchedAt: now, Known: true, RemainingPct: 50,
	}
	stale := base
	stale.ResetAtKnown, stale.ResetAt = true, now.Add(-time.Minute)
	if got := normalizedCandidateUsage(stale, now); got.ResetAtKnown || !got.Known {
		t.Fatalf("stale reset = %#v, want the reading kept and the reset dropped", got)
	}
	absent := base
	absent.ResetAtKnown = true
	if got := normalizedCandidateUsage(absent, now); got.ResetAtKnown {
		t.Fatalf("absent reset = %#v, want the reset dropped", got)
	}
	future := base
	future.ResetAtKnown, future.ResetAt = true, now.Add(time.Hour)
	if got := normalizedCandidateUsage(future, now); !got.ResetAtKnown || !got.ResetAt.Equal(future.ResetAt) {
		t.Fatalf("future reset = %#v, want it kept", got)
	}
}

func TestUsageTierRequiresTheProviderUsageSurface(t *testing.T) {
	candidates := []Candidate{{ID: "a", Enabled: true, SelectionJSON: usageTierSelectionJSON(t, tierModePace)}}
	if _, err := deriveOrderedTiers(candidates, false); err == nil {
		t.Fatal("usage mode was accepted while the provider usage surface is off")
	}
	if _, err := deriveOrderedTiers(candidates, true); err != nil {
		t.Fatalf("usage mode rejected while the provider usage surface is on: %v", err)
	}
}
