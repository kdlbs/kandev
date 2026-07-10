package runtimeflags

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	values map[string]bool
}

func (m *memoryStore) ListOverrides(context.Context) (map[string]bool, error) {
	out := make(map[string]bool, len(m.values))
	for k, v := range m.values {
		out[k] = v
	}
	return out, nil
}

func (m *memoryStore) SetOverride(_ context.Context, key string, value bool) error {
	if m.values == nil {
		m.values = map[string]bool{}
	}
	m.values[key] = value
	return nil
}

func (m *memoryStore) DeleteOverride(_ context.Context, key string) error {
	delete(m.values, key)
	return nil
}

func TestServiceOverrideWinsOverProfileDefault(t *testing.T) {
	svc := NewService(&memoryStore{values: map[string]bool{"features.office": true}}, Options{
		DefaultValues: map[string]bool{"features.office": false},
		RuntimeValues: map[string]bool{"features.office": false},
		IsExplicitEnv: func(string) bool { return false },
	})
	states, err := svc.ListStates(context.Background())
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	office := stateByKey(t, states, "features.office")
	if !office.EffectiveValue {
		t.Fatal("EffectiveValue = false, want true")
	}
	if office.Source != SourceOverride {
		t.Fatalf("Source = %q, want %q", office.Source, SourceOverride)
	}
	if !office.RequiresRestartToApply {
		t.Fatal("RequiresRestartToApply = false, want true")
	}
}

func TestServiceExplicitEnvLocksOverride(t *testing.T) {
	svc := NewService(&memoryStore{values: map[string]bool{"features.office": false}}, Options{
		DefaultValues: map[string]bool{"features.office": false},
		RuntimeValues: map[string]bool{"features.office": true},
		EnvValues:     map[string]bool{"KANDEV_FEATURES_OFFICE": true},
		IsExplicitEnv: func(name string) bool { return name == "KANDEV_FEATURES_OFFICE" },
	})
	states, err := svc.ListStates(context.Background())
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	office := stateByKey(t, states, "features.office")
	if !office.EffectiveValue {
		t.Fatal("EffectiveValue = false, want true")
	}
	if office.Source != SourceEnv {
		t.Fatalf("Source = %q, want %q", office.Source, SourceEnv)
	}
	if !office.EnvLocked {
		t.Fatal("EnvLocked = false, want true")
	}
}

func TestServiceExplicitImpliedEnvLocksDebugMode(t *testing.T) {
	svc := NewService(&memoryStore{values: map[string]bool{"debug.devMode": false}}, Options{
		DefaultValues: map[string]bool{"debug.devMode": false},
		RuntimeValues: map[string]bool{"debug.devMode": false},
		EnvValues: map[string]bool{
			"KANDEV_DEBUG_DEV_MODE":      false,
			"KANDEV_DEBUG_PPROF_ENABLED": true,
		},
		IsExplicitEnv: func(name string) bool { return name == "KANDEV_DEBUG_PPROF_ENABLED" },
	})
	states, err := svc.ListStates(context.Background())
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	debug := stateByKey(t, states, "debug.devMode")
	if !debug.EffectiveValue {
		t.Fatal("EffectiveValue = false, want true")
	}
	if debug.Source != SourceEnv {
		t.Fatalf("Source = %q, want %q", debug.Source, SourceEnv)
	}
	if !debug.EnvLocked {
		t.Fatal("EnvLocked = false, want true")
	}
}

func TestServiceNilStoreFailsFast(t *testing.T) {
	svc := NewService(nil, Options{})
	if _, err := svc.ListStates(context.Background()); !errors.Is(err, ErrStoreUnset) {
		t.Fatalf("ListStates error = %v, want %v", err, ErrStoreUnset)
	}
	if _, err := svc.SetOverride(context.Background(), "features.office", nil); !errors.Is(err, ErrStoreUnset) {
		t.Fatalf("SetOverride error = %v, want %v", err, ErrStoreUnset)
	}
}

func stateByKey(t *testing.T, states []RuntimeFlagState, key string) RuntimeFlagState {
	t.Helper()
	for _, state := range states {
		if state.Key == key {
			return state
		}
	}
	t.Fatalf("state %q missing", key)
	return RuntimeFlagState{}
}
