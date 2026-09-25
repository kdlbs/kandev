package models

import (
	"bytes"
	"encoding/json"
)

// CanonicalJSON returns a stable serialization of raw: object keys in sorted
// order, insignificant whitespace removed, and number literals preserved exactly
// as written.
//
// Preserving the literals is the point. The obvious implementation — unmarshal
// into interface{} and re-marshal — turns every number into a float64 and then
// back into JSON, so a Unix-second timestamp round-trips as 1.7576064e+09. Any
// comparison built on that would stop matching the stored bytes, which for a
// compare-and-set means a writer that can never win. json.Decoder.UseNumber
// keeps the literal, and json.Number re-marshals as itself.
//
// Empty, whitespace-only and JSON-null input all canonicalize to nil, so an
// absent value and an explicit null are one value.
func CanonicalJSON(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// CanonicalJSONOf canonicalizes a Go value by marshalling it first. It is the
// producer-side counterpart of CanonicalJSON: both sides of a comparison must be
// canonicalized through this package so key order and whitespace cannot decide it.
func CanonicalJSONOf(value interface{}) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return CanonicalJSON(raw)
}
