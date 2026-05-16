package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NextCronTime computes the next fire time after `after` for the given cron expression.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week.
// Timezone is optional; defaults to UTC.
func NextCronTime(expression, timezone string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected 5 cron fields, got %d", len(fields))
	}
	loc := time.UTC
	if timezone != "" {
		var err error
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
	}
	spec, err := parseCronSpec(fields)
	if err != nil {
		return time.Time{}, err
	}
	return findNextMatch(spec, after.In(loc)), nil
}

// ParseCronExpression validates a cron expression string and returns an error
// if it is malformed.
func ParseCronExpression(expression string) error {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return fmt.Errorf("expected 5 cron fields, got %d", len(fields))
	}
	_, err := parseCronSpec(fields)
	return err
}

type cronSpec struct {
	minutes     []int
	hours       []int
	daysOfMonth []int
	months      []int
	daysOfWeek  []int
}

// dayOfWeekNames maps abbreviated day names to their numeric cron value.
// Case-insensitive; lookup is performed after lowercasing the token.
var dayOfWeekNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// monthNames maps abbreviated month names to their numeric cron value.
// Case-insensitive; lookup is performed after lowercasing the token.
var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

func parseCronSpec(fields []string) (*cronSpec, error) {
	minutes, err := parseCronField(fields[0], 0, 59, nil)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hours, err := parseCronField(fields[1], 0, 23, nil)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseCronField(fields[2], 1, 31, nil)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	months, err := parseCronField(fields[3], 1, 12, monthNames)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseCronField(fields[4], 0, 6, dayOfWeekNames)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}
	return &cronSpec{minutes, hours, dom, months, dow}, nil
}

// parseCronField parses a single cron field (supports *, N, N-M, */N, N-M/N, comma lists).
// If names is non-nil, alphabetic tokens are resolved to numbers via the map
// before numeric parsing (e.g. MON->1, JAN->1). Lookup is case-insensitive.
func parseCronField(field string, min, max int, names map[string]int) ([]int, error) {
	var result []int
	for _, part := range strings.Split(field, ",") {
		vals, err := parseCronPart(part, min, max, names)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}
	return dedupSort(result), nil
}

func parseCronPart(part string, min, max int, names map[string]int) ([]int, error) {
	step, base, err := extractStep(part)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveNames(base, names)
	if err != nil {
		return nil, err
	}
	lo, hi, err := parseRange(resolved, min, max)
	if err != nil {
		return nil, err
	}
	if lo < min || hi > max || lo > hi {
		return nil, fmt.Errorf("value out of range [%d-%d] in %q", min, max, part)
	}
	var vals []int
	for i := lo; i <= hi; i += step {
		vals = append(vals, i)
	}
	return vals, nil
}

// resolveNames translates any alphabetic tokens in a cron part (e.g. "MON",
// "mon-fri") into their numeric equivalents using the provided name map.
// Wildcards are passed through. If names is nil and the part contains
// non-numeric tokens, an error is returned by the downstream numeric parser.
func resolveNames(part string, names map[string]int) (string, error) {
	if part == "*" || !containsAlpha(part) {
		return part, nil
	}
	if names == nil {
		return "", fmt.Errorf("alphabetic name %q not allowed in this field", part)
	}
	if idx := strings.Index(part, "-"); idx >= 0 {
		lo, err := resolveToken(part[:idx], names)
		if err != nil {
			return "", err
		}
		hi, err := resolveToken(part[idx+1:], names)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d-%d", lo, hi), nil
	}
	val, err := resolveToken(part, names)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(val), nil
}

func resolveToken(tok string, names map[string]int) (int, error) {
	if !containsAlpha(tok) {
		v, err := strconv.Atoi(tok)
		if err != nil {
			return 0, fmt.Errorf("invalid value %q", tok)
		}
		return v, nil
	}
	v, ok := names[strings.ToLower(tok)]
	if !ok {
		return 0, fmt.Errorf("unknown name %q", tok)
	}
	return v, nil
}

func containsAlpha(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func extractStep(part string) (int, string, error) {
	idx := strings.Index(part, "/")
	if idx < 0 {
		return 1, part, nil
	}
	step, err := strconv.Atoi(part[idx+1:])
	if err != nil || step <= 0 {
		return 0, "", fmt.Errorf("invalid step in %q", part)
	}
	return step, part[:idx], nil
}

func parseRange(part string, min, max int) (int, int, error) {
	if part == "*" {
		return min, max, nil
	}
	if idx := strings.Index(part, "-"); idx >= 0 {
		lo, err := strconv.Atoi(part[:idx])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range in %q", part)
		}
		hi, err := strconv.Atoi(part[idx+1:])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range in %q", part)
		}
		return lo, hi, nil
	}
	val, err := strconv.Atoi(part)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid value %q", part)
	}
	return val, val, nil
}

func dedupSort(vals []int) []int {
	seen := make(map[int]bool, len(vals))
	var result []int
	for _, v := range vals {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	// Simple insertion sort for small slices.
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j] < result[j-1]; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result
}

func findNextMatch(spec *cronSpec, after time.Time) time.Time {
	// Start from the next minute after `after`.
	t := after.Truncate(time.Minute).Add(time.Minute)
	// Search up to 366 days ahead.
	deadline := after.Add(366 * 24 * time.Hour)
	for t.Before(deadline) {
		if matchesSpec(spec, t) {
			return t.UTC()
		}
		t = t.Add(time.Minute)
	}
	// Fallback: return 24h from now.
	return after.Add(24 * time.Hour).UTC()
}

func matchesSpec(spec *cronSpec, t time.Time) bool {
	return contains(spec.minutes, t.Minute()) &&
		contains(spec.hours, t.Hour()) &&
		contains(spec.daysOfMonth, t.Day()) &&
		contains(spec.months, int(t.Month())) &&
		contains(spec.daysOfWeek, int(t.Weekday()))
}

func contains(vals []int, v int) bool {
	for _, val := range vals {
		if val == v {
			return true
		}
	}
	return false
}
