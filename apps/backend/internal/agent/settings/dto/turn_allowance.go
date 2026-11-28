package dto

import (
	"encoding/json"
	"time"
)

type ProfileTurnAllowanceDTO struct {
	ProfileID          string          `json:"profile_id"`
	Provider           string          `json:"provider"`
	BindingScope       string          `json:"binding_scope"`
	Unit               string          `json:"unit"`
	ProviderSpendState string          `json:"provider_spend_state"`
	Owner              json.RawMessage `json:"owner"`
	Learned            json.RawMessage `json:"learned"`
	UpdatedAt          *time.Time      `json:"updated_at,omitempty"`
}
