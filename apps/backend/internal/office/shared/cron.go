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
	if isAmbiguousFallBack(candidate, loc) {
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
// Reconstructing the same wall-clock fields with time.Date always resolves
// to the first (pre-transition) offset; a mismatch means candidate is the
// later, repeated occurrence.
func isAmbiguousFallBack(candidate time.Time, loc *time.Location) bool {
	canonical := time.Date(
		candidate.Year(), candidate.Month(), candidate.Day(),
		candidate.Hour(), candidate.Minute(), candidate.Second(), candidate.Nanosecond(),
		loc,
	)
	return !canonical.Equal(candidate)
}
