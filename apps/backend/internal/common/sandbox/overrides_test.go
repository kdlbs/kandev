package sandbox

import (
	"errors"
	"testing"
)

func TestSandboxPolicyOverrides(t *testing.T) {
	constraints := OverrideConstraints{ForbiddenArguments: []string{"--unsafe-route", "--external-policy"}, ForbiddenEnvironment: []string{"FIXTURE_ALLOW_ALL"}}
	for _, args := range [][]string{{"--unsafe-route"}, {"--unsafe-route=true"}, {"--external-policy", "/tmp/policy"}} {
		if err := ValidateOverrides(args, nil, constraints); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("provider override accepted: %v", args)
		}
	}
	for _, key := range []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "BASH_ENV", "ENV", "HOME", "XDG_CONFIG_HOME", "FIXTURE_ALLOW_ALL", "PATH", "NODE_OPTIONS", "PYTHONPATH", "RUBYOPT", "GIT_CONFIG_COUNT", "KANDEV_CONTROL_TOKEN", "TMPDIR"} {
		if err := ValidateOverrides(nil, map[string]string{key: "value"}, constraints); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("environment override accepted: %s", key)
		}
	}
	if err := ValidateOverrides([]string{"--safe-mode"}, map[string]string{"LANG": "C.UTF-8"}, constraints); err != nil {
		t.Fatal(err)
	}
}
