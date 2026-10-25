package manifest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sandboxProviderManifest = `
id: example.sandbox
api_version: 2
version: 1.0.0
display_name: Example sandbox
description: Test sandbox provider
author: kandev
categories: [tools]
runtime:
  type: binary
  executables: {linux-amd64: server/plugin}
capabilities:
  sandbox_provider: true
sandbox_providers:
  - key: confinement
    display_name: Confinement
    protocol_major: 1
`

// @covers AC-PLUGINS-SANDBOX-001.1
func TestSandboxProviderManifestRequiresExplicitSupportedCapability(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement string }{
		{"missing permission", "sandbox_provider: true", "sandbox_provider: false"},
		{"unknown protocol", "protocol_major: 1", "protocol_major: 2"},
		{"missing protocol", "protocol_major: 1", ""},
		{"unsafe key", "key: confinement", "key: ../confinement"},
		{"HTML label", "display_name: Confinement", "display_name: <b>Confinement</b>"},
		{"duplicate", "    protocol_major: 1", "    protocol_major: 1\n  - key: confinement\n    display_name: Duplicate\n    protocol_major: 1"},
		{"unknown descriptor", "    protocol_major: 1", "    protocol_major: 1\n    required_method: RunShell"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := Parse([]byte(strings.Replace(sandboxProviderManifest, tc.old, tc.replacement, 1)))
			if err == nil {
				err = parsed.Validate()
			}
			require.Error(t, err)
		})
	}
}

func TestSandboxProviderManifestRetainsDeclaration(t *testing.T) {
	parsed, err := Parse([]byte(sandboxProviderManifest))
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
	encoded, err := json.Marshal(parsed)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.JSONEq(t, `[{"key":"confinement","display_name":"Confinement","protocol_major":1}]`, string(fields["sandbox_providers"]))
}

func TestSandboxProviderManifestClonePreservesDeclaration(t *testing.T) {
	parsed, err := Parse([]byte(sandboxProviderManifest))
	require.NoError(t, err)
	clone := parsed.Clone()
	clone.SandboxProviders[0].ProtocolMajor = 2
	require.Equal(t, 1, parsed.SandboxProviders[0].ProtocolMajor)
}

func TestSandboxProviderManifestRequiresManagedRuntimeAndDeclaration(t *testing.T) {
	parsed, err := Parse([]byte(sandboxProviderManifest))
	require.NoError(t, err)
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.SandboxProviders = nil },
		func(m *Manifest) { m.Runtime = Runtime{} },
		func(m *Manifest) { m.APIVersion = 1 },
		func(m *Manifest) { m.SandboxProviders = make([]SandboxProvider, 17) },
	} {
		copy := parsed.Clone()
		mutate(&copy)
		require.Error(t, copy.Validate())
	}
}
