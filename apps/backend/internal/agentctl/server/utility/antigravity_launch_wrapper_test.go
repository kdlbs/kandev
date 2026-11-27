package utility

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestUtilityLaunchDropsAntigravitySkipPermissionsForEveryRuntime(t *testing.T) {
	for _, test := range []struct {
		name    string
		variant string
	}{
		{name: "qualified but unsupported CLI", variant: acpcompat.AntigravityRuntimeCLI},
		{name: "standalone runtime", variant: acpcompat.AntigravityRuntimeStandalone},
	} {
		t.Run(test.name, func(t *testing.T) {
			args, err := prepareACPLaunch(&InferenceConfigDTO{
				CLIFlags: []string{"--dangerously-skip-permissions", "--profile-flag"},
				AuthPolicy: &acpcompat.AuthPolicy{
					RuntimeVariant: test.variant,
				},
			}, acpcompat.AntigravityAgentID, "agy", []string{"agy", "acp"}, nil)
			if err != nil {
				t.Fatalf("prepareACPLaunch: %v", err)
			}
			if slices.Contains(args, "--dangerously-skip-permissions") || !slices.Contains(args, "--profile-flag") {
				t.Fatalf("utility launch args = %q, want skip flag omitted and unrelated flag retained", args)
			}
		})
	}
}
