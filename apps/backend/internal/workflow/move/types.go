// Package move contains the transport-neutral contract for a one-shot
// workflow-step move. It deliberately has no task, persistence, or
// orchestrator dependencies so every move entry point can share the same
// normalization and validation rules.
package move

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// EntryOptions are one-shot options applied when a task enters a workflow
// step. They do not modify the target step's durable configuration.
//
// The pointer to EntryOptions is the optional nested entry_options value on a
// move request. NormalizeEntryOptions returns nil when the value is empty.
type EntryOptions struct {
	ResetContext bool   `json:"reset_context,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	// SkipStepPrompt suppresses the destination step's configured prompt (and
	// its task-description fallback) for this one entry. With instructions, the
	// agent auto-starts a turn carrying only those instructions; without
	// instructions, no turn starts and the task lands idle.
	SkipStepPrompt bool `json:"skip_step_prompt,omitempty"`
}

// UnmarshalJSON keeps all request transports fail-closed when a caller sends
// a misspelled or otherwise unsupported option. Without a typed decoder,
// encoding/json silently drops unknown fields and the move would be accepted
// while applying a different contract than the caller requested.
func (o *EntryOptions) UnmarshalJSON(data []byte) error {
	type entryOptions EntryOptions
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded entryOptions
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	*o = EntryOptions(decoded)
	return nil
}

// StepEntryOptions is the explicit workflow-step vocabulary for the nested
// entry_options value. It is an alias so transports do not need duplicate
// shapes while callers can use the name that matches their boundary.
type StepEntryOptions = EntryOptions

// Options is a short vocabulary alias for callers that refer to the nested
// value as move options.
type Options = EntryOptions

// MoveChange describes whether a move changes the workflow step or only its
// position metadata. Entry options are meaningful only for Step changes.
type MoveChange string

const (
	MoveChangeNone         MoveChange = "none"
	MoveChangePositionOnly MoveChange = "position_only"
	MoveChangeStep         MoveChange = "step"
)

// HasOverrides reports whether options contain at least one effective value.
// Whitespace-only strings are treated as absent, matching normalization.
func HasOverrides(options *EntryOptions) bool {
	return normalizedCopy(options) != nil
}

// HasOverrides reports whether this value contains at least one effective
// entry option.
func (o *EntryOptions) HasOverrides() bool {
	return HasOverrides(o)
}

// DecodeEntryOptionsJSON decodes a persisted entry payload strictly. Unknown
// fields and trailing JSON are rejected so corrupted or newer payloads cannot
// be silently treated as an option-less move.
func DecodeEntryOptionsJSON(encoded []byte) (*EntryOptions, error) {
	trimmed := bytes.TrimSpace(encoded)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("workflow move entry options must be a JSON object")
	}
	for _, field := range []string{"reset_context", "instructions", "skip_step_prompt"} {
		if raw, ok := fields[field]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("workflow move entry option %s cannot be null", field)
		}
	}
	var options EntryOptions
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	if !options.HasOverrides() {
		return nil, nil
	}
	return &options, nil
}

// EncodeEntryOptionsJSON encodes one-shot move options for the pending marker.
// It round-trips with DecodeEntryOptionsJSON: option-less input encodes to the
// empty object, which decode treats as an ordinary move.
func EncodeEntryOptionsJSON(options *EntryOptions) (json.RawMessage, error) {
	normalized := normalizedCopy(options)
	if normalized == nil {
		return json.RawMessage("{}"), nil
	}
	return json.Marshal(normalized)
}

func normalizedCopy(options *EntryOptions) *EntryOptions {
	if options == nil {
		return nil
	}
	copy := *options
	copy.Instructions = strings.TrimSpace(copy.Instructions)
	if copy == (EntryOptions{}) {
		return nil
	}
	return &copy
}
