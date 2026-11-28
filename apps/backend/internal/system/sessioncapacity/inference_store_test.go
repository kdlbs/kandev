package sessioncapacity

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

const ownerJSON = `{"version":1,"active":{"attempt-a":{"key":"attempt-a","instance_id":"instance-a","owner_id":"owner-a","attempt_id":"attempt-a","grade":"A","grade_known":true,"state":"reserved"}}}`

func ownerSettingsJSON() []byte {
	return []byte(`{"enabled":true,"max_sessions":2,"policy":"all","grade_limits":{"A":2},"_inference_bindings":` + ownerJSON + `}`)
}

func TestInferenceOwnerOnlyDoesNotConfigureCapacity(t *testing.T) {
	raw := &compareAndSwapRawStore{}
	ctx := context.Background()
	if err := raw.Save(ctx, SettingsKey, []byte(`{"_inference_bindings":`+ownerJSON+`}`)); err != nil {
		t.Fatal(err)
	}
	got, err := NewStore(raw).LoadConsistent(ctx)
	if err != nil || got != nil {
		t.Fatalf("private ownership configured capacity: settings=%+v err=%v", got, err)
	}
}

func TestInferenceOwnerSettingsWritesPreserveBindings(t *testing.T) {
	for _, cas := range []bool{false, true} {
		for _, update := range []bool{false, true} {
			t.Run(testStoreOperationName(cas, update), func(t *testing.T) {
				var raw RawStore = &memoryRawStore{}
				if cas {
					raw = &compareAndSwapRawStore{}
				}
				ctx := context.Background()
				if err := raw.Save(ctx, SettingsKey, ownerSettingsJSON()); err != nil {
					t.Fatal(err)
				}
				if err := changeOwnerSettings(ctx, NewStore(raw), update); err != nil {
					t.Fatal(err)
				}
				value, _, err := raw.Get(ctx, SettingsKey)
				if err != nil {
					t.Fatal(err)
				}
				assertOwnerField(t, value, []byte(ownerJSON))
				settings, err := NewStore(raw).Load(ctx)
				if err != nil || settings == nil || settings.MaxSessions != 3 {
					t.Fatalf("settings update lost: %+v, %v", settings, err)
				}
			})
		}
	}
}

func testStoreOperationName(cas, update bool) string {
	name := "fallback"
	if cas {
		name = "cas"
	}
	if update {
		return name + "_update"
	}
	return name + "_save"
}

func changeOwnerSettings(ctx context.Context, store *Store, update bool) error {
	next := Settings{Enabled: true, MaxSessions: 3, Policy: PolicyAll, GradeLimits: map[string]int{"A": 2}}
	if !update {
		return store.Save(ctx, next)
	}
	_, err := store.Update(ctx, func(*Settings) (Settings, error) { return next, nil })
	return err
}

func assertOwnerField(t *testing.T, raw, want []byte) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	var gotOwner, wantOwner any
	if err := json.Unmarshal(fields["_inference_bindings"], &gotOwner); err != nil {
		t.Fatalf("ownership disappeared: %s: %v", raw, err)
	}
	if err := json.Unmarshal(want, &wantOwner); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotOwner, wantOwner) {
		t.Fatalf("ownership changed: got=%s want=%s", fields["_inference_bindings"], want)
	}
}

type interleavedOwnerStore struct {
	*compareAndSwapRawStore
	t               *testing.T
	injected        bool
	concurrentOwner []byte
}

func (r *interleavedOwnerStore) CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error) {
	if !r.injected {
		r.injected = true
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(expected, &fields); err != nil {
			r.t.Fatal(err)
		}
		fields["_inference_bindings"] = r.concurrentOwner
		next, err := json.Marshal(fields)
		if err != nil {
			r.t.Fatal(err)
		}
		if err := r.Save(ctx, key, next); err != nil {
			r.t.Fatal(err)
		}
	}
	return r.compareAndSwapRawStore.CompareAndSwap(ctx, key, expected, value)
}

func TestInferenceOwnerSettingsCASRetriesKeepNewOwner(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(testStoreOperationName(true, update), func(t *testing.T) {
			ctx := context.Background()
			owner := []byte(`{"version":1,"active":{"attempt-b":{"key":"attempt-b","instance_id":"instance-b","owner_id":"owner-b","attempt_id":"attempt-b","grade":"B","grade_known":true,"state":"running"}}}`)
			raw := &interleavedOwnerStore{compareAndSwapRawStore: &compareAndSwapRawStore{}, t: t, concurrentOwner: owner}
			if err := raw.Save(ctx, SettingsKey, ownerSettingsJSON()); err != nil {
				t.Fatal(err)
			}
			if err := changeOwnerSettings(ctx, NewStore(raw), update); err != nil {
				t.Fatal(err)
			}
			if !raw.injected {
				t.Fatal("settings did not use shared CAS boundary")
			}
			value, _, err := raw.Get(ctx, SettingsKey)
			if err != nil {
				t.Fatal(err)
			}
			assertOwnerField(t, value, owner)
		})
	}
}

func TestInferenceOwnerCorruptionIsNotSilentlyDiscarded(t *testing.T) {
	for _, owner := range []string{`null`, `{"version":2,"active":{}}`, `{"version":1,"active":{"attempt-a":{"key":"other"}}}`} {
		raw := &compareAndSwapRawStore{}
		ctx := context.Background()
		value := []byte(`{"enabled":true,"max_sessions":2,"_inference_bindings":` + owner + `}`)
		if err := raw.Save(ctx, SettingsKey, value); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(raw).LoadConsistent(ctx); !errors.Is(err, ErrInvalidPersisted) {
			t.Fatalf("corrupt owner accepted: %s: %v", owner, err)
		}
		if err := changeOwnerSettings(ctx, NewStore(raw), true); !errors.Is(err, ErrInvalidPersisted) {
			t.Fatalf("corrupt owner overwritten: %s: %v", owner, err)
		}
		got, _, err := raw.Get(ctx, SettingsKey)
		if err != nil || string(got) != string(value) {
			t.Fatalf("failed update changed source: %s %v", got, err)
		}
	}
}
