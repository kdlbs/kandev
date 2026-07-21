// Package optional provides JSON field types that distinguish "absent" from
// "explicitly null" in PATCH-style request payloads — a distinction a plain
// `*int` cannot make, because both decode to a nil pointer.
package optional

import "encoding/json"

// Int is a tri-state integer field for partial-update request bodies:
//
//   - key absent      → Present == false            (leave the target unchanged)
//   - key present null → Present == true, Value == nil  (clear to "unset")
//   - key present N    → Present == true, Value == &N   (set to N)
//
// Declare it as a plain (non-pointer) struct field WITHOUT `omitempty`:
//
//	MaxInflightTasks optional.Int `json:"maxInflightTasks"`
//
// encoding/json only invokes UnmarshalJSON when the key is present in the
// payload, so Present reliably reports whether the client sent the field.
type Int struct {
	Present bool
	Value   *int
}

// UnmarshalJSON records that the key was present and decodes null vs. number.
func (o *Int) UnmarshalJSON(data []byte) error {
	o.Present = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}
