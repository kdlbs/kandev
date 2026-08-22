package lifecycle

import "testing"

// TestBuildContainerLaunchConfig_AllowUserNamespacesFailsClosed pins the
// parsing of the allow_user_namespaces launch metadata key. The value relaxes
// the container's seccomp and AppArmor confinement, so anything that is not
// exactly "true" must leave it off rather than being coerced to on. Operators
// can write arbitrary strings into a profile's config map through the
// HTTP/WS settings API, and those land here verbatim.
func TestBuildContainerLaunchConfig_AllowUserNamespacesFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{name: "exact true enables", value: "true", want: true},
		{name: "uppercase does not enable", value: "TRUE"},
		{name: "mixed case does not enable", value: "True"},
		{name: "numeric one does not enable", value: "1"},
		{name: "yes does not enable", value: "yes"},
		{name: "padded true does not enable", value: " true "},
		{name: "empty string does not enable", value: ""},
		{name: "false does not enable", value: "false"},
		{name: "bool true does not enable", value: true},
		{name: "absent key does not enable", value: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{}
			if tt.value != nil {
				metadata[MetadataKeyAllowUserNamespaces] = tt.value
			}
			r := &DockerExecutor{}
			cfg, err := r.buildContainerLaunchConfig(&ExecutorCreateRequest{Metadata: metadata})
			if err != nil {
				t.Fatalf("buildContainerLaunchConfig() error = %v", err)
			}
			if cfg.AllowUserNamespaces != tt.want {
				t.Errorf("AllowUserNamespaces = %v for %#v, want %v",
					cfg.AllowUserNamespaces, tt.value, tt.want)
			}
		})
	}
}
