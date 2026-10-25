package orchestrator

import (
	"encoding/json"
	"math"
)

const (
	goalKey                   = "goal"
	goalObjectiveMaxSize      = 16 * 1024
	goalClearWatermarkInfoKey = "_kandev_goal_clear_watermark"
)

func mergeACPGoalMetaWithClearWatermark(
	existing, incoming map[string]any,
	attachmentChanged bool,
	existingClearWatermark any,
) (map[string]any, map[string]any) {
	merged := mergeACPGoalMeta(existing, incoming, attachmentChanged)
	if attachmentChanged {
		return merged, nil
	}

	watermark := parseACPGoalWatermark(existingClearWatermark)
	incomingGoal, incomingHasGoal := incoming[goalKey]
	if !incomingHasGoal {
		return merged, watermark
	}
	if incomingGoal == nil {
		if existingGoal, ok := parseACPGoal(existing[goalKey]); ok {
			return merged, goalWatermark(existingGoal)
		}
		return merged, watermark
	}
	parsed, ok := parseACPGoal(incomingGoal)
	if !ok {
		if existingGoal, existingOK := parseACPGoal(existing[goalKey]); existingOK {
			return merged, goalWatermark(existingGoal)
		}
		return merged, watermark
	}
	if watermark != nil && !goalIsAtLeastAsFresh(parsed, watermark) {
		merged[goalKey] = nil
		return merged, watermark
	}
	return merged, nil
}

func goalWatermark(goal map[string]any) map[string]any {
	return map[string]any{
		"createdAt": goal["createdAt"],
		"updatedAt": goal["updatedAt"],
	}
}

func parseACPGoalWatermark(value any) map[string]any {
	record, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	createdAt, createdOK := goalNumber(record["createdAt"])
	updatedAt, updatedOK := goalNumber(record["updatedAt"])
	if !createdOK || !updatedOK {
		return nil
	}
	return map[string]any{"createdAt": createdAt, "updatedAt": updatedAt}
}

var supportedGoalStatuses = map[string]struct{}{
	"active":   {},
	"paused":   {},
	"blocked":  {},
	"limited":  {},
	"complete": {},
}

// mergeACPGoalMeta retains the typed goal extension while preserving the
// existing replacement semantics for every other provider metadata key.
func mergeACPGoalMeta(
	existing, incoming map[string]any,
	attachmentChanged bool,
) map[string]any {
	merged := cloneACPMetadata(incoming)
	existingGoal, existingHasGoal := existing[goalKey]
	incomingGoal, incomingHasGoal := incoming[goalKey]

	if attachmentChanged {
		if parsed, ok := parseACPGoal(incomingGoal); incomingHasGoal && ok {
			merged[goalKey] = parsed
		} else {
			merged[goalKey] = nil
		}
		return merged
	}

	if !incomingHasGoal {
		if existingHasGoal {
			merged[goalKey] = existingGoal
		}
		return merged
	}
	if incomingGoal == nil {
		merged[goalKey] = nil
		return merged
	}

	parsed, ok := parseACPGoal(incomingGoal)
	if !ok {
		// A present but malformed replacement is an explicit non-active
		// projection. Retaining an older active goal would misrepresent the
		// provider after it has changed or invalidated the goal payload.
		merged[goalKey] = nil
		return merged
	}

	if existingParsed, existingOK := parseACPGoal(existingGoal); existingOK && !goalIsAtLeastAsFresh(parsed, existingParsed) {
		merged[goalKey] = existingParsed
		return merged
	}
	merged[goalKey] = parsed
	return merged
}

func cloneACPMetadata(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func parseACPGoal(value any) (map[string]any, bool) {
	record, ok := goalRecord(value)
	if !ok {
		return nil, false
	}
	objective, status, createdAt, updatedAt, ok := requiredGoalFields(record)
	if !ok {
		return nil, false
	}

	parsed := map[string]any{
		"objective": objective,
		"status":    status,
		"createdAt": createdAt,
		"updatedAt": updatedAt,
	}
	if !optionalGoalNumbers(record, parsed) {
		return nil, false
	}
	if !optionalGoalControlMethod(record, parsed) {
		return nil, false
	}
	return parsed, true
}

func goalRecord(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	record, ok := value.(map[string]any)
	return record, ok && record != nil
}

func requiredGoalFields(record map[string]any) (string, string, float64, float64, bool) {
	objective, objectiveOK := record["objective"].(string)
	if !objectiveOK || objective == "" || len(objective) > goalObjectiveMaxSize {
		return "", "", 0, 0, false
	}
	status, statusOK := record["status"].(string)
	if !statusOK {
		return "", "", 0, 0, false
	}
	if _, supported := supportedGoalStatuses[status]; !supported {
		return "", "", 0, 0, false
	}
	createdAt, createdOK := goalNumber(record["createdAt"])
	updatedAt, updatedOK := goalNumber(record["updatedAt"])
	if !createdOK || !updatedOK {
		return "", "", 0, 0, false
	}
	return objective, status, createdAt, updatedAt, true
}

func optionalGoalNumbers(record, parsed map[string]any) bool {
	for _, key := range []string{"tokenBudget", "tokensUsed", "timeUsedSeconds"} {
		raw, present := record[key]
		if !present || raw == nil {
			continue
		}
		number, valid := goalNumber(raw)
		if !valid {
			return false
		}
		parsed[key] = number
	}
	return true
}

func optionalGoalControlMethod(record, parsed map[string]any) bool {
	controlMethod, present := record["controlMethod"]
	if !present || controlMethod == nil {
		return true
	}
	method, valid := controlMethod.(string)
	if !valid {
		return false
	}
	parsed["controlMethod"] = method
	return true
}

func goalNumber(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	return number, number >= 0 && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func goalIsAtLeastAsFresh(incoming, existing map[string]any) bool {
	incomingCreated, incomingCreatedOK := goalNumber(incoming["createdAt"])
	existingCreated, existingCreatedOK := goalNumber(existing["createdAt"])
	incomingUpdated, incomingUpdatedOK := goalNumber(incoming["updatedAt"])
	existingUpdated, existingUpdatedOK := goalNumber(existing["updatedAt"])
	if !incomingCreatedOK || !existingCreatedOK || !incomingUpdatedOK || !existingUpdatedOK {
		return false
	}
	if incomingCreated != existingCreated {
		return incomingCreated > existingCreated
	}
	return incomingUpdated >= existingUpdated
}
