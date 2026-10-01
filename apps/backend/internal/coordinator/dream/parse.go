// Package dream runs the coordinator's Shadow dream: a bounded episode that
// reviews recorded turns and reports suggested changes, with no tool that
// writes and no output any turn reads.
package dream

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// Item kinds a report may carry.
const (
	KindNoteAdd             = "note_add"
	KindNoteUpdate          = "note_update"
	KindNoteRetire          = "note_retire"
	KindContextDiff         = "context_diff"
	KindStandingOrderAdd    = "standing_order_add"
	KindStandingOrderRetire = "standing_order_retire"
	MaxItems                = 10
	MaxConsidered           = 20
	MaxItemTextRunes        = 500
	MinCitedTurns           = 2
	maxFenceOverhead        = 16
)

var validKinds = map[string]bool{
	KindNoteAdd: true, KindNoteUpdate: true, KindNoteRetire: true,
	KindContextDiff: true, KindStandingOrderAdd: true, KindStandingOrderRetire: true,
}

// ErrBadOutput is returned by ParseAnswer for any answer that is not one valid
// document within the limits.
var ErrBadOutput = errors.New("dream: bad output")

// Item is one suggestion as the agent wrote it.
type Item struct {
	Kind         string   `json:"kind"`
	Text         string   `json:"text"`
	TargetID     string   `json:"target_id"`
	CitedTurnIDs []string `json:"cited_turn_ids"`
}

// Answer is the agent's whole answer.
type Answer struct {
	Items      []Item   `json:"items"`
	Considered []string `json:"considered"`
}

// ParseAnswer parses the last agent message once. One surrounding ```json
// fence is stripped; any other prose, an unknown key at any level, more than
// MaxItems items, more than MaxConsidered considered entries or an unknown
// kind is ErrBadOutput.
func ParseAnswer(raw string) (Answer, error) {
	body := stripFence(strings.TrimSpace(raw))
	dec := json.NewDecoder(bytes.NewReader([]byte(body)))
	dec.DisallowUnknownFields()
	var a Answer
	if err := dec.Decode(&a); err != nil {
		return Answer{}, ErrBadOutput
	}
	if dec.More() || dec.InputOffset() != int64(len(body)) {
		return Answer{}, ErrBadOutput
	}
	if len(a.Items) > MaxItems || len(a.Considered) > MaxConsidered {
		return Answer{}, ErrBadOutput
	}
	for _, it := range a.Items {
		if !validKinds[it.Kind] {
			return Answer{}, ErrBadOutput
		}
	}
	if a.Items == nil {
		a.Items = []Item{}
	}
	if a.Considered == nil {
		a.Considered = []string{}
	}
	return a, nil
}

func stripFence(s string) string {
	if len(s) < 6 || !strings.HasPrefix(s, "```") || !strings.HasSuffix(s, "```") {
		return s
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(s, "```"), "```")
	inner = strings.TrimPrefix(inner, "json")
	return strings.TrimSpace(inner)
}
