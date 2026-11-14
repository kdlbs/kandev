package routingerr

import (
	"regexp"
	"strings"
	"time"
)

const openCodePeriodUsageRule = "opencode.stderr.usage_limit.v1"

// openCodePeriodResetRules are the OpenCode quota rules whose line may carry a
// relative "resets in" deadline: the period limits and the free usage limit,
// which is classified by its own unlabelled and prefixed rules.
var openCodePeriodResetRules = map[string]bool{
	openCodePeriodUsageRule:                    true,
	"opencode.stderr.free_usage_unlabelled.v1": true,
	"opencode.stderr.free_usage_prefixed.v1":   true,
}

var openCodeRelativeResetPattern = regexp.MustCompile(`(?i)\bresets?\s+in\s+([^\r\n.!?;]+)`)
var openCodeRelativeUnitPattern = regexp.MustCompile(`(?i)(\d{1,4})\s*(days?|hours?|hrs?|minutes?|mins?|min|seconds?|secs?)\b`)
var openCodeRelativeGluedSuffix = regexp.MustCompile(`^[.!?;]+[^\s.!?;]`)

// openCodePeriodResetAt accepts relative timing only on a line that itself
// matches the classified OpenCode period-limit rule. Multiple different
// deadlines, unrecognized units, and missing observation time stay unknown.
func openCodePeriodResetAt(text string, observedAt time.Time) *time.Time {
	if observedAt.IsZero() {
		return nil
	}
	var found *time.Time
	matched := false
	for _, line := range strings.Split(text, "\n") {
		if !matchesOpenCodePeriodRule(line) {
			continue
		}
		if matched {
			return nil
		}
		matched = true
		matches := openCodeRelativeResetPattern.FindAllStringSubmatchIndex(line, -1)
		if len(matches) == 0 {
			continue
		}
		if len(matches) != 1 || openCodeRelativeGluedSuffix.MatchString(line[matches[0][1]:]) {
			return nil
		}
		duration, ok := parseOpenCodeRelativeDuration(line[matches[0][2]:matches[0][3]])
		if !ok {
			return nil
		}
		reset := observedAt.Add(duration)
		found = &reset
	}
	return found
}

func matchesOpenCodePeriodRule(line string) bool {
	for _, rule := range providerRules["opencode-acp"] {
		if openCodePeriodResetRules[rule.id] && rule.pattern.MatchString(line) {
			return true
		}
	}
	return false
}

func parseOpenCodeRelativeDuration(text string) (time.Duration, bool) {
	text = strings.TrimSpace(text)
	units := openCodeRelativeUnitPattern.FindAllStringSubmatchIndex(text, -1)
	if len(units) == 0 {
		return 0, false
	}
	var total time.Duration
	position := 0
	for _, unit := range units {
		separator := strings.ToLower(strings.Join(strings.Fields(text[position:unit[0]]), " "))
		if !openCodeRelativeSeparatorValid(separator, position == 0) {
			return 0, false
		}
		value, ok := parseIntInRange(text[unit[2]:unit[3]], 0, 999)
		if !ok {
			return 0, false
		}
		name := strings.ToLower(text[unit[4]:unit[5]])
		part := time.Duration(value)
		switch {
		case strings.HasPrefix(name, "d"):
			part *= 24 * time.Hour
		case strings.HasPrefix(name, "h"):
			part *= time.Hour
		case strings.HasPrefix(name, "s"):
			part *= time.Second
		default:
			part *= time.Minute
		}
		if part > time.Duration(1<<63-1)-total {
			return 0, false
		}
		total += part
		position = unit[1]
	}
	if total <= 0 || strings.TrimSpace(text[position:]) != "" {
		return 0, false
	}
	return total, true
}

func openCodeRelativeSeparatorValid(separator string, initial bool) bool {
	if initial {
		return separator == ""
	}
	switch separator {
	case "", ",", "and", ", and":
		return true
	default:
		return false
	}
}
