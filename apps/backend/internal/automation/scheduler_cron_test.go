package automation

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestNextCronFire_PinnedExpressionsFire pins the "never fires" bug: the old
// interval-reducing scheduler only understood a `*/N` step in the minute or
// hour field, so any pinned expression (a fixed minute/hour, a weekday range,
// a day-of-month) produced "no step interval found" and never fired. Each of
// these must now yield a concrete next-fire time.
func TestNextCronFire_PinnedExpressionsFire(t *testing.T) {
	// A Wednesday at 08:00 UTC — a deterministic anchor.
	after := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		expr string
		want time.Time
	}{
		{
			name: "daily at 09:00",
			expr: "0 9 * * *",
			want: time.Date(2026, time.July, 22, 9, 0, 0, 0, time.UTC),
		},
		{
			name: "daily at 14:30",
			expr: "30 14 * * *",
			want: time.Date(2026, time.July, 22, 14, 30, 0, 0, time.UTC),
		},
		{
			name: "weekdays at 09:15 skips to next weekday when after is already past",
			expr: "15 9 * * 1-5",
			// 08:00 Wed -> 09:15 same Wed.
			want: time.Date(2026, time.July, 22, 9, 15, 0, 0, time.UTC),
		},
		{
			name: "first of month at 00:00",
			expr: "0 0 1 * *",
			want: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "sunday weekly (named alias via standard field)",
			expr: "0 0 * * 0",
			// Next Sunday after Wed 22nd is the 26th.
			want: time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nextCronFire(tc.expr, "", after)
			if err != nil {
				t.Fatalf("nextCronFire(%q) error: %v", tc.expr, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("nextCronFire(%q) = %s, want %s", tc.expr, got.UTC(), tc.want)
			}
		})
	}
}

// TestNextCronFire_ShorthandStillWorks guards the shipped presets that DID work
// under the old scheduler (@every / @hourly / @daily / @weekly). They must keep
// firing after the rewrite.
func TestNextCronFire_ShorthandStillWorks(t *testing.T) {
	after := time.Date(2026, time.July, 22, 8, 30, 0, 0, time.UTC)

	cases := []struct {
		expr string
		want time.Time
	}{
		{"@every 5m", time.Date(2026, time.July, 22, 8, 35, 0, 0, time.UTC)},
		{"@hourly", time.Date(2026, time.July, 22, 9, 0, 0, 0, time.UTC)},
		{"@daily", time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC)},
		{"0 */6 * * *", time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := nextCronFire(tc.expr, "", after)
			if err != nil {
				t.Fatalf("nextCronFire(%q) error: %v", tc.expr, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("nextCronFire(%q) = %s, want %s", tc.expr, got.UTC(), tc.want)
			}
		})
	}
}

// TestNextCronFire_Timezone pins the timezone bug: "daily at 09:00" in a
// non-UTC zone must resolve to that zone's 09:00 wall-clock, not UTC's. The
// old scheduler ignored ScheduledTriggerConfig.Timezone entirely.
func TestNextCronFire_Timezone(t *testing.T) {
	// 08:00 UTC on 2026-07-22. In America/New_York (EDT, UTC-4 in July) that is
	// 04:00 local, so the next 09:00 EDT is 13:00 UTC the same day.
	after := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	got, err := nextCronFire("0 9 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	want := time.Date(2026, time.July, 22, 13, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("NY 09:00 in July: got %s, want %s (13:00 UTC)", got.UTC(), want)
	}

	// In winter (EST, UTC-5) the same 09:00 local is 14:00 UTC.
	afterWinter := time.Date(2026, time.January, 15, 6, 0, 0, 0, time.UTC)
	gotWinter, err := nextCronFire("0 9 * * *", "America/New_York", afterWinter)
	if err != nil {
		t.Fatalf("nextCronFire (winter) error: %v", err)
	}
	wantWinter := time.Date(2026, time.January, 15, 14, 0, 0, 0, time.UTC)
	if !gotWinter.Equal(wantWinter) {
		t.Fatalf("NY 09:00 in January: got %s, want %s (14:00 UTC)", gotWinter.UTC(), wantWinter)
	}
}

// TestNextCronFire_DSTSpringForward covers the DST edge: at the US spring
// forward (2026-03-08, 02:00 -> 03:00 local), a 09:00 daily schedule must
// still land on a real 09:00 local instant. The day the clocks jump, 09:00 EDT
// is 13:00 UTC.
func TestNextCronFire_DSTSpringForward(t *testing.T) {
	after := time.Date(2026, time.March, 8, 5, 0, 0, 0, time.UTC) // 00:00 EST that morning
	got, err := nextCronFire("0 9 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	want := time.Date(2026, time.March, 8, 13, 0, 0, 0, time.UTC) // 09:00 EDT
	if !got.Equal(want) {
		t.Fatalf("DST spring-forward 09:00 EDT: got %s, want %s", got.UTC(), want)
	}
}

// TestNextCronFire_DefaultsToUTCRegardlessOfHost pins the non-UTC-host
// requirement: with no timezone configured, the schedule is interpreted in UTC,
// independent of the process's local clock. We assert the computed instant is
// exactly UTC 09:00, which would differ if the host TZ leaked in.
func TestNextCronFire_DefaultsToUTCRegardlessOfHost(t *testing.T) {
	after := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	got, err := nextCronFire("0 9 * * *", "", after)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	want := time.Date(2026, time.July, 22, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("empty timezone should mean UTC: got %s, want %s", got.UTC(), want)
	}
}

// TestNextCronFire_InvalidExpression surfaces a clear error instead of silently
// never firing.
func TestNextCronFire_InvalidExpression(t *testing.T) {
	if _, err := nextCronFire("not a cron", "", time.Now()); err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
	if _, err := nextCronFire("0 9 * * *", "Mars/Phobos", time.Now()); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

// TestCronScheduler_PinnedExpression_FiresWhenDueThenWaits is the end-to-end
// regression for the pinned-expression bug through shouldFire: a "daily at
// 09:00 UTC" trigger created at 08:00 is not due at 08:59, is due at 09:00, and
// after firing (LastEvaluatedAt advanced) is not due again until the next day.
func TestCronScheduler_PinnedExpression_FiresWhenDueThenWaits(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	cs := NewCronScheduler(svc, log)

	created := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "0 9 * * *", Timezone: "UTC"})
	trig := &AutomationTrigger{
		Type:      TriggerTypeScheduled,
		Config:    cfg,
		Enabled:   true,
		CreatedAt: created,
	}

	if cs.shouldFire(trig, time.Date(2026, time.July, 22, 8, 59, 0, 0, time.UTC)) {
		t.Fatal("pinned 09:00 trigger should not be due at 08:59")
	}
	if !cs.shouldFire(trig, time.Date(2026, time.July, 22, 9, 0, 30, 0, time.UTC)) {
		t.Fatal("pinned 09:00 trigger should be due at 09:00:30")
	}

	// Simulate the fire advancing LastEvaluatedAt.
	fired := time.Date(2026, time.July, 22, 9, 0, 30, 0, time.UTC)
	trig.LastEvaluatedAt = &fired
	if cs.shouldFire(trig, time.Date(2026, time.July, 22, 9, 5, 0, 0, time.UTC)) {
		t.Fatal("pinned 09:00 trigger must not refire minutes after it fired")
	}
	if !cs.shouldFire(trig, time.Date(2026, time.July, 23, 9, 0, 5, 0, time.UTC)) {
		t.Fatal("pinned 09:00 trigger should be due again the next day at 09:00")
	}
}
