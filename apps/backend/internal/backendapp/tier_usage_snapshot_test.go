package backendapp

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func tierUsageTestReading(now time.Time, utilization float64, reset *time.Time) dto.AgentProfileUsageDTO {
	at := now
	return dto.AgentProfileUsageDTO{
		ProfileID: "profile-1", ModelID: "model", State: "ok", Provider: "anthropic",
		AccountID: "anthropic:profile-1", FetchedAt: &at,
		Windows: []dto.AgentProfileUsageWindowDTO{
			{Label: "5-hour", UtilizationPct: 10},
			{Label: "7-day", UtilizationPct: utilization, ResetAt: reset},
		},
	}
}

func resetPtr(at time.Time) *time.Time { return &at }

func TestTierUsageSnapshotUsesTheConstrainingWindow(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	reset := now.Add(time.Hour)
	snapshot := newTierUsageSnapshot()
	if got := snapshot.Lookup("profile-1"); got.Known {
		t.Fatalf("unobserved lookup = %#v, want unknown", got)
	}
	snapshot.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now, 40, resetPtr(reset))})
	got := snapshot.Lookup("profile-1")
	if !got.Known || got.RemainingPct != 60 || got.Bucket.Window != "7-day" || got.Bucket.AccountRef != "anthropic:profile-1" {
		t.Fatalf("lookup = %#v", got)
	}
	if !got.ResetAtKnown || !got.ResetAt.Equal(reset) {
		t.Fatalf("reset = %v known=%v", got.ResetAt, got.ResetAtKnown)
	}
	if got.ConsumptionRateKnown {
		t.Fatalf("one sample claimed a rate: %#v", got)
	}
}

func TestTierUsageSnapshotRequiresTwoProvedResetsForRate(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	reset := now.Add(time.Hour)
	other := now.Add(2 * time.Hour)

	snapshot := newTierUsageSnapshot()
	snapshot.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now, 30, resetPtr(reset))})
	snapshot.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now.Add(5*time.Minute), 40, resetPtr(reset))})
	if got := snapshot.Lookup("profile-1"); !got.ConsumptionRateKnown || got.ConsumptionPctPerHour != 120 {
		t.Fatalf("rate = %v known=%v, want 120%%/h", got.ConsumptionPctPerHour, got.ConsumptionRateKnown)
	}

	missingPrior := newTierUsageSnapshot()
	missingPrior.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now, 30, nil)})
	missingPrior.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now.Add(5*time.Minute), 40, resetPtr(reset))})
	if got := missingPrior.Lookup("profile-1"); got.ConsumptionRateKnown {
		t.Fatalf("rate from a prior without a proved reset: %#v", got)
	}

	missingCurrent := newTierUsageSnapshot()
	missingCurrent.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now, 30, resetPtr(reset))})
	missingCurrent.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now.Add(5*time.Minute), 40, nil)})
	if got := missingCurrent.Lookup("profile-1"); got.ConsumptionRateKnown {
		t.Fatalf("rate from a current without a proved reset: %#v", got)
	}

	changedReset := newTierUsageSnapshot()
	changedReset.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now, 30, resetPtr(reset))})
	changedReset.Observe([]dto.AgentProfileUsageDTO{tierUsageTestReading(now.Add(5*time.Minute), 40, resetPtr(other))})
	if got := changedReset.Lookup("profile-1"); got.ConsumptionRateKnown {
		t.Fatalf("rate across a changed reset: %#v", got)
	}
}

func TestTierUsageSnapshotKeepsUnavailableReadingsUnknown(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	reset := now.Add(time.Hour)
	base := tierUsageTestReading(now, 40, resetPtr(reset))

	cases := map[string]dto.AgentProfileUsageDTO{
		"stale":       func() dto.AgentProfileUsageDTO { v := base; v.Stale = true; return v }(),
		"unavailable": func() dto.AgentProfileUsageDTO { v := base; v.State = "unavailable"; return v }(),
		"no account":  func() dto.AgentProfileUsageDTO { v := base; v.AccountID = ""; return v }(),
		"no windows":  func() dto.AgentProfileUsageDTO { v := base; v.Windows = nil; return v }(),
	}
	for name, reading := range cases {
		snapshot := newTierUsageSnapshot()
		snapshot.Observe([]dto.AgentProfileUsageDTO{reading})
		if got := snapshot.Lookup("profile-1"); got.Known {
			t.Fatalf("%s observation = %#v, want unknown", name, got)
		}
	}
	if got, ok := candidateUsageFromDTO(base); !ok || !got.Known || got.Limited {
		t.Fatalf("valid reading = %#v ok=%v", got, ok)
	}
	limited := tierUsageTestReading(now, 100, resetPtr(reset))
	limited.Windows[1].LimitReached = true
	if got, ok := candidateUsageFromDTO(limited); !ok || !got.Limited {
		t.Fatalf("limited reading = %#v ok=%v", got, ok)
	}
}
