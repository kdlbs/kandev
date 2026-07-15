package telemetry

import (
	"context"
	"testing"
)

func TestConsentDefaultsToUnasked(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	if err := svc.loadConsent(context.Background()); err != nil {
		t.Fatalf("loadConsent: %v", err)
	}
	state := svc.Consent()
	if state.Status != ConsentUnasked {
		t.Fatalf("expected unasked, got %q", state.Status)
	}
	if state.InstallID != "" {
		t.Fatalf("expected no install id before consent, got %q", state.InstallID)
	}
}

func TestGrantMintsInstallIDAndPersists(t *testing.T) {
	svc, store, _ := newTestService(t, nil, Options{})
	state, err := svc.SetConsent(context.Background(), ConsentGranted)
	if err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
	if state.Status != ConsentGranted || state.InstallID == "" {
		t.Fatalf("expected granted with install id, got %+v", state)
	}

	// A fresh service over the same store must see the same state.
	reloaded := NewService(store, nil, newTestLogger(), Options{Sink: &fakeSink{}})
	if err := reloaded.loadConsent(context.Background()); err != nil {
		t.Fatalf("loadConsent: %v", err)
	}
	got := reloaded.Consent()
	if got.Status != ConsentGranted || got.InstallID != state.InstallID {
		t.Fatalf("reload mismatch: %+v vs %+v", got, state)
	}
}

func TestGrantIsStableAcrossRepeatedGrants(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	first, err := svc.SetConsent(context.Background(), ConsentGranted)
	if err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
	second, err := svc.SetConsent(context.Background(), ConsentGranted)
	if err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
	if first.InstallID != second.InstallID {
		t.Fatalf("install id must be stable: %q vs %q", first.InstallID, second.InstallID)
	}
}

func TestDenyClearsInstallID(t *testing.T) {
	svc, store, _ := newTestService(t, nil, Options{})
	if _, err := svc.SetConsent(context.Background(), ConsentGranted); err != nil {
		t.Fatalf("SetConsent(granted): %v", err)
	}
	state, err := svc.SetConsent(context.Background(), ConsentDenied)
	if err != nil {
		t.Fatalf("SetConsent(denied): %v", err)
	}
	if state.Status != ConsentDenied || state.InstallID != "" {
		t.Fatalf("expected denied without install id, got %+v", state)
	}

	reloaded := NewService(store, nil, newTestLogger(), Options{Sink: &fakeSink{}})
	if err := reloaded.loadConsent(context.Background()); err != nil {
		t.Fatalf("loadConsent: %v", err)
	}
	if got := reloaded.Consent(); got.Status != ConsentDenied || got.InstallID != "" {
		t.Fatalf("reload after deny mismatch: %+v", got)
	}
}

func TestSetConsentRejectsInvalidStatus(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	for _, status := range []ConsentStatus{ConsentUnasked, "yes", ""} {
		if _, err := svc.SetConsent(context.Background(), status); err == nil {
			t.Fatalf("expected error for status %q", status)
		}
	}
}
