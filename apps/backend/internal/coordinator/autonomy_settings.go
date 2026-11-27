package coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PATCH field names of the phase 3 autonomy settings and the field name their
// validation errors name.
const (
	PatchFieldAutonomyEnabled = "autonomy_enabled"
	PatchFieldCostCeilingUSD  = "cost_ceiling_usd"
	fieldCostCeiling          = "cost_ceiling"
)

const (
	subcentsPerUSD      = 10_000
	minCeilingSubcents  = 100
	maxCeilingSubcents  = 10_000 * subcentsPerUSD
	ceilingFractionBase = 100
)

var ceilingPattern = regexp.MustCompile(`^[0-9]{1,5}(\.[0-9]{1,2})?$`)

// ParseCostCeilingUSD converts a decimal USD string ("12", "0.01", "10000.00")
// to subcents with integer arithmetic. It accepts amounts from 0.01 to
// 10000.00; anything else is a *FieldError naming cost_ceiling.
func ParseCostCeilingUSD(raw string) (int64, error) {
	invalid := &FieldError{Field: fieldCostCeiling, Message: "cost_ceiling_usd must be a decimal amount from 0.01 to 10000.00"}
	if !ceilingPattern.MatchString(raw) {
		return 0, invalid
	}
	whole, frac, _ := strings.Cut(raw, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, invalid
	}
	subcents := w * subcentsPerUSD
	if frac != "" {
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, invalid
		}
		if len(frac) == 1 {
			f *= 10
		}
		subcents += f * (subcentsPerUSD / ceilingFractionBase)
	}
	if subcents < minCeilingSubcents || subcents > maxCeilingSubcents {
		return 0, invalid
	}
	return subcents, nil
}

// FormatCostCeilingUSD renders subcents as a decimal USD string with two places.
func FormatCostCeilingUSD(subcents int64) string {
	return fmt.Sprintf("%d.%02d", subcents/subcentsPerUSD, (subcents%subcentsPerUSD)/(subcentsPerUSD/ceilingFractionBase))
}

// applyAutonomyFields parses the two phase 3 keys of req into patch. A key
// absent from req leaves its field unchanged.
func applyAutonomyFields(req PatchCoordinatorRequest, patch *CoordinatorPatch) error {
	if raw, present := req[PatchFieldAutonomyEnabled]; present {
		var v any
		on, ok := false, false
		if err := json.Unmarshal(raw, &v); err == nil {
			on, ok = v.(bool)
		}
		if !ok {
			return &FieldError{Field: PatchFieldAutonomyEnabled, Message: "autonomy_enabled must be true or false"}
		}
		patch.AutonomyEnabled = &on
	}
	raw, present := req[PatchFieldCostCeilingUSD]
	if !present {
		return nil
	}
	patch.CeilingSet = true
	if bytes.Equal(raw, jsonNull) {
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return &FieldError{Field: fieldCostCeiling, Message: "cost_ceiling_usd must be a decimal string or null"}
	}
	subcents, err := ParseCostCeilingUSD(text)
	if err != nil {
		return err
	}
	patch.CostCeilingSubcents = &subcents
	return nil
}

// checkAutonomyInterlock rejects a resulting row that has autonomy on and no
// ceiling; it applies only to a PATCH that touched one of the two fields.
func checkAutonomyInterlock(patch CoordinatorPatch, merged *Coordinator) error {
	if patch.AutonomyEnabled == nil && !patch.CeilingSet {
		return nil
	}
	if merged.AutonomyEnabled && merged.CostCeilingSubcents == nil {
		return &FieldError{Field: fieldCostCeiling, Message: "autonomy requires a cost ceiling"}
	}
	return nil
}

var jsonNull = []byte(jsonNullLiteral)
