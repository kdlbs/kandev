package coordinator

import (
	"bytes"
	"encoding/json"
	"math"
)

// Note mutation operations.
const (
	noteOpCreate = "create"
	noteOpEdit   = "edit"
	noteOpRevert = "revert"
	noteOpDelete = "delete"
)

// noteRequest is a decoded note mutation body (envelope and field-type checks).
type noteRequest struct {
	op      string
	typ     string
	text    string
	cites   []NoteCitation
	version int
	target  int
}

var noteFields = map[string][]string{
	noteOpCreate: {fieldType, fieldText, fieldCitations},
	noteOpEdit:   {fieldType, fieldText, fieldCitations, fieldVersion},
	noteOpRevert: {fieldTargetVersion, fieldVersion},
	noteOpDelete: {fieldVersion},
}

// parseNoteRequest decodes body for op. It reports an unknown, missing or
// null-where-required field as invalid_request and a wrong JSON type as
// invalid_field; content shape rules are checked later.
func parseNoteRequest(op string, body []byte, tooLarge bool) (*noteRequest, *NoteError) {
	if tooLarge {
		e := noteErr(NoteInvalidRequest, "")
		e.TooLarge = true
		return nil, e
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(body), &obj) != nil || obj == nil {
		return nil, noteErr(NoteInvalidRequest, "")
	}
	allowed := map[string]bool{}
	for _, f := range noteFields[op] {
		allowed[f] = true
	}
	for k := range obj {
		if !allowed[k] {
			return nil, noteErr(NoteInvalidRequest, k)
		}
	}
	req := &noteRequest{op: op}
	if nerr := req.decodeContent(obj); nerr != nil {
		return nil, nerr
	}
	if nerr := req.decodeVersions(obj); nerr != nil {
		return nil, nerr
	}
	return req, nil
}

func (r *noteRequest) decodeContent(obj map[string]json.RawMessage) *NoteError {
	if r.op != noteOpCreate && r.op != noteOpEdit {
		return nil
	}
	for _, f := range []struct {
		name string
		dst  *string
	}{{fieldType, &r.typ}, {fieldText, &r.text}} {
		raw, ok := obj[f.name]
		if !ok || isJSONNull(raw) {
			return noteErr(NoteInvalidRequest, f.name)
		}
		if json.Unmarshal(raw, f.dst) != nil {
			return noteErr(NoteInvalidField, f.name)
		}
	}
	raw, present := obj[fieldCitations]
	if !present && r.op == noteOpEdit {
		return noteErr(NoteInvalidRequest, fieldCitations)
	}
	if present {
		cites, nerr := parseCitations(raw)
		if nerr != nil {
			return nerr
		}
		r.cites = cites
	}
	return nil
}

func (r *noteRequest) decodeVersions(obj map[string]json.RawMessage) *NoteError {
	if r.op == noteOpCreate {
		return nil
	}
	raw, ok := obj[fieldVersion]
	if !ok || isJSONNull(raw) {
		return noteErr(NoteInvalidRequest, fieldVersion)
	}
	v, ok := jsonInt(raw)
	if !ok || (r.op != noteOpDelete && v < 1) {
		return noteErr(NoteInvalidField, fieldVersion)
	}
	r.version = v
	if r.op != noteOpRevert {
		return nil
	}
	raw, ok = obj[fieldTargetVersion]
	if !ok || isJSONNull(raw) {
		return noteErr(NoteInvalidRequest, fieldTargetVersion)
	}
	t, ok := jsonInt(raw)
	if !ok || t < 1 {
		return noteErr(NoteInvalidField, fieldTargetVersion)
	}
	r.target = t
	return nil
}

func isJSONNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// jsonInt decodes a JSON number that is exactly an integer.
func jsonInt(raw json.RawMessage) (int, bool) {
	var f float64
	if bytes.ContainsAny(raw, ".eE") || json.Unmarshal(raw, &f) != nil || f != math.Trunc(f) || math.Abs(f) > 1<<31 {
		return 0, false
	}
	return int(f), true
}
