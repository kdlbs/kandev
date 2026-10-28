package manifest

import (
	"strings"
	"testing"
)

func TestDestroyTimeoutSecondsValidatorTable(t *testing.T) {
	drain := "    requires_host_capabilities: [{name: drain_receipt, version: 1}]\n"
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"absent defaults", drain, ""},
		{"default value without the capability", "    destroy_timeout_seconds: 30\n", ""},
		{"upper bound", drain + "    destroy_timeout_seconds: 300\n", ""},
		{"below the minimum", drain + "    destroy_timeout_seconds: 29\n", "destroy_timeout_seconds must be between 30 and 300"},
		{"above the maximum", drain + "    destroy_timeout_seconds: 301\n", "destroy_timeout_seconds must be between 30 and 300"},
		{"over 30 needs drain_receipt", "    destroy_timeout_seconds: 60\n", "destroy_timeout_seconds over 30 requires the drain_receipt capability"},
		{"over 30 with another capability only",
			"    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n    destroy_timeout_seconds: 60\n",
			"destroy_timeout_seconds over 30 requires the drain_receipt capability"},
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
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "executor_providers[0]") {
				t.Fatalf("Validate() = %v, want error containing %q and the provider path", err, tc.want)
			}
		})
	}
}

func TestDestroyTimeoutSecondsDefaultsToThirtyForDeclaringProviders(t *testing.T) {
	drain := "    requires_host_capabilities: [{name: drain_receipt, version: 1}]\n"
	m := providerManifestWith(t, drain, "0.99.0")
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if got := m.ExecutorProviders[0].DestroyTimeoutSeconds; got != DefaultExecutorDestroyTimeoutSeconds {
		t.Fatalf("DestroyTimeoutSeconds = %d, want the default %d", got, DefaultExecutorDestroyTimeoutSeconds)
	}
}
