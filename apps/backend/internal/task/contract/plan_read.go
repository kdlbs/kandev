package contract

import (
	"encoding/json"
	"fmt"
)

const (
	DefaultPlanReadCharacters = 4096
	MaxPlanReadCharacters     = 8192
	MaxPlanReadOffset         = int64(9007199254740991)
)

// PlanReadOptions preserves the presence of range fields: offset zero enables
// a partial read, while omitted range fields retain the full-read contract.
type PlanReadOptions struct {
	Offset          *int64  `json:"offset,omitempty"`
	Limit           *int64  `json:"limit,omitempty"`
	ExpectedVersion *string `json:"expected_version,omitempty"`
}

type PlanReadValidationError struct {
	Message string
}

func (e *PlanReadValidationError) Error() string { return e.Message }

func (o PlanReadOptions) Validate() error {
	if o.Offset != nil && (*o.Offset < 0 || *o.Offset > MaxPlanReadOffset) {
		return planReadFieldError("offset", fmt.Sprintf("an integer between 0 and %d", MaxPlanReadOffset))
	}
	if o.Limit != nil && (*o.Limit < 1 || *o.Limit > MaxPlanReadCharacters) {
		return planReadFieldError("limit", "an integer between 1 and 8192")
	}
	if o.ExpectedVersion != nil && *o.ExpectedVersion == "" {
		return planReadFieldError("expected_version", "a non-empty string")
	}
	return nil
}

func planReadFieldError(field, expected string) error {
	return &PlanReadValidationError{Message: field + " must be " + expected}
}

// ParsePlanReadOptions rejects malformed optional arguments rather than
// interpreting them as absent and silently returning the entire document.
func ParsePlanReadOptions(data []byte) (PlanReadOptions, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return PlanReadOptions{}, planReadFieldError("arguments", "an object")
	}
	var options PlanReadOptions
	for field, target := range map[string]**int64{"offset": &options.Offset, "limit": &options.Limit} {
		if raw, present := fields[field]; present {
			var value int64
			if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
				return options, planReadFieldError(field, "an integer")
			}
			*target = &value
		}
	}
	if raw, present := fields["expected_version"]; present {
		var version string
		if string(raw) == "null" || json.Unmarshal(raw, &version) != nil {
			return options, planReadFieldError("expected_version", "a non-empty string")
		}
		options.ExpectedVersion = &version
	}
	return options, options.Validate()
}
