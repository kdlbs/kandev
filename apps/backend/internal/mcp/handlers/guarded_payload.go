package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// guardedPayload is a JSON object that keeps its keys in document order and
// keeps duplicates, so the guard sees exactly the keys the handler's decoder
// will see.
type guardedPayload struct {
	entries []guardedEntry
}

type guardedEntry struct {
	key   string
	value json.RawMessage
}

var errGuardedPayloadShape = errors.New("payload is not a JSON object")

// parseGuardedPayload decodes a payload into ordered entries. An empty or
// null payload is an empty object; any other non-object is refused.
func parseGuardedPayload(raw json.RawMessage) (*guardedPayload, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return &guardedPayload{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errGuardedPayloadShape
	}
	payload := &guardedPayload{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errGuardedPayloadShape
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		payload.entries = append(payload.entries, guardedEntry{key: key, value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errGuardedPayloadShape
	}
	return payload, nil
}

// canonicalize renames every key that folds to a guarded field name to the
// canonical spelling. Two keys folding to one guarded field (including two
// identical spellings) report ok=false.
func (p *guardedPayload) canonicalize(guarded []string) bool {
	for _, name := range guarded {
		seen := false
		for i := range p.entries {
			if !strings.EqualFold(p.entries[i].key, name) {
				continue
			}
			if seen {
				return false
			}
			seen = true
			p.entries[i].key = name
		}
	}
	return true
}

// get returns the value of a canonical key.
func (p *guardedPayload) get(name string) (json.RawMessage, bool) {
	for _, e := range p.entries {
		if e.key == name {
			return e.value, true
		}
	}
	return nil, false
}

// set replaces the value of a canonical key or appends it.
func (p *guardedPayload) set(name string, value any) {
	encoded, _ := json.Marshal(value)
	for i := range p.entries {
		if p.entries[i].key == name {
			p.entries[i].value = encoded
			return
		}
	}
	p.entries = append(p.entries, guardedEntry{key: name, value: encoded})
}

func (p *guardedPayload) remove(name string) {
	kept := p.entries[:0]
	for _, e := range p.entries {
		if e.key != name {
			kept = append(kept, e)
		}
	}
	p.entries = kept
}

func (p *guardedPayload) marshal() json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, e := range p.entries {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(e.key)
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(e.value)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// stringField reads a string-typed field. present is false for a missing or
// null field; wellTyped is false when the value is not a string.
func (p *guardedPayload) stringField(name string) (value string, present, wellTyped bool) {
	raw, ok := p.get(name)
	if !ok || isJSONNull(raw) {
		return "", false, true
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, false
	}
	return value, true, true
}

// stringListField reads an array-of-strings field; a null element or any
// other element type is ill-typed.
func (p *guardedPayload) stringListField(name string) (values []string, wellTyped bool) {
	raw, ok := p.get(name)
	if !ok || isJSONNull(raw) {
		return nil, true
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false
	}
	for _, item := range items {
		var s string
		if isJSONNull(item) || json.Unmarshal(item, &s) != nil {
			return nil, false
		}
		values = append(values, s)
	}
	return values, true
}
