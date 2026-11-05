package manifest

import (
	"strings"
	"testing"
)

func TestWorkspacePreparationValidatorTable(t *testing.T) {
	both := "    requires_host_capabilities: [{name: repository_intent, version: 1}, {name: workspace_descriptor, version: 1}]\n"
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"absent is core", "", ""},
		{"core without capabilities", "    workspace_preparation: core\n", ""},
		{"provider with both capabilities", both + "    workspace_preparation: provider\n", ""},
		{"provider without capabilities", "    workspace_preparation: provider\n", "workspace_preparation provider requires the repository_intent and workspace_descriptor capabilities"},
		{"provider with intent only", "    requires_host_capabilities: [{name: repository_intent, version: 1}]\n    workspace_preparation: provider\n", "workspace_preparation provider requires the repository_intent and workspace_descriptor capabilities"},
		{"provider with descriptor only", "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n    workspace_preparation: provider\n", "workspace_preparation provider requires the repository_intent and workspace_descriptor capabilities"},
		{"unknown value", both + "    workspace_preparation: remote\n", "workspace_preparation must be core or provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := providerManifestWith(t, tc.yaml, "0.99.0").Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate(): %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWorkspacePreparationDefaultsToCore(t *testing.T) {
	m := providerManifestWith(t, "", "0.99.0")
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if got := m.ExecutorProviders[0].WorkspacePreparation; got != WorkspacePreparationCore {
		t.Fatalf("WorkspacePreparation = %q, want %q", got, WorkspacePreparationCore)
	}
}
