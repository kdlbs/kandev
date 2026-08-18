package cron

import (
	"fmt"
	"strings"
	"time"

	robfigcron "github.com/robfig/cron/v3"
)

var scheduleParser = robfigcron.NewParser(
	robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow | robfigcron.Descriptor,
)

// NextScheduleTime returns the first occurrence strictly after after. Empty
// timezones mean UTC. Wall-clock schedules use the supplied IANA timezone.
func NextScheduleTime(expression, timezone string, after time.Time) (time.Time, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return time.Time{}, fmt.Errorf("empty cron expression")
	}
	if !strings.HasPrefix(expression, "@every") && !hasTimezonePrefix(expression) {
		if timezone == "" {
			timezone = "UTC"
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return time.Time{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
		expression = "CRON_TZ=" + timezone + " " + expression
	}
	schedule, err := scheduleParser.Parse(expression)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cron expression: %w", err)
	}
	return schedule.Next(after), nil
}

func hasTimezonePrefix(expression string) bool {
	return strings.HasPrefix(expression, "TZ=") || strings.HasPrefix(expression, "CRON_TZ=")
}
