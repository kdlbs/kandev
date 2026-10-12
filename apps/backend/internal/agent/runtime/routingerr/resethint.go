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

// parseResetHint extracts an explicitly zoned, dated provider retry time from
// free text. Clock-only reset notices use their provider-specific parser.
func parseResetHint(text string) *time.Time {
	return parseResetHintAt(text, time.Now())
}

func parseResetHintAt(text string, now time.Time) *time.Time {
	parts, ok := parseResetHintParts(text)
	if ok {
		return resolveResetHintYear(parts, now)
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
