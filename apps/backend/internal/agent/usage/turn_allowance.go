package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const KandevTurnUnit = "kandev_turn"

var ErrTurnAllowanceUnknown = errors.New("configured turn allowance binding is unknown")
var ErrTurnAllowanceInvalid = errors.New("invalid turn allowance")

// TurnAllowance configures native turns, not provider HTTP requests or spend.
type TurnAllowance struct {
	ConcurrentTurns *int64     `json:"concurrent_turns,omitempty"`
	TurnsPerWindow  *int64     `json:"turns_per_window,omitempty"`
	WindowSeconds   int64      `json:"window_seconds,omitempty"`
	ResetAnchor     *time.Time `json:"reset_anchor,omitempty"`
	BlockUntil      *time.Time `json:"block_until,omitempty"`
}

type TurnAllowanceRecord struct {
	AccountKey string          `json:"-" db:"account_key"`
	Provider   string          `json:"provider" db:"provider"`
	Owner      json.RawMessage `json:"owner" db:"config_json"`
	Learned    json.RawMessage `json:"learned" db:"learned_json"`
	UpdatedAt  time.Time       `json:"updated_at" db:"updated_at"`
}

type TurnAllowanceLimited struct {
	Dimension string
	RetryAt   time.Time
}

func (e *TurnAllowanceLimited) Error() string {
	return "shared Kandev turn allowance reached: " + e.Dimension
}

func DecodeTurnAllowance(raw json.RawMessage) (TurnAllowance, error) {
	var policy TurnAllowance
	if len(raw) == 0 {
		return policy, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return policy, ErrTurnAllowanceInvalid
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return policy, ErrTurnAllowanceInvalid
	}
	if policy.ConcurrentTurns != nil && *policy.ConcurrentTurns < 0 || policy.TurnsPerWindow != nil && *policy.TurnsPerWindow < 0 {
		return policy, ErrTurnAllowanceInvalid
	}
	if policy.WindowSeconds < 0 || policy.WindowSeconds > (1<<63-1)/int64(time.Second) {
		return policy, ErrTurnAllowanceInvalid
	}
	if policy.TurnsPerWindow != nil && *policy.TurnsPerWindow > 0 && policy.WindowSeconds == 0 {
		return policy, ErrTurnAllowanceInvalid
	}
	return policy, nil
}

// PatchTurnAllowance preserves stored unknown fields and never edits learned JSON.
func PatchTurnAllowance(prior, patch json.RawMessage) (json.RawMessage, error) {
	owner, edits := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if len(prior) != 0 && (json.Unmarshal(prior, &owner) != nil || owner == nil) {
		return nil, ErrTurnAllowanceInvalid
	}
	if json.Unmarshal(patch, &edits) != nil || edits == nil {
		return nil, ErrTurnAllowanceInvalid
	}
	for key, value := range edits {
		switch key {
		case "concurrent_turns", "turns_per_window", "window_seconds", "reset_anchor", "block_until":
			if string(value) == "null" {
				delete(owner, key)
			} else {
				owner[key] = value
			}
		default:
			return nil, fmt.Errorf("%w: unsupported field", ErrTurnAllowanceInvalid)
		}
	}
	result, err := json.Marshal(owner)
	if err != nil {
		return nil, err
	}
	_, err = DecodeTurnAllowance(result)
	return result, err
}

func (p TurnAllowance) Configured(now time.Time) bool {
	return p.ConcurrentTurns != nil && *p.ConcurrentTurns > 0 || p.TurnsPerWindow != nil && *p.TurnsPerWindow > 0 || p.BlockUntil != nil && now.Before(*p.BlockUntil)
}

// TurnWindow is a fixed interval anchored by an explicit instant or the UTC epoch.
func (p TurnAllowance) TurnWindow(now time.Time) (time.Time, time.Time) {
	anchor := int64(0)
	if p.ResetAnchor != nil {
		anchor = p.ResetAnchor.Unix()
	}
	span := p.WindowSeconds
	if span <= 0 {
		return time.Time{}, time.Time{}
	}
	delta := now.Unix() - anchor
	period := delta / span
	if delta < 0 && delta%span != 0 {
		period--
	}
	start := time.Unix(anchor+period*span, 0).UTC()
	return start, start.Add(time.Duration(span) * time.Second)
}
