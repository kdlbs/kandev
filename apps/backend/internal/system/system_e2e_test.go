package system

import "testing"

func TestE2ENPMRegistryURLIsStrictlyTestGated(t *testing.T) {
	t.Setenv(e2eNPMRegistryURLEnv, " http://127.0.0.1:1234/kandev ")

	t.Setenv("KANDEV_E2E_MOCK", "false")
	if got := e2eNPMRegistryURL(); got != "" {
		t.Fatalf("e2eNPMRegistryURL() without test gate = %q, want empty", got)
	}

	t.Setenv("KANDEV_E2E_MOCK", "true")
	if got := e2eNPMRegistryURL(); got != "http://127.0.0.1:1234/kandev" {
		t.Fatalf("e2eNPMRegistryURL() = %q", got)
	}
}
