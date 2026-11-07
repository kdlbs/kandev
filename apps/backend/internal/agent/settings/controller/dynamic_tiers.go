package controller

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func (c *Controller) SetDynamicProfileTiersEnabled(enabled bool) {
	c.dynamicProfileTiersEnabled = enabled
}

type storedTierRow struct {
	raw  map[string]json.RawMessage
	head string
	tier json.RawMessage
}

func tierObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%w: selection must be a JSON object", ErrDynamicProfileRule)
	}
	return object, nil
}

func joinedTier(object map[string]json.RawMessage) bool {
	var joined bool
	_ = json.Unmarshal(object["join_previous"], &joined)
	return joined
}

func storedTierRows(routes []models.DynamicAgentRoute) (map[string]storedTierRow, error) {
	rows := make(map[string]storedTierRow, len(routes))
	head := ""
	var tier json.RawMessage
	for _, route := range routes {
		policy, err := decodeDynamicPolicyDocument(route.RulesJSON, route.Position)
		if err != nil {
			return nil, err
		}
		object, err := tierObject(policy.Selection)
		if err != nil {
			return nil, err
		}
		if object == nil || !joinedTier(object) {
			head, tier = route.ExecutionProfileID, object["tier"]
		}
		rows[route.ExecutionProfileID] = storedTierRow{raw: object, head: head, tier: tier}
	}
	return rows, nil
}

// mergeTierObjects merges only explicitly supplied fields. Unknown model and
// usage metadata remains data; editing adjacency does not reset it.
func mergeTierObjects(base, patch map[string]json.RawMessage) map[string]json.RawMessage {
	merged := make(map[string]json.RawMessage, len(base)+len(patch))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range patch {
		left, leftErr := tierObject(merged[key])
		right, rightErr := tierObject(value)
		if leftErr == nil && rightErr == nil && left != nil && right != nil {
			merged[key], _ = json.Marshal(mergeTierObjects(left, right))
		} else {
			merged[key] = value
		}
	}
	return merged
}

func sameTierObject(left, right map[string]json.RawMessage) bool {
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	var lv, rv any
	_ = json.Unmarshal(l, &lv)
	_ = json.Unmarshal(r, &rv)
	return reflect.DeepEqual(lv, rv)
}

func (c *Controller) prepareTierSelections(routes []models.DynamicAgentRoute, request *dto.DynamicAgentProfileDTO) error {
	if request == nil {
		return nil
	}
	stored, err := storedTierRows(routes)
	if err != nil {
		return err
	}
	previousHead := ""
	for index := range request.Candidates {
		candidate := &request.Candidates[index]
		if err := normalizeDynamicCandidatePolicy(candidate); err != nil {
			return err
		}
		row := stored[candidate.ExecutionProfileID]
		patch, err := tierObject(candidate.Policies.Selection)
		if err != nil {
			return err
		}
		if !c.dynamicProfileTiersEnabled && patch != nil && !sameTierObject(row.raw, patch) {
			return fmt.Errorf("%w: dynamic profile tiers are disabled", ErrDynamicProfileRule)
		}
		var joinOverride *bool
		if patch == nil || !c.dynamicProfileTiersEnabled {
			if row.raw == nil {
				candidate.Policies.Selection = nil
				previousHead = row.head
				continue
			}
			joined := index > 0 && row.head != "" && row.head == previousHead
			joinOverride = &joined
		}
		previousHead = row.head
		candidate.Policies.Selection = mergeTierRow(row, patch, joinOverride)
	}
	return nil
}

func mergeTierRow(row storedTierRow, patch map[string]json.RawMessage, joinOverride *bool) json.RawMessage {
	merged := mergeTierObjects(row.raw, patch)
	if len(merged) == 0 {
		return nil
	}
	if joinOverride != nil {
		merged["join_previous"], _ = json.Marshal(*joinOverride)
	}
	_, explicit := patch["tier"]
	if !explicit || joinOverride != nil {
		if joinedTier(merged) {
			delete(merged, "tier")
		} else {
			merged["tier"] = row.tier
			if len(merged["tier"]) == 0 {
				merged["tier"] = json.RawMessage(`{"mode":"order","on_failure":"same_tier_next"}`)
			}
		}
	}
	encoded, _ := json.Marshal(merged)
	return encoded
}

func validateTierSelection(raw json.RawMessage, position int) error {
	object, err := tierObject(raw)
	if err != nil || object == nil {
		return err
	}
	if value, present := object["join_previous"]; present {
		var joined bool
		if err := json.Unmarshal(value, &joined); err != nil {
			return fmt.Errorf("%w: join_previous must be boolean", ErrDynamicProfileRule)
		}
	}
	if joinedTier(object) && position == 0 {
		return fmt.Errorf("%w: first candidate cannot join", ErrDynamicProfileRule)
	}
	if joinedTier(object) && len(object["tier"]) > 0 && string(object["tier"]) != "null" {
		return fmt.Errorf("%w: only tier heads own a tier policy", ErrDynamicProfileRule)
	}
	return nil
}
