package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
)

func TestAuthMethodFields(t *testing.T) {
	desc := "with description"
	cases := []struct {
		name        string
		in          acp.AuthMethod
		wantID      string
		wantName    string
		wantDesc    *string
		wantMetaKey string
	}{
		{
			name: "Agent variant",
			in: acp.AuthMethod{Agent: &acp.AuthMethodAgent{
				Id:          "agent-id",
				Name:        "Agent",
				Description: &desc,
				Meta:        map[string]any{"k": "agent"},
			}},
			wantID: "agent-id", wantName: "Agent", wantDesc: &desc, wantMetaKey: "agent",
		},
		{
			name: "Terminal variant",
			in: acp.AuthMethod{Terminal: &acp.AuthMethodTerminalInline{
				Id:   "term-id",
				Name: "Terminal",
				Meta: map[string]any{"k": "terminal"},
			}},
			wantID: "term-id", wantName: "Terminal", wantMetaKey: "terminal",
		},
		{
			name: "EnvVar variant",
			in: acp.AuthMethod{EnvVar: &acp.AuthMethodEnvVarInline{
				Id:   "env-id",
				Name: "EnvVar",
				Meta: map[string]any{"k": "envvar"},
			}},
			wantID: "env-id", wantName: "EnvVar", wantMetaKey: "envvar",
		},
		{
			name: "zero-value (no variant set)",
			in:   acp.AuthMethod{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, name, desc, meta := AuthMethodFields(tc.in)
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if (desc == nil) != (tc.wantDesc == nil) {
				t.Errorf("desc nil mismatch: got=%v want=%v", desc, tc.wantDesc)
			}
			if desc != nil && tc.wantDesc != nil && *desc != *tc.wantDesc {
				t.Errorf("desc = %q, want %q", *desc, *tc.wantDesc)
			}
			if tc.wantMetaKey == "" {
				if meta != nil {
					t.Errorf("meta = %v, want nil", meta)
				}
				return
			}
			if got, _ := meta["k"].(string); got != tc.wantMetaKey {
				t.Errorf("meta[k] = %q, want %q", got, tc.wantMetaKey)
			}
		})
	}
}
