package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestServerBindDeploymentFallback(t *testing.T) {
	tests := []struct {
		name, service, container, host, yaml string
		want                                 []string
	}{
		{name: "standalone", want: []string{"127.0.0.1"}},
		{name: "service", service: "true", want: []string{"0.0.0.0"}},
		{name: "container", container: "1", want: []string{"0.0.0.0"}},
		{name: "service with YAML host", service: "true", yaml: "server:\n  host: ::1\n", want: []string{"::1"}},
		{name: "container with YAML hosts", container: "1", yaml: "server:\n  hosts: [127.0.0.1, '::1']\n", want: []string{"127.0.0.1", "::1"}},
		{name: "environment overrides YAML", container: "1", host: "127.0.0.1", yaml: "server:\n  hosts: [0.0.0.0]\n", want: []string{"127.0.0.1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KANDEV_RUNNING_AS_SERVICE", tc.service)
			t.Setenv("KANDEV_INTERNAL_CONTAINER_RUNTIME", tc.container)
			t.Setenv("KANDEV_SERVER_HOST", tc.host)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := LoadWithPath(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := cfg.Server.ResolvedBinds()
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("ResolvedBinds() = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
