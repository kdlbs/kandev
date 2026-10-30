package mcpconfig

import (
	"bytes"
	"encoding/json"
	"io"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

type BuiltinPackageChoices map[mcpprofile.PackageID]string

// UnmarshalJSON rejects repeated package IDs instead of accepting the last
// value. A policy must have one unambiguous value for each package.
func (choices *BuiltinPackageChoices) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*choices = nil
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ErrBuiltinPolicyInvalid
	}
	result := make(BuiltinPackageChoices)
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return ErrBuiltinPolicyInvalid
		}
		id := mcpprofile.PackageID(key)
		if _, duplicate := result[id]; duplicate {
			return ErrBuiltinPolicyInvalid
		}
		var value string
		if decoder.Decode(&value) != nil {
			return ErrBuiltinPolicyInvalid
		}
		result[id] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return ErrBuiltinPolicyInvalid
	}
	if _, err = decoder.Token(); err != io.EOF {
		return ErrBuiltinPolicyInvalid
	}
	*choices = result
	return nil
}
