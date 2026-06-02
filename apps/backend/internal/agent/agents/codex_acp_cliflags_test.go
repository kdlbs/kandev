package agents

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestCodexACP_CLIFlagTokenise(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "approval_policy",
			raw:  "-c approval_policy=never",
			want: []string{"-c", "approval_policy=never"},
		},
		{
			name: "sandbox_permissions",
			raw:  CodexACPSandboxDiskFullReadCLIFlag,
			want: []string{"-c", `sandbox_permissions=["disk-full-read-access"]`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := cliflags.Tokenise(tc.raw)
			if err != nil {
				t.Fatalf("Tokenise: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tokens = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCodexACP_ResolveSeededCLIFlags_DefaultDisabled(t *testing.T) {
	flags := make([]models.CLIFlag, 0, len(codexACPPermSettings))
	for _, s := range codexACPPermSettings {
		flagText := s.CLIFlag
		if s.CLIFlagValue != "" {
			flagText = s.CLIFlag + " " + s.CLIFlagValue
		}
		flags = append(flags, models.CLIFlag{Flag: flagText, Enabled: s.Default})
	}
	got, err := cliflags.Resolve(flags)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no argv when defaults disabled, got %#v", got)
	}
}
