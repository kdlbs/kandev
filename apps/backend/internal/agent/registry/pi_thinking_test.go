package registry

import "testing"

func TestPiThinkingMockRegistration(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_E2E_MOCK_PI", "true")
	reg, cleanup, err := Provide(newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup() //nolint:errcheck
	pi, ok := reg.Get("pi-acp")
	if !ok {
		t.Fatal("missing disposable Pi")
	}
	if pi.Runtime().Env["KANDEV_MOCK_PI_THINKING"] != "true" {
		t.Fatal("fixture mode missing from runtime")
	}
}
