package sessioncapacity

import (
	"encoding/json"
	"fmt"
)

func decodeGradeLimits(raw json.RawMessage) (map[string]int, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("grade_limits must be an object: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("grade_limits must be an object")
	}
	limits := make(map[string]int, len(fields))
	for grade, rawMaximum := range fields {
		if isJSONNull(rawMaximum) {
			return nil, fmt.Errorf("grade maximum must be an integer")
		}
		var maximum int
		if err := json.Unmarshal(rawMaximum, &maximum); err != nil {
			return nil, fmt.Errorf("grade maximum must be an integer: %w", err)
		}
		limits[grade] = maximum
	}
	return limits, nil
}

func storedGradeLimits(raw []byte) (map[string]int, bool, error) {
	var fields struct {
		GradeLimits json.RawMessage `json:"grade_limits"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false, err
	}
	if len(fields.GradeLimits) == 0 {
		return nil, false, nil
	}
	limits, err := decodeGradeLimits(fields.GradeLimits)
	return limits, true, err
}
