package sessioncapacity

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	_ "github.com/mattn/go-sqlite3"
)

func testInferenceOwner(key string) InferenceBinding {
	return InferenceBinding{Key: key, InstanceID: "instance", OwnerID: "owner", AttemptID: key, Grade: "A", GradeKnown: true, State: "reserved"}
}

func TestInferenceOwnerInvalidUpdateKeepsPersistedOwner(t *testing.T) {
	for _, field := range []string{"key", "owner", "instance", "attempt", "terminal"} {
		t.Run(field, func(t *testing.T) {
			ctx := context.Background()
			raw := &compareAndSwapRawStore{}
			if err := raw.Save(ctx, SettingsKey, ownerSettingsJSON()); err != nil {
				t.Fatal(err)
			}
			err := NewStore(raw).UpdateInferenceBindings(ctx, func(active map[string]InferenceBinding) error {
				invalid := testInferenceOwner("new")
				switch field {
				case "key":
					invalid.Key = "mismatch"
				case "owner":
					invalid.OwnerID = ""
				case "instance":
					invalid.InstanceID = ""
				case "attempt":
					invalid.AttemptID = ""
				case "terminal":
					invalid.TerminalState = "maybe_done"
				}
				active["new"] = invalid
				return nil
			})
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("invalid owner update accepted: %s: %v", field, err)
			}
			value, _, err := raw.Get(ctx, SettingsKey)
			if err != nil || string(value) != string(ownerSettingsJSON()) {
				t.Fatalf("invalid update damaged original state: %s, %v", value, err)
			}
		})
	}
}

func TestInferenceOwnerNilUpdateReturnsValidation(t *testing.T) {
	defer func() {
		if got := recover(); got != nil {
			t.Fatalf("nil callback panicked: %v", got)
		}
	}()
	err := NewStore(&compareAndSwapRawStore{}).UpdateInferenceBindings(context.Background(), nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("nil update = %v", err)
	}
}

func TestInferenceOwnerRequiresConsistentCAS(t *testing.T) {
	store := NewStore(&memoryRawStore{})
	if _, err := store.LoadInferenceBindings(context.Background()); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("non-CAS read = %v", err)
	}
	err := store.UpdateInferenceBindings(context.Background(), func(map[string]InferenceBinding) error { t.Fatal("non-CAS update callback called"); return nil })
	if !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("non-CAS update = %v", err)
	}
}

type interleavedSQLiteOwnerStore struct {
	*systemsettings.Store
	beforeCAS func() error
	injected  bool
}

func (s *interleavedSQLiteOwnerStore) CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error) {
	if !s.injected {
		s.injected = true
		if err := s.beforeCAS(); err != nil {
			return false, err
		}
	}
	return s.Store.CompareAndSwap(ctx, key, expected, value)
}

func newInferenceSQLiteStore(t *testing.T) *systemsettings.Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	raw, err := systemsettings.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInferenceOwnerSQLiteCASPreservesBothWriters(t *testing.T) {
	for _, settingsWriteFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(settingsWriteFirst), func(t *testing.T) {
			ctx := context.Background()
			raw := newInferenceSQLiteStore(t)
			if err := raw.Save(ctx, SettingsKey, ownerSettingsJSON()); err != nil {
				t.Fatal(err)
			}
			setSettings := func(store *Store) error { return changeOwnerSettings(ctx, store, false) }
			addOwner := func(store *Store) error {
				return store.UpdateInferenceBindings(ctx, func(active map[string]InferenceBinding) error { active["new"] = testInferenceOwner("new"); return nil })
			}
			first, concurrent := setSettings, addOwner
			if !settingsWriteFirst {
				first, concurrent = addOwner, setSettings
			}
			wrapper := &interleavedSQLiteOwnerStore{Store: raw, beforeCAS: func() error { return concurrent(NewStore(raw)) }}
			if err := first(NewStore(wrapper)); err != nil {
				t.Fatal(err)
			}
			if !wrapper.injected {
				t.Fatal("CAS boundary not exercised")
			}
			store := NewStore(raw)
			owners, err := store.LoadInferenceBindings(ctx)
			if err != nil || len(owners) != 2 || owners["new"].AttemptID != "new" || owners["attempt-a"].OwnerID != "owner-a" {
				t.Fatalf("owners lost: %+v, %v", owners, err)
			}
			settings, err := store.LoadConsistent(ctx)
			if err != nil || settings == nil || settings.MaxSessions != 3 || settings.GradeLimits["A"] != 2 {
				t.Fatalf("settings lost: %+v %v", settings, err)
			}
		})
	}
}
