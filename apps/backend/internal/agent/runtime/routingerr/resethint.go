package routingerr

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// resetHintPattern captures a provider retry time only when the notice also
// identifies its time zone. An unzoned wall-clock value is ambiguous across
// backend and provider locations, so it must not become a circuit deadline.
var resetHintPattern = regexp.MustCompile(
	`(?i)try again at\s+([A-Za-z]{3,9})[\s,]+(\d{1,2})(st|nd|rd|th)?(?:[\s,]+(\d{4}))?[\s,]+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*(AM|PM)?\s+(UTC|GMT|Z|[+-]\d{2}:?\d{2})(?:\s|[.,;!?]|$)`,
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

// parseResetHint extracts an explicitly zoned provider retry time from free
// text. It returns nil when the notice has no supported timezone or the
// timestamp is malformed.
func parseResetHint(text string) *time.Time {
	return parseResetHintAt(text, time.Now())
}

func parseResetHintAt(text string, now time.Time) *time.Time {
	parts, ok := parseResetHintParts(text)
	if !ok {
		return nil
	}
	return resolveResetHintYear(parts, now)
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
