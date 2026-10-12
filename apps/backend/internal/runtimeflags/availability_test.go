package runtimeflags

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

// TestStateForReportsAvailabilityFromProbe pins the additive availability
// mechanism from system-design/agent-survival-across-restart-02.md: a nil-able
// probe on the flag definition, present in the serialized state only when the
// flag is unavailable, and an unavailable flag behaves as disabled regardless
// of its stored value.
func TestStateForReportsAvailabilityFromProbe(t *testing.T) {
	svc := NewService(&memoryStore{}, Options{
		DefaultValues: map[string]bool{"features.testProbe": true},
		RuntimeValues: map[string]bool{"features.testProbe": true},
		IsExplicitEnv: func(string) bool { return false },
	})
	def := RuntimeFlagDefinition{
		Key:             "features.testProbe",
		EnvVar:          "KANDEV_FEATURES_TEST_PROBE",
		Kind:            KindFeature,
		Label:           "Test probe flag",
		Description:     "test",
		Stability:       StabilityExperimental,
		RiskLevel:       RiskHigh,
		RiskDescription: "test",
		RestartRequired: true,
		Mutable:         true,
		Available:       func() (bool, string) { return false, "platform_unsupported" },
	}

	state := svc.stateFor(def, map[string]bool{"features.testProbe": true})

	if state.Availability == nil {
		t.Fatal("Availability = nil, want a populated availability object for an unavailable flag")
	}
	if state.Availability.Available {
		t.Fatal("Availability.Available = true, want false")
	}
	if state.Availability.ReasonCode != "platform_unsupported" {
		t.Fatalf("Availability.ReasonCode = %q, want %q", state.Availability.ReasonCode, "platform_unsupported")
	}
	if state.EffectiveValue {
		t.Fatal("EffectiveValue = true, want false: an unavailable flag must behave as disabled regardless of its stored override")
	}
}

func TestStateForNilProbeReportsNoAvailabilityObject(t *testing.T) {
	svc := NewService(&memoryStore{}, Options{
		DefaultValues: map[string]bool{"features.office": false},
		RuntimeValues: map[string]bool{"features.office": false},
		IsExplicitEnv: func(string) bool { return false },
	})
	def, ok := DefinitionByKey("features.office")
	if !ok {
		t.Fatal("features.office definition missing")
	}
	state := svc.stateFor(def, nil)
	if state.Availability != nil {
		t.Fatalf("Availability = %+v, want nil when the definition has no probe", state.Availability)
	}
}

// TestSetOverrideRefusesEnablingAnUnavailableFlag pins that turning an
// unavailable flag on is refused rather than silently stored, while clearing
// or disabling it is still allowed. The probe is injected on a throwaway
// registration since the real agent-survival probe is host-dependent.
func TestSetOverrideRefusesEnablingAnUnavailableFlag(t *testing.T) {
	const key = "features.testProbeRefuse"
	registrations = append(registrations, runtimeFlagRegistration{
		definition: RuntimeFlagDefinition{
			Key:             key,
			EnvVar:          "KANDEV_FEATURES_TEST_PROBE_REFUSE",
			Kind:            KindFeature,
			Label:           "Test probe refuse flag",
			Description:     "test",
			Stability:       StabilityExperimental,
			RiskLevel:       RiskHigh,
			RiskDescription: "test",
			RestartRequired: true,
			Mutable:         true,
			Available:       func() (bool, string) { return false, "platform_unsupported" },
		},
		read:  func(*config.Config) bool { return false },
		apply: func(*config.Config, bool) {},
	})
	t.Cleanup(func() { registrations = registrations[:len(registrations)-1] })

	svc := NewService(&memoryStore{}, Options{
		DefaultValues: map[string]bool{key: false},
		RuntimeValues: map[string]bool{key: false},
		IsExplicitEnv: func(string) bool { return false },
	})

	enable := true
	if _, err := svc.SetOverride(context.Background(), key, &enable); !errors.Is(err, ErrFlagUnavailable) {
		t.Fatalf("SetOverride(true) error = %v, want %v", err, ErrFlagUnavailable)
	}

	disable := false
	if _, err := svc.SetOverride(context.Background(), key, &disable); err != nil {
		t.Fatalf("SetOverride(false) on an unavailable flag: %v", err)
	}

	if _, err := svc.SetOverride(context.Background(), key, nil); err != nil {
		t.Fatalf("SetOverride(nil) on an unavailable flag: %v", err)
	}
}
