package usage

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Reset window periods. A five-hour window is a fixed duration; the others are
// calendar boundaries in the configured timezone, so a monthly plan resets on
// the calendar month rather than on a fixed 30-day duration.
const (
	WindowPeriodFiveHour = "five_hour"
	WindowPeriodDay      = "day"
	WindowPeriodWeek     = "week"
	WindowPeriodMonth    = "month"
)

// WindowUnit names the quantity a limit is expressed in.
const (
	WindowUnitMoney  = "money"
	WindowUnitTokens = "tokens"
)

// fiveHourDuration is the fixed length of a five-hour provider window.
const fiveHourDuration = 5 * time.Hour

// ResetAnchor describes when a user-entered window resets. The anchor supplies
// the local reset time, and optionally the weekday, the day of month and the
// fixed-window phase.
//
// The wire form is a space-separated list: a leading HH:MM time of day,
// followed by any of "mon".."sun", "day N" and "phase N" (whole hours).
// Examples: "09:00", "09:00 mon", "00:00 day 1", "09:00 mon phase 2".
type ResetAnchor struct {
	Hour       int
	Minute     int
	Weekday    time.Weekday
	HasWeekday bool
	DayOfMonth int
	PhaseHours int
}

// ParseResetAnchor parses the wire form. An unparsable anchor is an error, so a
// bad reset can never be silently replaced by a plausible looking instant.
func ParseResetAnchor(raw string) (ResetAnchor, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return ResetAnchor{}, fmt.Errorf("a reset anchor is required")
	}
	anchor, err := parseResetTimeOfDay(fields[0])
	if err != nil {
		return ResetAnchor{}, err
	}
	for rest := fields[1:]; len(rest) > 0; {
		consumed, err := anchor.applyAnchorField(rest)
		if err != nil {
			return ResetAnchor{}, err
		}
		rest = rest[consumed:]
	}
	return anchor, nil
}

func parseResetTimeOfDay(raw string) (ResetAnchor, error) {
	hour, minute, err := parseClockTime(raw)
	if err != nil {
		return ResetAnchor{}, err
	}
	return ResetAnchor{Hour: hour, Minute: minute}, nil
}

func parseClockTime(raw string) (int, int, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("reset time %q must be HH:MM", raw)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("reset hour %q must be 0 through 23", parts[0])
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("reset minute %q must be 0 through 59", parts[1])
	}
	return hour, minute, nil
}

// applyAnchorField consumes one or two tokens and returns how many it read.
// "day" and "phase" take a separate value token; a weekday name is its own key.
func (a *ResetAnchor) applyAnchorField(fields []string) (int, error) {
	key := fields[0]
	switch strings.ToLower(key) {
	case "mon", "tue", "wed", "thu", "fri", "sat", "sun":
		a.Weekday = weekdayFromName(key)
		a.HasWeekday = true
		return 1, nil
	case "day":
		value, err := anchorValue(key, fields)
		if err != nil {
			return 0, err
		}
		day, err := strconv.Atoi(value)
		if err != nil || day < 1 || day > 31 {
			return 0, fmt.Errorf("day of month %q must be 1 through 31", value)
		}
		a.DayOfMonth = day
		return 2, nil
	case "phase":
		value, err := anchorValue(key, fields)
		if err != nil {
			return 0, err
		}
		phase, err := strconv.Atoi(value)
		if err != nil || phase < 0 || phase > 23 {
			return 0, fmt.Errorf("phase %q must be 0 through 23", value)
		}
		a.PhaseHours = phase
		return 2, nil
	default:
		return 0, fmt.Errorf("unknown reset anchor field %q", key)
	}
}

func anchorValue(key string, fields []string) (string, error) {
	if len(fields) < 2 {
		return "", fmt.Errorf("%s anchor needs a value, for example %q", key, key+" 1")
	}
	return strings.TrimSpace(fields[1]), nil
}

func weekdayFromName(name string) time.Weekday {
	switch strings.ToLower(name) {
	case "mon":
		return time.Monday
	case "tue":
		return time.Tuesday
	case "wed":
		return time.Wednesday
	case "thu":
		return time.Thursday
	case "fri":
		return time.Friday
	case "sat":
		return time.Saturday
	default:
		return time.Sunday
	}
}

// ResetWindow is one half-open interval [Start, Reset). Start is the window
// start and Reset is the instant the window expires.
type ResetWindow struct {
	Start time.Time
	Reset time.Time
}

// ResolveResetWindow returns the window containing now, or ok=false when the
// period or the anchor cannot produce a valid window. A future start and a
// non-positive span are invalid rather than clamped, so a caller cannot score
// pace from a window that does not exist.
//
// Calendar periods are resolved with time.Date in the target location, so DST
// transitions and month-end lengths are handled by the zone rules rather than by
// adding fixed durations. A day-of-month beyond the length of a month clamps to
// that month's last valid day.
func ResolveResetWindow(now time.Time, period string, anchor ResetAnchor, loc *time.Location) (ResetWindow, bool) {
	if loc == nil {
		return ResetWindow{}, false
	}
	if period == WindowPeriodFiveHour {
		return resolveFiveHourWindow(now, anchor, loc)
	}
	switch period {
	case WindowPeriodDay:
		return resolveDayWindow(now, anchor, loc)
	case WindowPeriodWeek:
		return resolveWeeklyWindow(now, anchor, loc)
	case WindowPeriodMonth:
		return resolveMonthWindow(now, anchor, loc)
	default:
		return ResetWindow{}, false
	}
}

// resolveFiveHourWindow steps whole five-hour blocks from the anchor instant.
// Absolute stepping keeps the block length exact across a DST transition, which
// is what a provider's fixed window means.
func resolveFiveHourWindow(now time.Time, anchor ResetAnchor, loc *time.Location) (ResetWindow, bool) {
	// Fixed five-hour blocks form one continuous absolute grid. The anchor
	// sets the phase on a stable reference date, and the grid never re-anchors,
	// so an in-progress window is not reset before its scheduled reset.
	base := time.Date(2000, 1, 1, anchor.Hour, anchor.Minute, 0, 0, loc)
	base = base.Add(time.Duration(anchor.PhaseHours) * time.Hour)
	elapsed := now.Sub(base)
	blocks := elapsed / fiveHourDuration
	if elapsed < 0 && elapsed%fiveHourDuration != 0 {
		blocks--
	}
	reset := base.Add((blocks + 1) * fiveHourDuration)
	start := reset.Add(-fiveHourDuration)
	if !reset.After(start) || !reset.After(now) {
		return ResetWindow{}, false
	}
	return ResetWindow{Start: start, Reset: reset}, true
}

// fiveHourWindowFrom steps whole five-hour blocks from a concrete anchor
// instant, preserving the owner-supplied date and the fixed block length.
func fiveHourWindowFrom(now, anchor time.Time, loc *time.Location) (ResetWindow, bool) {
	base := anchor.In(loc)
	elapsed := now.Sub(base)
	blocks := elapsed / fiveHourDuration
	if elapsed < 0 && elapsed%fiveHourDuration != 0 {
		blocks--
	}
	reset := base.Add((blocks + 1) * fiveHourDuration)
	start := reset.Add(-fiveHourDuration)
	if !reset.After(start) || !reset.After(now) {
		return ResetWindow{}, false
	}
	return ResetWindow{Start: start, Reset: reset}, true
}

func resolveDayWindow(now time.Time, anchor ResetAnchor, loc *time.Location) (ResetWindow, bool) {
	local := now.In(loc)
	todayReset := time.Date(local.Year(), local.Month(), local.Day(), anchor.Hour, anchor.Minute, 0, 0, loc)
	if !now.Before(todayReset) {
		todayReset = time.Date(local.Year(), local.Month(), local.Day()+1, anchor.Hour, anchor.Minute, 0, 0, loc)
	}
	start := todayReset.AddDate(0, 0, -1)
	return validResetWindow(start, todayReset, now)
}

// resolveWeeklyWindow anchors the week on the configured weekday, defaulting to
// Monday when the anchor names none.
func resolveWeeklyWindow(now time.Time, anchor ResetAnchor, loc *time.Location) (ResetWindow, bool) {
	weekday := time.Monday
	if anchor.HasWeekday {
		weekday = anchor.Weekday
	}
	local := now.In(loc)
	thisWeekReset := time.Date(local.Year(), local.Month(), local.Day(), anchor.Hour, anchor.Minute, 0, 0, loc)
	thisWeekReset = shiftToWeekday(thisWeekReset, weekday, loc)
	if !now.Before(thisWeekReset) {
		thisWeekReset = shiftToWeekday(thisWeekReset.AddDate(0, 0, 7), weekday, loc)
	}
	start := shiftToWeekday(thisWeekReset.AddDate(0, 0, -7), weekday, loc)
	return validResetWindow(start, thisWeekReset, now)
}

func shiftToWeekday(value time.Time, weekday time.Weekday, loc *time.Location) time.Time {
	delta := (int(weekday) - int(value.In(loc).Weekday()) + 7) % 7
	if delta == 0 {
		return value
	}
	return time.Date(
		value.In(loc).Year(), value.In(loc).Month(), value.In(loc).Day()+delta,
		value.In(loc).Hour(), value.In(loc).Minute(), 0, 0, loc,
	)
}

// resolveMonthWindow resolves a calendar month, so a monthly plan resets on the
// calendar boundary rather than on a fixed 30-day duration. A day of month
// beyond the month's length clamps to the last valid day.
func resolveMonthWindow(now time.Time, anchor ResetAnchor, loc *time.Location) (ResetWindow, bool) {
	local := now.In(loc)
	dayOfMonth := anchor.DayOfMonth
	if dayOfMonth == 0 {
		dayOfMonth = 1
	}
	currentReset := monthReset(local.Year(), local.Month(), dayOfMonth, anchor, loc)
	if !now.Before(currentReset) {
		next := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, loc)
		currentReset = monthReset(next.Year(), next.Month(), dayOfMonth, anchor, loc)
	}
	previous := time.Date(currentReset.Year(), currentReset.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
	start := monthReset(previous.Year(), previous.Month(), dayOfMonth, anchor, loc)
	return validResetWindow(start, currentReset, now)
}

// monthReset clamps the day of month onto the last valid day of that month, so
// a day 31 anchor lands on 28, 29 or 30 as appropriate.
func monthReset(year int, month time.Month, dayOfMonth int, anchor ResetAnchor, loc *time.Location) time.Time {
	resolved := time.Date(year, month, dayOfMonth, anchor.Hour, anchor.Minute, 0, 0, loc)
	if resolved.Month() != month {
		last := daysInMonth(year, month)
		resolved = time.Date(year, month, last, anchor.Hour, anchor.Minute, 0, 0, loc)
	}
	return resolved
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func validResetWindow(start, reset, now time.Time) (ResetWindow, bool) {
	if !reset.After(start) || !reset.After(now) || now.Before(start) {
		return ResetWindow{}, false
	}
	return ResetWindow{Start: start, Reset: reset}, true
}
