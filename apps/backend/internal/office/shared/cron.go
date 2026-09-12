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
	specSchedule, ok := schedule.(*cron.SpecSchedule)
	if !ok {
		// cronParser is configured with only the 5 standard fields (no
		// descriptors), so Parse always returns *SpecSchedule for a
		// well-formed 5-field expression. This would only trip if
		// robfig/cron's internal type changed — fail loudly rather than
		// silently degrading matchesWallClock to "everything matches".
		return time.Time{}, fmt.Errorf("internal: unexpected schedule type %T", schedule)
	}
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
// This rescans only the local days around each affected transition at minute
// granularity, so a sparse schedule cannot turn the shared scheduler tick
// into a multi-year scan.
func findEarlierMatchAcrossSubHourTransition(spec *cron.SpecSchedule, loc *time.Location, after, candidate time.Time) (time.Time, bool) {
	if spec == nil {
		return time.Time{}, false
	}

	var earlier time.Time
	found := walkZoneTransitions(loc, after, candidate, func(transition time.Time) bool {
		delta := zoneOffsetAt(transition) - zoneOffsetAt(transition.Add(-time.Second))
		if delta < 0 {
			delta = -delta
		}
		if delta%3600 == 0 {
			return false
		}

		windowStart, windowEnd := transitionDayWindow(loc, transition)
		t := after.Truncate(time.Minute)
		if !t.After(after) {
			t = t.Add(time.Minute)
		}
		if t.Before(windowStart) {
			t = windowStart
		}
		if candidate.Before(windowEnd) {
			windowEnd = candidate
		}
		for t.Before(windowEnd) {
			if !isAmbiguousFallBack(t) && matchesWallClock(spec, t) {
				earlier = t
				return true
			}
			t = t.Add(time.Minute)
		}
		return false
	})
	return earlier, found
}

const maxZoneTransitionHops = 512

// walkZoneTransitions visits each transition strictly between after and
// candidate. Some TZif files use a POSIX extension whose ZoneBounds result
// can stop advancing at a leap-year boundary, so a non-progressing result is
// advanced to the next local year before the next lookup. The hop limit is a
// final guard against malformed or unexpectedly long timezone data.
func walkZoneTransitions(loc *time.Location, after, candidate time.Time, visit func(time.Time) bool) bool {
	if loc == nil || visit == nil || !after.Before(candidate) {
		return false
	}
	t := after.In(loc)
	for hops := 0; hops < maxZoneTransitionHops; hops++ {
		_, transition := t.ZoneBounds()
		if transition.IsZero() || !transition.Before(candidate) {
			return false
		}
		if !transition.After(t) {
			nextYear := time.Date(t.Year()+1, time.January, 1, 0, 0, 0, 0, loc)
			if !nextYear.After(t) {
				return false
			}
			t = nextYear
			continue
		}
		if visit(transition) {
			return true
		}
		t = transition
	}
	return false
}

func transitionDayWindow(loc *time.Location, transition time.Time) (time.Time, time.Time) {
	local := transition.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day()-1, 0, 0, 0, 0, loc)
	end := time.Date(local.Year(), local.Month(), local.Day()+2, 0, 0, 0, 0, loc)
	return start, end
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
// the minute bitmask under a different, unmatched hour. NextCronTime's own
// type-assertion guard means spec is never nil on that call path; the nil
// check here is defense against a future caller.
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
// when at least one of day-of-month or day-of-week is unrestricted (carries
// starBit), the fields are ANDed (the wildcard side is always true, so only
// the restricted side matters). When both are restricted, they are ORed,
// per crontab(5).
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
