package shared

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// ErrUnsatisfiableCron indicates a syntactically valid but impossible cron
// expression (e.g. "0 0 30 2 *", February 30th) that can never match a
// wall-clock date.
var ErrUnsatisfiableCron = errors.New("cron expression can never fire")

// cronParser accepts the standard 5-field cron syntax: minute hour
// day-of-month month day-of-week. No descriptors (@daily, @every) - Office's
// contract is strictly 5 fields.
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// NextCronTime computes the next fire time strictly after `after` for the
// given 5-field cron expression, in the given timezone (empty means UTC).
//
// Day-of-month and day-of-week are ORed when both are restricted, matching
// crontab(5): "0 0 13 * 5" means the 13th of the month OR any Friday, not
// only Friday the 13th.
//
// DST policy: a wall-clock slot fires at most once. A slot that does not
// exist (spring-forward gap) is skipped. A slot that occurs twice (fall-back
// repeated hour) fires only on its first occurrence.
func NextCronTime(expression, timezone string, after time.Time) (time.Time, error) {
	loc, err := resolveLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	trimmed := strings.TrimSpace(expression)
	if len(strings.Fields(trimmed)) != 5 {
		return time.Time{}, fmt.Errorf("parse cron expression: %q: must be exactly 5 whitespace-separated fields (minute hour day-of-month month day-of-week); descriptors and TZ/CRON_TZ prefixes are not supported", expression)
	}
	schedule, err := cronParser.Parse(trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cron expression: %w", err)
	}
	specSchedule, _ := schedule.(*cron.SpecSchedule)
	start := after.In(loc)
	candidate := schedule.Next(start)
	if candidate.IsZero() {
		return time.Time{}, fmt.Errorf("%w: %q", ErrUnsatisfiableCron, expression)
	}
	for isAmbiguousFallBack(candidate) || !matchesWallClock(specSchedule, candidate) {
		candidate = schedule.Next(candidate)
		if candidate.IsZero() {
			return time.Time{}, fmt.Errorf("%w: %q", ErrUnsatisfiableCron, expression)
		}
	}
	if earlier, ok := findEarlierMatchAcrossSubHourTransition(specSchedule, loc, start, candidate); ok {
		candidate = earlier
	}
	return candidate.UTC(), nil
}

// findEarlierMatchAcrossSubHourTransition recovers a fire that
// schedule.Next skipped over because its hour-advancing loop steps by an
// absolute 1h and desynchronizes from a zone transition whose offset delta
// is not a whole hour (only Australia/Lord_Howe, +10:30<->+11:00, does this
// among IANA zones): the loop can jump straight past a day that has a
// genuinely matching slot. matchesWallClock cannot recover this on its own
// because it only validates candidates schedule.Next actually returns.
//
// This rescans (after, candidate) at minute granularity, but only when that
// interval contains such a transition, so no other zone's candidate path
// pays for the scan.
func findEarlierMatchAcrossSubHourTransition(spec *cron.SpecSchedule, loc *time.Location, after, candidate time.Time) (time.Time, bool) {
	if spec == nil || !intervalHasSubHourTransition(loc, after, candidate) {
		return time.Time{}, false
	}
	t := after.Truncate(time.Minute)
	if !t.After(after) {
		t = t.Add(time.Minute)
	}
	for t.Before(candidate) {
		if !isAmbiguousFallBack(t) && matchesWallClock(spec, t) {
			return t, true
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, false
}

// intervalHasSubHourTransition reports whether (after, candidate) contains a
// zone transition whose offset delta is not a whole number of hours, e.g.
// the 30-minute Australia/Lord_Howe shift.
func intervalHasSubHourTransition(loc *time.Location, after, candidate time.Time) bool {
	t := after.In(loc)
	for {
		_, end := t.ZoneBounds()
		if end.IsZero() || !end.Before(candidate) {
			return false
		}
		delta := zoneOffsetAt(end) - zoneOffsetAt(end.Add(-time.Second))
		if delta < 0 {
			delta = -delta
		}
		if delta%3600 != 0 {
			return true
		}
		t = end
	}
}

func zoneOffsetAt(t time.Time) int {
	_, offset := t.Zone()
	return offset
}

// matchesWallClock reports whether candidate's local wall-clock fields
// actually satisfy spec. In a sub-hour DST zone (e.g. Australia/Lord_Howe,
// +10:30/+11:00), robfig's minute-increment loop advances in absolute time:
// crossing a spring-forward gap can shift the hour by 30 minutes without
// re-entering the hour-matching loop, so the returned instant can satisfy
// the minute bitmask under a different, unmatched hour. spec is nil for a
// non-standard-field schedule (only 5-field expressions reach this package),
// in which case every candidate is treated as matching.
func matchesWallClock(spec *cron.SpecSchedule, candidate time.Time) bool {
	if spec == nil {
		return true
	}
	if 1<<uint(candidate.Month())&spec.Month == 0 {
		return false
	}
	if 1<<uint(candidate.Hour())&spec.Hour == 0 {
		return false
	}
	if 1<<uint(candidate.Minute())&spec.Minute == 0 {
		return false
	}
	if 1<<uint(candidate.Second())&spec.Second == 0 {
		return false
	}
	return dayMatches(spec, candidate)
}

// dayMatches mirrors robfig/cron's SpecSchedule.dayMatches (unexported):
// day-of-month and day-of-week are ANDed when neither is restricted (both
// carry starBit) and ORed otherwise, per crontab(5).
func dayMatches(spec *cron.SpecSchedule, t time.Time) bool {
	const starBit = 1 << 63
	domMatch := 1<<uint(t.Day())&spec.Dom > 0
	dowMatch := 1<<uint(t.Weekday())&spec.Dow > 0
	if spec.Dom&starBit > 0 || spec.Dow&starBit > 0 {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}

// resolveLocation maps an empty timezone to UTC, matching
// internal/automation/scheduler.go's nextCronFire.
func resolveLocation(timezone string) (*time.Location, error) {
	if timezone == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	return loc, nil
}

// isAmbiguousFallBack reports whether candidate is the second occurrence of
// a local wall-clock time made ambiguous by a DST fall-back transition.
//
// time.Date's disambiguation of ambiguous wall-clock fields is documented as
// implementation-defined ("the choice of time zone, and therefore the time,
// is not guaranteed"), and in practice resolves to different occurrences in
// different zone families (e.g. it picks the earlier instant in
// America/New_York but the later one in zones with a UTC+0 winter offset
// such as Europe/London). This instead reasons from candidate's own zone
// transition: candidate's period starts at the most recent offset change;
// if that change was a fall-back (offset decreased), the first
// offsetDelta-wide slice of the new period repeats wall-clock times already
// seen under the old offset.
func isAmbiguousFallBack(candidate time.Time) bool {
	start, _ := candidate.ZoneBounds()
	if start.IsZero() {
		return false
	}
	_, currentOffset := candidate.Zone()
	_, priorOffset := start.Add(-time.Second).Zone()
	if priorOffset <= currentOffset {
		return false // not a fall-back transition (spring-forward or no change)
	}
	repeatedWindow := time.Duration(priorOffset-currentOffset) * time.Second
	return candidate.Before(start.Add(repeatedWindow))
}
