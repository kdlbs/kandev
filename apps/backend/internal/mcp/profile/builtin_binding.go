package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
)

const BuiltinBindingMetadataKey = "kandev.builtin_mcp_binding"
const BuiltinPackagesCapability = "builtin-mcp-packages-v1"

type builtinBinding struct {
	Profile Context `json:"profile"`
	Digest  string  `json:"digest"`
}

// MarshalBuiltinBinding seals canonical runtime state for integrity checks.
// Only the trusted launch builder may supply this metadata; the digest is not
// an authentication credential.
func MarshalBuiltinBinding(ctx Context) (string, error) {
	canonical, digest, err := canonicalBuiltinBinding(ctx)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(builtinBinding{Profile: canonical, Digest: digest})
	return string(encoded), err
}

func canonicalBuiltinBinding(ctx Context) (Context, string, error) {
	if ctx.BuiltinPackages == nil || ctx.BuiltinPackages.Validate() != nil || ctx.Surface != ctx.BuiltinPackages.Surface || ctx.ManagedToolPolicy != nil || ctx.CoordinatorToolPolicy != nil {
		return Context{}, "", errors.New("invalid built-in MCP binding")
	}
	canonical := Normalize(ctx)
	slices.Sort(canonical.BuiltinPackages.Packages)
	slices.Sort(canonical.Capabilities)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return Context{}, "", err
	}
	digest := sha256.Sum256(encoded)
	return canonical, hex.EncodeToString(digest[:]), nil
}

// ParseBuiltinBindingMetadata accepts persisted JSON or its database round trip.
// Explicit missing, malformed, or altered binding data never means legacy mode.
func ParseBuiltinBindingMetadata(value any) (*Context, error) {
	var encoded []byte
	switch v := value.(type) {
	case string:
		encoded = []byte(v)
	case map[string]any:
		var err error
		encoded, err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("missing built-in MCP binding")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var binding builtinBinding
	if err := decoder.Decode(&binding); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("invalid built-in MCP binding suffix")
	}
	canonical, digest, err := canonicalBuiltinBinding(binding.Profile)
	if err != nil {
		return nil, err
	}
	if binding.Digest != digest {
		return nil, errors.New("built-in MCP binding digest mismatch")
	}
	return &canonical, nil
}
