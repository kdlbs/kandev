package manifest

import (
	"strings"
	"testing"
)

func providerManifestWith(t *testing.T, extra string, minVersion string) *Manifest {
	t.Helper()
	text := validExecutorProviderManifest(`handle: {type: string}`)
	text = strings.Replace(text, "    contract_version: 1\n", "    contract_version: 1\n"+extra, 1)
	if minVersion != "" {
		text = strings.Replace(text, "api_version: 2\n", "api_version: 2\nmin_kandev_version: "+minVersion+"\n", 1)
	}
	parsed, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	return parsed
}

func TestRequiresHostCapabilitiesValidatorTable(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"valid", "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n", ""},
		{"unknown name", "    requires_host_capabilities: [{name: teleport, version: 1}]\n", `unknown capability "teleport"`},
		{"duplicate", "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}, {name: workspace_descriptor, version: 1}]\n", "duplicate capability"},
		{"zero version", "    requires_host_capabilities: [{name: workspace_descriptor, version: 0}]\n", "at least 1"},
		{"out of range", "    requires_host_capabilities: [{name: workspace_descriptor, version: 2}]\n", "outside the supported range 1-1"},
		{"drain receipt at version 1", "    requires_host_capabilities: [{name: drain_receipt, version: 1}]\n", ""},
		{"drain receipt at version 2", "    requires_host_capabilities: [{name: drain_receipt, version: 2}]\n", ""},
		{"drain receipt above range", "    requires_host_capabilities: [{name: drain_receipt, version: 3}]\n", "outside the supported range 1-2"},
		{"missing dependency", "    requires_host_capabilities: [{name: provider_maintenance, version: 1}]\n", `requires "provider_operations"`},
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

func TestMinKandevVersionMustCoverTheDeclaredCapabilities(t *testing.T) {
	declare := "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n"
	err := providerManifestWith(t, declare, "0.97.0").Validate()
	if err == nil || !strings.Contains(err.Error(), "workspace_descriptor") || !strings.Contains(err.Error(), "0.99.0") {
		t.Fatalf("Validate() = %v, want min_kandev_version rejection naming the capability and release", err)
	}
	if err := providerManifestWith(t, declare, "").Validate(); err == nil {
		t.Fatal("an absent min_kandev_version must be rejected for a manifest that requires a capability")
	}
	if err := providerManifestWith(t, declare, "0.99.0").Validate(); err != nil {
		t.Fatalf("min_kandev_version at the release must pass: %v", err)
	}
	if err := providerManifestWith(t, "", "").Validate(); err != nil {
		t.Fatalf("a manifest declaring no capability is unaffected: %v", err)
	}
}

func TestStateSchemaExpandsToTheContiguousReadableList(t *testing.T) {
	text := "    state_schema: {current: 3, min_readable: 2}\n"
	m := providerManifestWith(t, text, "")
	m.ExecutorProviders[0].SupportedStateVersions = nil
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	got := m.ExecutorProviders[0].SupportedStateVersions
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("supported_state_versions = %v, want [2 3]", got)
	}
}

func TestStateSchemaRejections(t *testing.T) {
	cases := []struct{ name, schema, want string }{
		{"min above current", "{current: 1, min_readable: 2}", "min_readable"},
		{"zero current", "{current: 0, min_readable: 0}", "current"},
		{"span over 16", "{current: 20, min_readable: 1}", "at most 16"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := providerManifestWith(t, "    state_schema: "+tc.schema+"\n", "")
			m.ExecutorProviders[0].SupportedStateVersions = nil
			if err := m.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
	both := providerManifestWith(t, "    state_schema: {current: 2, min_readable: 1}\n", "")
	if err := both.Validate(); err == nil || !strings.Contains(err.Error(), "both state_schema and supported_state_versions") {
		t.Fatalf("declaring both must be rejected, got %v", err)
	}
}

func TestStateBudgetBytes(t *testing.T) {
	declare := "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n"
	cases := []struct {
		budget  string
		wantErr bool
	}{
		{"1024", false}, {"12288", false}, {"1023", true}, {"12289", true},
	}
	for _, tc := range cases {
		err := providerManifestWith(t, declare+"    state_budget_bytes: "+tc.budget+"\n", "0.99.0").Validate()
		if (err != nil) != tc.wantErr {
			t.Fatalf("budget %s: error = %v, wantErr %v", tc.budget, err, tc.wantErr)
		}
	}
	err := providerManifestWith(t, "    state_budget_bytes: 2048\n", "").Validate()
	if err == nil || !strings.Contains(err.Error(), "state_budget_bytes requires") {
		t.Fatalf("a budget on a provider declaring no capability must be rejected, got %v", err)
	}
}

func TestStateBudgetDefault(t *testing.T) {
	declare := "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n"
	m := providerManifestWith(t, declare, "0.99.0")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := m.ExecutorProviders[0].StateBudgetBytes; got != DefaultExecutorStateBudgetBytes {
		t.Fatalf("default budget = %d, want %d", got, DefaultExecutorStateBudgetBytes)
	}
}

func TestLaunchPolicyIDsRules(t *testing.T) {
	base := "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}, {name: launch_policy, version: 1}]\n"
	ok := providerManifestWith(t, base+"    launch_policy_ids: [sandbox, seccomp.v1]\n", "0.99.0")
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	for name, ids := range map[string]string{
		"duplicate": "[a, a]", "uppercase": "[Bad]", "empty": `[""]`,
	} {
		if err := providerManifestWith(t, base+"    launch_policy_ids: "+ids+"\n", "0.99.0").Validate(); err == nil {
			t.Fatalf("%s launch_policy_ids must be rejected", name)
		}
	}
	if err := providerManifestWith(t, "    launch_policy_ids: [a]\n", "").Validate(); err == nil {
		t.Fatal("launch_policy_ids without the launch_policy capability must be rejected")
	}
}

func TestDeprecatedCapabilityWarnsInsteadOfFailing(t *testing.T) {
	// Registry has no deprecated rows yet; the validator exposes warnings through Warnings().
	m := providerManifestWith(t, "    requires_host_capabilities: [{name: workspace_descriptor, version: 1}]\n", "0.99.0")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if warnings := m.ExecutorProviderWarnings(); len(warnings) != 0 {
		t.Fatalf("no deprecated capability is declared, got warnings %v", warnings)
	}
}
