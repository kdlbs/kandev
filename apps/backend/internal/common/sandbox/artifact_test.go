package sandbox

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-SANDBOX-001.3
func TestSandboxArtifactBindsPrivateValuesAsJSONStrings(t *testing.T) {
	const source = `{"callback":{"token":""},"unchanged":9007199254740993}`
	secret := "private\"\n\\value"
	bindings := []ArtifactBinding{{Path: []string{"callback", "token"}, Reference: "private.token"}}
	content, err := RenderJSONArtifact(source, bindings, map[string]string{"private.token": secret})
	require.NoError(t, err)
	var parsed struct {
		Callback  struct{ Token string }
		Unchanged json.Number
	}
	require.NoError(t, json.Unmarshal(content, &parsed))
	require.Equal(t, secret, parsed.Callback.Token)
	require.Equal(t, "9007199254740993", parsed.Unchanged.String())
}

func TestSandboxArtifactRejectsAmbiguousOrUnboundInsertions(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		bindings      []ArtifactBinding
		values        map[string]string
	}{
		{"missing reference", `{"token":""}`, []ArtifactBinding{{[]string{"token"}, "secret"}}, nil},
		{"missing field", `{}`, []ArtifactBinding{{[]string{"token"}, "secret"}}, map[string]string{"secret": "value"}},
		{"replace authority", `{"enabled":true}`, []ArtifactBinding{{[]string{"enabled"}, "secret"}}, map[string]string{"secret": "value"}},
		{"replace literal", `{"token":"existing"}`, []ArtifactBinding{{[]string{"token"}, "secret"}}, map[string]string{"secret": "value"}},
		{"duplicate binding", `{"token":""}`, []ArtifactBinding{{[]string{"token"}, "secret"}, {[]string{"token"}, "secret"}}, map[string]string{"secret": "value"}},
		{"empty path", `{}`, []ArtifactBinding{{nil, "secret"}}, map[string]string{"secret": "value"}},
		{"trailing document", `{} {}`, nil, nil},
		{"oversized content", strings.Repeat(" ", 1<<20) + `{}`, nil, nil},
		{"oversized expansion", `{"token":""}`, []ArtifactBinding{{[]string{"token"}, "secret"}}, map[string]string{"secret": strings.Repeat("x", 1<<20)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, err := RenderJSONArtifact(tc.content, tc.bindings, tc.values)
			require.ErrorIs(t, err, ErrInvalidPolicy)
			require.Empty(t, content)
		})
	}
}
