package manifest

import (
	"strings"
	"testing"
)

func TestRuntimeAPIURLValidatorTable(t *testing.T) {
	forward := "    requires_host_capabilities: [{name: api_url_forward, version: 1}]\n"
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"absent is host_public", "", ""},
		{"host_public without a capability", "    runtime_api_url: host_public\n", ""},
		{"provider_forwarded with the capability", forward + "    runtime_api_url: provider_forwarded\n", ""},
		{"provider_forwarded without the capability", "    runtime_api_url: provider_forwarded\n", "runtime_api_url provider_forwarded requires the api_url_forward capability"},
		{"provider_forwarded with another capability only",
			"    requires_host_capabilities: [{name: tunnel_transport, version: 1}]\n    runtime_api_url: provider_forwarded\n",
			"runtime_api_url provider_forwarded requires the api_url_forward capability"},
		{"unknown value", forward + "    runtime_api_url: relay\n", "runtime_api_url must be host_public or provider_forwarded"},
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

func TestRuntimeAPIURLDefaultsToHostPublic(t *testing.T) {
	m := providerManifestWith(t, "", "0.99.0")
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if got := m.ExecutorProviders[0].RuntimeAPIURL; got != RuntimeAPIURLHostPublic {
		t.Fatalf("RuntimeAPIURL = %q, want %q", got, RuntimeAPIURLHostPublic)
	}
}
