package routingerr

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

// resetHintPattern captures a provider retry time only when the notice also
// identifies its time zone. An unzoned wall-clock value is ambiguous across
// backend and provider locations, so it must not become a circuit deadline.
var resetHintPattern = regexp.MustCompile(
	`(?i)try again at\s+([A-Za-z]{3,9})[\s,]+(\d{1,2})(st|nd|rd|th)?(?:[\s,]+(\d{4}))?[\s,]+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*(AM|PM)?\s+(UTC|GMT|Z|[+-]\d{2}:?\d{2})(?:\s|[.,;!?]|$)`,
)

var resetClockHintPattern = regexp.MustCompile(
	`(?i)\bresets\s+(\d{1,2})(?::(\d{2}))?\s*(AM|PM)\s+\(([^()\r\n]+)\)`,
)

// resetAtClockHintPattern is Devin's reset suffix, e.g.
// "(at 12:01 UTC)". It names a clock and a zone but no date.
var resetAtClockHintPattern = regexp.MustCompile(
	`(?i)\(at\s+(\d{1,2}):(\d{2})\s*(AM|PM)?\s+(UTC|GMT|Z|[+-]\d{2}:?\d{2})\)`,
)

// resetRelativeHoursPattern and resetRelativeMinutesPattern capture the
// relative part of Devin's notice, e.g. "in 1 hour 12 minutes" or
// "in 8 minutes". They only disambiguate which day the clock suffix belongs
// to; the clock still supplies the exact instant.
var (
	resetRelativeHoursPattern = regexp.MustCompile(
		`(?i)\bin\s+(\d{1,4})\s*hours?\b(?:\s+(\d{1,4})\s*minutes?\b)?`,
	)
	resetRelativeMinutesPattern = regexp.MustCompile(
		`(?i)\bin\s+(\d{1,4})\s*minutes?\b`,
	)
)

// shortResetHintPattern is OpenCode's relative retry suffix, e.g.
// "retry shortly (after 24 s)". Only the diagnostic's own trailing suffix
// qualifies; the bounded capture keeps a malformed value out of the duration
// conversion.
var shortResetHintPattern = regexp.MustCompile(
	`(?im)\(after\s+([0-9]{1,4})\s*s\)\s*$`,
)

// minShortResetHintSeconds and maxShortResetHintSeconds bound a usable
// relative hint. Anything outside the range is ignored, not clamped, so a
// provider cannot keep a session parked past the first capacity slots.
const (
	minShortResetHintSeconds = 1
	maxShortResetHintSeconds = 60
)

var monthNames = [...]string{
	"january", "february", "march", "april", "may", "june",
	"july", "august", "september", "october", "november", "december",
}

var shortMonthNames = [...]string{
	"jan", "feb", "mar", "apr", "may", "jun",
	"jul", "aug", "sep", "oct", "nov", "dec",
}

type resetHintParts struct {
	month    time.Month
	day      int
	year     int
	hasYear  bool
	hour     int
	minute   int
	second   int
	location *time.Location
}

func parseResetHintAt(text string, now time.Time) *time.Time {
	parts, ok := parseResetHintParts(text)
	if ok {
		return resolveResetHintYear(parts, now)
	}
	return nil
}

// parseShortResetHintAt derives a bounded relative retry time from a
// diagnostic's trailing "(after N s)" suffix. Any other shape yields no hint,
// and an unusable hint never invalidates the failure's classification.
func parseShortResetHintAt(text string, now time.Time) *time.Time {
	match := shortResetHintPattern.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	seconds, ok := parseIntInRange(match[1], minShortResetHintSeconds, maxShortResetHintSeconds)
	if !ok {
		return nil
	}
	hint := now.Add(time.Duration(seconds) * time.Second)
	return &hint
}

// parseOpenCodeCapacityResetHintAt derives the relative hint from the same
// line that established the admission-refusal signature. Unrelated lines,
// channels, and quoted examples cannot contribute a suffix.
func parseOpenCodeCapacityResetHintAt(text string, now time.Time) *time.Time {
	for _, line := range strings.Split(text, "\n") {
		if !openCodeAdmissionRefusedLinePattern.MatchString(line) {
			continue
		}
		return parseShortResetHintAt(line, now)
	}
	return nil
}

func parseResetClockHintAt(text string, now time.Time) *time.Time {
	match := resetClockHintPattern.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	hour, ok := parseClockHour(match[1], match[3])
	if !ok {
		return nil
	}
	minute := 0
	if match[2] != "" {
		minute, ok = parseIntInRange(match[2], 0, 59)
		if !ok {
			return nil
		}
	}
	location, ok := parseIANAResetLocation(match[4])
	if !ok {
		return nil
	}
	return nextDailyResetAt(now, hour, minute, location)
}

// parseResetAtClockHintAt derives the next occurrence of Devin's explicit
// reset clock. The notice carries no date, so the clock is resolved near the
// estimated reset date in the stated zone. When the notice also states a
// relative duration, the occurrence closest to now+duration wins: a clock that
// has just elapsed by a few seconds must not roll a whole day.
func parseResetAtClockHintAt(text string, now time.Time) *time.Time {
	match := resetAtClockHintPattern.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	hour, ok := parseClockHour(match[1], match[3])
	if !ok {
		return nil
	}
	minute, ok := parseIntInRange(match[2], 0, 59)
	if !ok {
		return nil
	}
	location, ok := parseResetLocation(match[4])
	if !ok {
		return nil
	}
	duration, hasDuration := parseResetRelativeDuration(text)
	return resolveResetAtClock(now, hour, minute, location, duration, hasDuration)
}

// parseResetRelativeDuration reads the notice's "in N hours M minutes" part.
// Values outside the usable range yield no duration rather than a clamped one.
func parseResetRelativeDuration(text string) (time.Duration, bool) {
	if match := resetRelativeHoursPattern.FindStringSubmatch(text); match != nil {
		hours, ok := parseIntInRange(match[1], 0, 24)
		if !ok {
			return 0, false
		}
		minutes, ok := parseOptionalIntInRange(match[2], 0, 0, 59)
		if !ok {
			return 0, false
		}
		return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute, true
	}
	if match := resetRelativeMinutesPattern.FindStringSubmatch(text); match != nil {
		minutes, ok := parseIntInRange(match[1], 0, 59)
		if !ok {
			return 0, false
		}
		return time.Duration(minutes) * time.Minute, true
	}
	return 0, false
}

// resolveResetAtClock picks the clock occurrence consistent with the stated
// relative duration. Without a usable duration the first occurrence after now
// wins, which is the clock-only rule.
func resolveResetAtClock(
	now time.Time,
	hour, minute int,
	location *time.Location,
	duration time.Duration,
	hasDuration bool,
) *time.Time {
	if !hasDuration {
		return nextDailyResetAt(now, hour, minute, location)
	}
	target := now.Add(duration)
	localTarget := target.In(location)
	firstDate := time.Date(localTarget.Year(), localTarget.Month(), localTarget.Day(), 0, 0, 0, 0, time.UTC)
	var best time.Time
	var bestDelta time.Duration
	found := false
	for dayOffset := -1; dayOffset <= 1; dayOffset++ {
		for _, candidate := range resetClockWallTimeInstants(firstDate.AddDate(0, 0, dayOffset), hour, minute, location) {
			delta := candidate.Sub(target)
			if delta < 0 {
				delta = -delta
			}
			if !found || delta < bestDelta {
				best, bestDelta, found = candidate, delta, true
			}
		}
	}
	if found {
		return &best
	}
	return nextDailyResetAt(now, hour, minute, location)
}

// nextDailyResetAt returns the first instant at the given wall clock that is
// after now, within today or the next calendar day in location.
func nextDailyResetAt(now time.Time, hour, minute int, location *time.Location) *time.Time {
	localNow := now.In(location)
	firstDate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	for dayOffset := 0; dayOffset <= 1; dayOffset++ {
		date := firstDate.AddDate(0, 0, dayOffset)
		candidates := resetClockWallTimeInstants(date, hour, minute, location)
		if len(candidates) == 0 {
			// A gap later today is the selected reset and must remain unusable.
			// Once its wall clock has elapsed, the next calendar day can be valid.
			if dayOffset == 0 && localNow.Hour()*60+localNow.Minute() > hour*60+minute {
				continue
			}
			return nil
		}
		if len(candidates) == 1 {
			if candidates[0].After(now) {
				return &candidates[0]
			}
			continue
		}
		for _, candidate := range candidates {
			if candidate.After(now) {
				return nil
			}
		}
	}
	return nil
}

func resetClockWallTimeInstants(date time.Time, hour, minute int, location *time.Location) []time.Time {
	wall := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.UTC)
	unique := make(map[int64]struct{})
	var candidates []time.Time
	for delta := -48 * time.Hour; delta <= 48*time.Hour; delta += 15 * time.Minute {
		_, offset := wall.Add(delta).In(location).Zone()
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		localized := candidate.In(location)
		if localized.Year() != date.Year() || localized.Month() != date.Month() || localized.Day() != date.Day() ||
			localized.Hour() != hour || localized.Minute() != minute || localized.Second() != 0 {
			continue
		}
		key := candidate.UnixNano()
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })
	return candidates
}

func parseIANAResetLocation(raw string) (*time.Location, bool) {
	if strings.EqualFold(raw, "UTC") {
		return time.UTC, true
	}
	if raw == "" || strings.EqualFold(raw, "Local") || !strings.Contains(raw, "/") ||
		strings.Contains(raw, `\`) || path.IsAbs(raw) || path.Clean(raw) != raw {
		return nil, false
	}
	for _, component := range strings.Split(raw, "/") {
		if component == "" || strings.HasPrefix(component, ".") {
			return nil, false
		}
	}
	location, err := time.LoadLocation(raw)
	if err != nil {
		return nil, false
	}
	return location, true
}

func parseResetHintParts(text string) (resetHintParts, bool) {
	match := resetHintPattern.FindStringSubmatch(text)
	if match == nil {
		return resetHintParts{}, false
	}
	month, ok := monthFromName(match[1])
	if !ok {
		return resetHintParts{}, false
	}
	day, ok := parseIntInRange(match[2], 1, 31)
	if !ok || !validDayOrdinal(day, match[3]) {
		return resetHintParts{}, false
	}
	hour, ok := parseClockHour(match[5], match[8])
	if !ok {
		return resetHintParts{}, false
	}
	minute, ok := parseIntInRange(match[6], 0, 59)
	if !ok {
		return resetHintParts{}, false
	}
	second, ok := parseOptionalIntInRange(match[7], 0, 0, 59)
	if !ok {
		return resetHintParts{}, false
	}
	location, ok := parseResetLocation(match[9])
	if !ok {
		return resetHintParts{}, false
	}

	parts := resetHintParts{
		month:    month,
		day:      day,
		hour:     hour,
		minute:   minute,
		second:   second,
		location: location,
	}
	if match[4] != "" {
		parts.year, ok = parseIntInRange(match[4], 1970, 9999)
		if !ok {
			return resetHintParts{}, false
		}
		parts.hasYear = true
	}
	return parts, true
}

func resolveResetHintYear(parts resetHintParts, now time.Time) *time.Time {
	if parts.hasYear {
		parsed, ok := makeResetTime(parts.year, parts.month, parts.day, parts.hour, parts.minute, parts.second, parts.location)
		if !ok {
			return nil
		}
		return &parsed
	}
	localNow := now.In(parts.location)
	for year := localNow.Year(); year <= localNow.Year()+8 && year <= 9999; year++ {
		parsed, ok := makeResetTime(year, parts.month, parts.day, parts.hour, parts.minute, parts.second, parts.location)
		if ok && parsed.After(now) {
			return &parsed
		}
	}
	return nil
}

func validDayOrdinal(day int, suffix string) bool {
	if suffix == "" {
		return true
	}
	expected := "th"
	if day%100 < 11 || day%100 > 13 {
		switch day % 10 {
		case 1:
			expected = "st"
		case 2:
			expected = "nd"
		case 3:
			expected = "rd"
		}
	}
	return strings.EqualFold(suffix, expected)
}

func monthFromName(name string) (time.Month, bool) {
	name = strings.ToLower(name)
	for i := range monthNames {
		if name == monthNames[i] || name == shortMonthNames[i] {
			return time.Month(i + 1), true
		}
	}
	return 0, false
}

func parseClockHour(raw, meridiem string) (int, bool) {
	if meridiem == "" {
		return parseIntInRange(raw, 0, 23)
	}
	hour, ok := parseIntInRange(raw, 1, 12)
	if !ok {
		return 0, false
	}
	if strings.EqualFold(meridiem, "AM") {
		if hour == 12 {
			return 0, true
		}
		return hour, true
	}
	if strings.EqualFold(meridiem, "PM") {
		if hour < 12 {
			return hour + 12, true
		}
		return hour, true
	}
	return 0, false
}

func parseResetLocation(raw string) (*time.Location, bool) {
	switch strings.ToUpper(raw) {
	case "UTC", "GMT", "Z":
		return time.UTC, true
	}
	if len(raw) != 6 && len(raw) != 5 {
		return nil, false
	}
	if raw[0] != '+' && raw[0] != '-' {
		return nil, false
	}
	offset := strings.ReplaceAll(raw[1:], ":", "")
	hours, ok := parseIntInRange(offset[:2], 0, 14)
	if !ok {
		return nil, false
	}
	minutes, ok := parseIntInRange(offset[2:], 0, 59)
	if !ok || (hours == 14 && minutes != 0) {
		return nil, false
	}
	seconds := hours*60*60 + minutes*60
	if raw[0] == '-' {
		seconds = -seconds
	}
	return time.FixedZone(raw, seconds), true
}

func makeResetTime(year int, month time.Month, day, hour, minute, second int, location *time.Location) (time.Time, bool) {
	parsed := time.Date(year, month, day, hour, minute, second, 0, location)
	return parsed, parsed.Year() == year && parsed.Month() == month && parsed.Day() == day &&
		parsed.Hour() == hour && parsed.Minute() == minute && parsed.Second() == second
}

func parseIntInRange(raw string, min, max int) (int, bool) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}

func parseOptionalIntInRange(raw string, fallback, min, max int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	return parseIntInRange(raw, min, max)
}
