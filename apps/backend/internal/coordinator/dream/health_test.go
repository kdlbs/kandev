package dream

import (
	"testing"
	"time"
)

var now0 = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time   { t := now0.Add(-d); return &t }
func ahead(d time.Duration) *time.Time { t := now0.Add(d); return &t }

func okInput() HealthInput {
	return HealthInput{Now: now0, Enabled: true, Autonomy: true, ContainmentOK: true, SpendMeasurable: true}
}

func TestHealthTable(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*HealthInput)
		state string
		cond  string
	}{
		{"off", func(i *HealthInput) { i.Enabled = false; i.Running = true }, HealthOff, ""},
		{"running", func(i *HealthInput) { i.Running = true; i.Paused = true }, HealthRunning, ""},
		{"paused before autonomy", func(i *HealthInput) { i.Paused = true; i.Autonomy = false }, HealthWaiting, WaitPaused},
		{"autonomy off", func(i *HealthInput) { i.Autonomy = false }, HealthWaiting, WaitAutonomyOff},
		{"containment", func(i *HealthInput) { i.ContainmentOK = false }, HealthWaiting, WaitContainment},
		{"spend unmeasurable", func(i *HealthInput) { i.SpendMeasurable = false }, HealthWaiting, WaitSpendUnmeasurable},
		{"ceiling", func(i *HealthInput) { i.AtCeiling = true }, HealthWaiting, WaitCeiling},
		{"admission before failed", func(i *HealthInput) { i.Autonomy = false; i.LastFailed = true }, HealthWaiting, WaitAutonomyOff},
		{"last failed", func(i *HealthInput) { i.LastFailed = true; i.LastFailedReason = "timeout" }, HealthFailed, WaitLastFailed},
		{"debt 72h is not failed", func(i *HealthInput) { i.DebtSince = ago(72 * time.Hour) }, HealthStale, WaitOverdue},
		{"debt over 72h failed", func(i *HealthInput) { i.DebtSince = ago(72*time.Hour + time.Second) }, HealthFailed, WaitOverdue},
		{"debt 36h is not stale", func(i *HealthInput) { i.DebtSince = ago(36 * time.Hour) }, HealthFresh, ""},
		{"debt over 36h stale", func(i *HealthInput) { i.DebtSince = ago(36*time.Hour + time.Second) }, HealthStale, WaitOverdue},
		{"accepted within 36h", func(i *HealthInput) { i.LastAcceptedAt = ago(35 * time.Hour); i.DebtSince = ago(time.Hour) }, HealthFresh, ""},
		{"accepted long ago, no debt", func(i *HealthInput) { i.LastAcceptedAt = ago(500 * time.Hour) }, HealthFresh, ""},
		{"no accepted, no debt", func(i *HealthInput) {}, HealthWaiting, WaitNoEvidence},
		{"debt but spacing", func(i *HealthInput) { i.DebtSince = ago(time.Hour); i.NextAfter = ahead(time.Hour) }, HealthWaiting, WaitSpacing},
		{"debt, spacing passed", func(i *HealthInput) { i.DebtSince = ago(time.Hour); i.NextAfter = ago(time.Minute) }, HealthFresh, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := okInput()
			c.mut(&in)
			got := Health(in)
			if got.State != c.state || got.Condition != c.cond {
				t.Fatalf("Health = %s/%s, want %s/%s", got.State, got.Condition, c.state, c.cond)
			}
		})
	}
}
