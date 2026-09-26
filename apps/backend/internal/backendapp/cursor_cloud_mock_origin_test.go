package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/cursorcloud"
)

func TestCursorCloudProdIgnoresMockOrigin(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "false")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "false")
	t.Setenv(cursorcloud.MockSelectorEnv, "true")
	t.Setenv(cursorcloud.MockOriginEnv, "http://127.0.0.1:18181")

	origin, err := cursorcloud.RuntimeAPIOrigin(true)
	if err != nil {
		t.Fatal(err)
	}
	if origin != cursorcloud.APIOrigin {
		t.Fatalf("production origin = %q, want fixed provider origin %q", origin, cursorcloud.APIOrigin)
	}
}

func TestCursorCloudDevIgnoresMockOrigin(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "false")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")
	t.Setenv(cursorcloud.MockSelectorEnv, "true")
	t.Setenv(cursorcloud.MockOriginEnv, "http://127.0.0.1:18181")

	origin, err := cursorcloud.RuntimeAPIOrigin(true)
	if err != nil {
		t.Fatal(err)
	}
	if origin != cursorcloud.APIOrigin {
		t.Fatalf("development origin = %q, want fixed provider origin %q", origin, cursorcloud.APIOrigin)
	}
}

func TestCursorCloudE2ELoopbackCallback(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "false")
	t.Setenv(cursorcloud.MockSelectorEnv, "true")
	if err := cursorcloud.ValidateCallbackURL("http://127.0.0.1:18182" + cursorcloud.ManagedCallbackPath); err != nil {
		t.Fatalf("e2e loopback callback rejected: %v", err)
	}
	if err := cursorcloud.ValidateCallbackURL("http://callback.example.test" + cursorcloud.ManagedCallbackPath); err == nil {
		t.Fatal("non-loopback HTTP callback accepted in e2e")
	}
}
