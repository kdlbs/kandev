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

// TestNextCronFire_DSTFallBackFiresOnce covers the US fall-back boundary
// (2026-11-01, America/New_York, clocks fall from 02:00 EDT back to 01:00
// EST): a "30 1 * * *" schedule's wall-clock slot occurs twice as two
// distinct absolute instants. robfig's raw SpecSchedule.Next would return the
// first (-04:00) occurrence, and feeding that back in would return the
// second (-05:00) occurrence of the *same* local day rather than advancing to
// the next day. nextCronFire must suppress the repeat.
func TestNextCronFire_DSTFallBackFiresOnce(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	after := time.Date(2026, time.November, 1, 0, 59, 0, 0, loc) // unambiguous, before the repeated hour
	first, err := nextCronFire("30 1 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	wantFirst := time.Date(2026, time.November, 1, 1, 30, 0, 0, loc) // -04:00 (EDT)
	if !first.Equal(wantFirst) {
		t.Fatalf("first fall-back fire: got %s, want %s", first, wantFirst)
	}
	if _, offset := first.Zone(); offset != -4*3600 {
		t.Fatalf("first fall-back fire should be EDT (-04:00), got offset %d", offset)
	}

	second, err := nextCronFire("30 1 * * *", "America/New_York", first)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	// The ambiguous -05:00 sibling of the same local day must be skipped;
	// the next fire is the following day's 01:30 EST.
	wantSecond := time.Date(2026, time.November, 2, 1, 30, 0, 0, loc)
	if !second.Equal(wantSecond) {
		t.Fatalf("second fall-back fire: got %s, want %s (must skip repeated-hour sibling)", second, wantSecond)
	}
}

// TestNextCronFire_DSTFallBackFiresOnce_EuropeLondon covers a zone family
// where Go's ambiguous-time-construction rules resolve differently than for
// America/New_York (a naive time.Date reconstruction of the wall clock would
// pick a different occurrence in this zone family than in others): London
// falls back from BST (+01:00) to GMT (+00:00) on 2026-10-25. The second
// occurrence of "30 1 * * *" must still be suppressed.
func TestNextCronFire_DSTFallBackFiresOnce_EuropeLondon(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	after := time.Date(2026, time.October, 25, 0, 59, 0, 0, loc) // unambiguous, before the repeated hour
	first, err := nextCronFire("30 1 * * *", "Europe/London", after)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	if _, offset := first.Zone(); offset != 3600 {
		t.Fatalf("first fall-back fire should be BST (+01:00), got offset %d", offset)
	}

	second, err := nextCronFire("30 1 * * *", "Europe/London", first)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	wantSecond := time.Date(2026, time.October, 26, 1, 30, 0, 0, loc)
	if !second.Equal(wantSecond) {
		t.Fatalf("second fall-back fire: got %s, want %s (must skip repeated-hour sibling)", second, wantSecond)
	}
}

// TestNextCronFire_DSTFallBackFiresOnce_HalfHourOffset covers a zone with a
// non-hour-wide DST transition (Australia/Lord_Howe: +11:00 -> +10:30, a
// 30-minute fall-back on 2026-04-05), proving the suppression window is
// sized to the actual offset delta rather than hardcoded to an hour.
func TestNextCronFire_DSTFallBackFiresOnce_HalfHourOffset(t *testing.T) {
	loc, err := time.LoadLocation("Australia/Lord_Howe")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	after := time.Date(2026, time.April, 5, 0, 59, 0, 0, loc) // unambiguous, before the repeated (half-)hour
	first, err := nextCronFire("30 1 * * *", "Australia/Lord_Howe", after)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	if _, offset := first.Zone(); offset != 11*3600 {
		t.Fatalf("first fall-back fire should be +11:00, got offset %d", offset)
	}

	second, err := nextCronFire("30 1 * * *", "Australia/Lord_Howe", first)
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	wantSecond := time.Date(2026, time.April, 6, 1, 30, 0, 0, loc)
	if !second.Equal(wantSecond) {
		t.Fatalf("second fall-back fire: got %s, want %s (must skip repeated-hour sibling)", second, wantSecond)
	}
}

// TestNextCronFire_DSTFallBackNonRegression pins the boundaries the fix must
// leave untouched: a slot outside the repeated hour, plain UTC (no
// transitions), and the spring-forward gap.
func TestNextCronFire_DSTFallBackNonRegression(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	t.Run("outside repeated hour still fires once at expected instant", func(t *testing.T) {
		after := time.Date(2026, time.November, 1, 1, 0, 0, 0, loc)
		got, err := nextCronFire("0 3 * * *", "America/New_York", after)
		if err != nil {
			t.Fatalf("nextCronFire error: %v", err)
		}
		want := time.Date(2026, time.November, 1, 3, 0, 0, 0, loc) // EST, post-transition
		if !got.Equal(want) {
			t.Fatalf("got %s, want %s", got, want)
		}
	})

	t.Run("UTC has no transitions to suppress", func(t *testing.T) {
		after := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
		got, err := nextCronFire("30 1 * * *", "UTC", after)
		if err != nil {
			t.Fatalf("nextCronFire error: %v", err)
		}
		want := time.Date(2026, time.November, 1, 1, 30, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("got %s, want %s", got, want)
		}
	})

	t.Run("spring-forward gap is unaffected", func(t *testing.T) {
		// 2026-03-08 is the US spring-forward day (02:00 EST -> 03:00 EDT), so
		// 02:30 never exists that day; robfig already skips the non-existent
		// slot to the next valid occurrence (the following day), and this
		// change must not alter that.
		after := time.Date(2026, time.March, 8, 0, 0, 0, 0, loc)
		got, err := nextCronFire("30 2 * * *", "America/New_York", after)
		if err != nil {
			t.Fatalf("nextCronFire error: %v", err)
		}
		want := time.Date(2026, time.March, 9, 2, 30, 0, 0, loc) // EDT, next day (02:30 doesn't exist on the 8th)
		if !got.Equal(want) {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
}

// TestNextCronFire_EveryIntervalUnaffectedByFallBack proves @every
// (cron.ConstantDelaySchedule) is exempt from the wall-clock DST policy: it
// must keep firing every interval straight through the repeated hour rather
// than having fires displaced or dropped.
func TestNextCronFire_EveryIntervalUnaffectedByFallBack(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	after := time.Date(2026, time.November, 1, 1, 15, 0, 0, loc)
	wantOffsets := []time.Duration{30 * time.Minute, 30 * time.Minute, 30 * time.Minute}

	got := after
	for i, want := range wantOffsets {
		next, err := nextCronFire("@every 30m", "America/New_York", got)
		if err != nil {
			t.Fatalf("nextCronFire error: %v", err)
		}
		if delta := next.Sub(got); delta != want {
			t.Fatalf("fire %d: interval was %s, want %s", i, delta, want)
		}
		got = next
	}
}

// TestNextCronFire_UnsatisfiableExpressionReturnsZeroWithoutHanging pins the
// existing (out-of-scope) zero-time behavior for an unsatisfiable expression
// and proves the fall-back suppression loop added by this change does not
// spin forever on it.
func TestNextCronFire_UnsatisfiableExpressionReturnsZeroWithoutHanging(t *testing.T) {
	done := make(chan struct{})
	var got time.Time
	var err error
	go func() {
		got, err = nextCronFire("0 0 30 2 *", "America/New_York", time.Now())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("nextCronFire did not return for an unsatisfiable expression")
	}
	if err != nil {
		t.Fatalf("nextCronFire error: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("expected zero time for unsatisfiable expression, got %s", got)
	}
}

// TestCronScheduler_DSTFallBack_FiresOnceThenWaits is the end-to-end
// regression through shouldFire, mirroring
// TestCronScheduler_PinnedExpression_FiresWhenDueThenWaits: after firing at
// 01:30 EDT with LastEvaluatedAt advanced, the trigger must not be due again
// at the ambiguous 01:30 EST sibling.
func TestCronScheduler_DSTFallBack_FiresOnceThenWaits(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	cs := NewCronScheduler(svc, log)

	created := time.Date(2026, time.November, 1, 1, 0, 0, 0, loc)
	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "30 1 * * *", Timezone: "America/New_York"})
	trig := &AutomationTrigger{
		Type:      TriggerTypeScheduled,
		Config:    cfg,
		Enabled:   true,
		CreatedAt: created,
	}

	fireInstant := time.Date(2026, time.November, 1, 1, 30, 0, 0, loc) // -04:00
	if !cs.shouldFire(trig, fireInstant) {
		t.Fatal("trigger should be due at the first (EDT) 01:30 occurrence")
	}
	trig.LastEvaluatedAt = &fireInstant

	// The ambiguous second occurrence (-05:00, same wall-clock reading) must
	// not be seen as due again.
	ambiguousSibling := fireInstant.Add(time.Hour)
	if cs.shouldFire(trig, ambiguousSibling) {
		t.Fatal("trigger must not refire at the repeated-hour sibling instant")
	}

	// The next day's 01:30 EST is genuinely due.
	nextDay := time.Date(2026, time.November, 2, 1, 35, 0, 0, loc)
	if !cs.shouldFire(trig, nextDay) {
		t.Fatal("trigger should be due again the next day at 01:30 EST")
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
