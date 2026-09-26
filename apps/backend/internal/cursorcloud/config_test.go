package cursorcloud

import (
	"testing"
)

func TestRuntimeAPIOriginIgnoresMockOriginOutsideE2E(t *testing.T) {
	for _, profile := range []struct {
		name string
		env  map[string]string
	}{{"prod", nil}, {"dev", map[string]string{"KANDEV_DEBUG_DEV_MODE": "true"}}} {
		t.Run(profile.name, func(t *testing.T) {
			t.Setenv("KANDEV_E2E_MOCK", "")
			t.Setenv("KANDEV_DEBUG_DEV_MODE", "")
			t.Setenv(MockSelectorEnv, "true")
			t.Setenv(MockOriginEnv, "http://127.0.0.1:43123")
			for name, value := range profile.env {
				t.Setenv(name, value)
			}
			got, err := RuntimeAPIOrigin(true)
			if err != nil {
				t.Fatalf("RuntimeAPIOrigin: %v", err)
			}
			if got != APIOrigin {
				t.Fatalf("origin = %q, want fixed production origin %q", got, APIOrigin)
			}
		})
	}
}

func TestRuntimeAPIOriginUsesOnlyE2ELoopbackMock(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")
	t.Setenv(MockSelectorEnv, "true")
	t.Setenv(MockOriginEnv, "http://[::1]:43123")

	got, err := RuntimeAPIOrigin(true)
	if err != nil {
		t.Fatalf("RuntimeAPIOrigin: %v", err)
	}
	if got != "http://[::1]:43123" {
		t.Fatalf("origin = %q, want injected loopback origin", got)
	}
	if _, err := RuntimeAPIOrigin(false); err != nil {
		t.Fatalf("disabled feature must ignore fixture origin: %v", err)
	}

	t.Setenv(MockOriginEnv, "http://example.com")
	if _, err := RuntimeAPIOrigin(true); err == nil {
		t.Fatal("non-loopback mock origin unexpectedly accepted")
	}
}

func TestValidateCallbackURLAllowsLoopbackHTTPOnlyForE2EMock(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "")
	t.Setenv(MockSelectorEnv, "true")
	if err := ValidateCallbackURL("http://127.0.0.1:43210" + ManagedCallbackPath); err == nil {
		t.Fatal("loopback HTTP callback accepted outside e2e")
	}

	t.Setenv("KANDEV_E2E_MOCK", "true")
	if err := ValidateCallbackURL("http://localhost:43210" + ManagedCallbackPath); err != nil {
		t.Fatalf("e2e loopback callback rejected: %v", err)
	}
	for _, value := range []string{
		"http://localhost.evil:43210" + ManagedCallbackPath,
		"http://192.0.2.8:43210" + ManagedCallbackPath,
		"https://user:pass@example.com" + ManagedCallbackPath,
		"https://example.com" + ManagedCallbackPath + "?token=secret",
		"https://example.com/unregistered/callback",
	} {
		if err := ValidateCallbackURL(value); err == nil {
			t.Errorf("callback %q unexpectedly accepted", value)
		}
	}
	if err := ValidateCallbackURL("https://example.com" + ManagedCallbackPath); err != nil {
		t.Fatalf("HTTPS callback rejected: %v", err)
	}
	if err := ValidateCallbackURL("https://example.com"); err != nil {
		t.Fatalf("HTTPS callback origin rejected: %v", err)
	}
}

func TestManagedCallbackURLAppendsGrantWithoutQueryCredentials(t *testing.T) {
	for _, callback := range []string{
		"https://example.test",
		"https://example.test" + ManagedCallbackPath,
	} {
		got, err := ManagedCallbackURL(callback, "grant-1")
		if err != nil {
			t.Fatalf("ManagedCallbackURL(%q) error = %v", callback, err)
		}
		if got != "https://example.test"+ManagedCallbackPath+"/grant-1" {
			t.Errorf("ManagedCallbackURL(%q) = %q", callback, got)
		}
	}
	if _, err := ManagedCallbackURL("https://example.test/other", "grant-1"); err == nil {
		t.Fatal("unregistered callback path was accepted")
	}
}
