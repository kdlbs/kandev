package sandbox

import (
	"bytes"
	"encoding/json"
)

// UnmarshalJSON refuses restrictions that this host cannot represent.
func (policy *PolicyV2) UnmarshalJSON(raw []byte) error {
	type payload PolicyV2
	var decoded payload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*policy = PolicyV2(decoded)
	return nil
}
