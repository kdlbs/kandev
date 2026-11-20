package automation

import (
	"encoding/json"
	"strings"
)

const jsonNullLiteral = "null"

// scheduledConfigIdentity is the bounded identity of a saved cron schedule.
// It deliberately ignores JSON formatting, member order, and unknown members.
type scheduledConfigIdentity struct {
	expression string
	timezone   string
	scheduled  bool
}

// decodeScheduledTriggerConfig decodes a persisted scheduled config. Missing
// and null values are the empty schedule rather than a decoding failure.
func decodeScheduledTriggerConfig(raw json.RawMessage) (ScheduledTriggerConfig, error) {
	var cfg ScheduledTriggerConfig
	if len(raw) == 0 || string(raw) == jsonNullLiteral {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ScheduledTriggerConfig{}, err
	}
	return cfg, nil
}

// normalizeScheduledConfig applies the definition comparison rules from the
// schedule replacement design. It does not prove arbitrary cron equivalence
// and does not canonicalize timezone aliases.
func normalizeScheduledConfig(raw json.RawMessage) (scheduledConfigIdentity, error) {
	cfg, err := decodeScheduledTriggerConfig(raw)
	if err != nil {
		return scheduledConfigIdentity{}, err
	}
	expression := strings.TrimSpace(cfg.CronExpression)
	if expression == "" {
		return scheduledConfigIdentity{}, nil
	}
	timezone := cfg.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	// The parser owns inline timezone precedence, so a separate timezone field
	// cannot change the schedule of these expressions.
	if strings.HasPrefix(expression, "@every") || hasCronTZPrefix(expression) {
		timezone = ""
	}
	return scheduledConfigIdentity{
		expression: expression,
		timezone:   timezone,
		scheduled:  true,
	}, nil
}

// scheduledDefinitionChanged reports whether a supplied config replaces the
// current saved schedule definition. Any unreadable definition is a
// replacement so a repaired row starts from a fresh boundary.
func scheduledDefinitionChanged(current, supplied json.RawMessage) bool {
	currentIdentity, currentErr := normalizeScheduledConfig(current)
	suppliedIdentity, suppliedErr := normalizeScheduledConfig(supplied)
	if currentErr != nil || suppliedErr != nil {
		return true
	}
	return currentIdentity != suppliedIdentity
}
