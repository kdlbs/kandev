package manifest

import (
	"strings"
	"testing"
)

// A durable bootstrap republishes the environment on recovery from the
// provider's recorded workspace descriptor and repository set, which only a
// provider-prepared workspace has. Core preparation would fail every recovery,
// so the manifest is refused up front.
func TestDurableBootstrapRequiresProviderWorkspacePreparation(t *testing.T) {
	both := "    requires_host_capabilities: [{name: repository_intent, version: 1}, {name: workspace_descriptor, version: 1}, {name: durable_bootstrap, version: 1}]\n"
	coreOnly := "    requires_host_capabilities: [{name: durable_bootstrap, version: 1}]\n"
	want := "durable_bootstrap requires workspace_preparation provider"
	cases := []struct {
		name, yaml, want string
	}{
		{"default core", coreOnly, want},
		{"explicit core", coreOnly + "    workspace_preparation: core\n", want},
		{"core with descriptor", "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}, {name: durable_bootstrap, version: 1}]\n", want},
		{"provider preparation", both + "    workspace_preparation: provider\n", ""},
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
