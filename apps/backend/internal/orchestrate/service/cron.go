package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// nextCronTick computes the next fire time after `after` for the given cron expression.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week.
// Timezone is optional; defaults to UTC.
func nextCronTick(expression, timezone string, after time.Time) (time.Time, error) {
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

type cronSpec struct {
	minutes     []int
	hours       []int
	daysOfMonth []int
	months      []int
	daysOfWeek  []int
}

func parseCronSpec(fields []string) (*cronSpec, error) {
	minutes, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hours, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	months, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}
	return &cronSpec{minutes, hours, dom, months, dow}, nil
}

// parseCronField parses a single cron field (supports *, N, N-M, */N, N-M/N, comma lists).
func parseCronField(field string, min, max int) ([]int, error) {
	var result []int
	for _, part := range strings.Split(field, ",") {
		vals, err := parseCronPart(part, min, max)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}
	return dedupSort(result), nil
}

func parseCronPart(part string, min, max int) ([]int, error) {
	step, base, err := extractStep(part)
	if err != nil {
		return nil, err
	}
	lo, hi, err := parseRange(base, min, max)
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
