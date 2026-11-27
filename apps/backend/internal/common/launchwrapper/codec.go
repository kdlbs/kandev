package launchwrapper

import (
	"bytes"
	"encoding/json"
	"slices"
)

// IsZero distinguishes an omitted wrapper from an explicit envelope.
func (s Spec) IsZero() bool { return s.Version == 0 && s.Source == "" && len(s.Argv) == 0 }

// Profile copies tokens accepted by the profile authority boundary.
func Profile(tokens []string) Spec {
	if len(tokens) == 0 {
		return Spec{}
	}
	return Spec{Version: Version, Source: SourceProfile, Argv: slices.Clone(tokens)}
}

// UnmarshalJSON rejects invalid present envelopes, including null.
func (s *Spec) UnmarshalJSON(data []byte) error {
	type wire Spec
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return ErrInvalid
	}
	candidate := Spec(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}
