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
	schedule, err := cronParser.Parse(strings.TrimSpace(expression))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cron expression: %w", err)
	}
	candidate := schedule.Next(after.In(loc))
	if candidate.IsZero() {
		return time.Time{}, fmt.Errorf("%w: %q", ErrUnsatisfiableCron, expression)
	}
	for isAmbiguousFallBack(candidate) {
		candidate = schedule.Next(candidate)
		if candidate.IsZero() {
			return time.Time{}, fmt.Errorf("%w: %q", ErrUnsatisfiableCron, expression)
		}
	}
	return candidate.UTC(), nil
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
