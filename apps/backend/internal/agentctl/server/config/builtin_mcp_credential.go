package config

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// BuiltinMCPAuthorizationHeader carries the instance-scoped built-in MCP
// caller credential on every built-in server entry.
const BuiltinMCPAuthorizationHeader = "Authorization"

const builtinMCPBearerPrefix = "Bearer "

// builtinMCPCredentialBytes is the entropy of one caller credential (256 bits).
const builtinMCPCredentialBytes = 32

// NewBuiltinMCPCredential returns a fresh random caller credential for one
// agentctl instance. It is never derived from the control credential.
func NewBuiltinMCPCredential() (string, error) {
	raw := make([]byte, builtinMCPCredentialBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate built-in MCP credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// BuiltinMCPHeaders returns the header set every built-in entry carries for
// credential. An empty credential yields no headers, which is the shape an
// older agentctl produces.
func BuiltinMCPHeaders(credential string) map[string]string {
	if credential == "" {
		return nil
	}
	return map[string]string{BuiltinMCPAuthorizationHeader: builtinMCPBearerPrefix + credential}
}

// BuiltinMCPHeadersMatch reports whether headers are exactly the header set
// BuiltinMCPHeaders produces for credential: no extra, missing, or foreign
// values.
func BuiltinMCPHeadersMatch(headers map[string]string, credential string) bool {
	want := BuiltinMCPHeaders(credential)
	if len(headers) != len(want) {
		return false
	}
	if len(want) == 0 {
		return true
	}
	got, ok := headers[BuiltinMCPAuthorizationHeader]
	return ok && subtle.ConstantTimeCompare([]byte(got), []byte(want[BuiltinMCPAuthorizationHeader])) == 1
}

func applyBuiltinMCPAuth(cfg *InstanceConfig, overrides *InstanceOverrides) {
	if overrides == nil {
		return
	}
	cfg.BuiltinMCPCredential = overrides.BuiltinMCPCredential
	cfg.BuiltinMCPAuthEnforced = overrides.BuiltinMCPAuthEnforced && overrides.BuiltinMCPCredential != ""
}
