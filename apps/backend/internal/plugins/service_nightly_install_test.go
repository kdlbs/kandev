package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/plugins/store"
)

// @covers AC-PLUGINS-PLUGINS-001.14
func TestServiceInstall_NightlyMinimumHostVersion(t *testing.T) {
	const nightly = "v1.2.4-nightly.sha123456789abc"
	for _, tt := range []struct {
		name    string
		running string
		minimum string
		reject  bool
	}{
		{name: "nightly rejects future", running: nightly, minimum: "2.0.0", reject: true},
		{name: "nightly rejects same core stable", running: nightly, minimum: "1.2.4", reject: true},
		{name: "nightly accepts earlier stable", running: nightly, minimum: "1.2.3"},
		{name: "stable rejects future", running: "v1.2.4", minimum: "2.0.0", reject: true},
		{name: "stable accepts same core", running: "v1.2.4", minimum: "1.2.4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, dir, fsStore, rt := newTestServiceWithDir(t)
			svc.SetKandevVersion(tt.running)
			const id = "kandev-plugin-nightly"
			rec, err := svc.Install(t.Context(), minKandevVersionPackage(t, id, "1.0.0", tt.minimum))
			if (err != nil) != tt.reject {
				t.Errorf("Install() error = %v, want rejection %t", err, tt.reject)
			}
			if (rec == nil) != tt.reject {
				t.Errorf("Install() record = %v, want absent %t", rec, tt.reject)
			}
			_, statErr := os.Stat(filepath.Join(dir, id, "1.0.0"))
			if tt.reject && !os.IsNotExist(statErr) {
				t.Errorf("rejected package directory remains: %v", statErr)
			} else if !tt.reject && statErr != nil {
				t.Errorf("accepted package directory missing: %v", statErr)
			}
			persisted, getErr := fsStore.Get(id)
			if tt.reject && !errors.Is(getErr, store.ErrNotFound) {
				t.Errorf("rejected package store error = %v, want not found", getErr)
			} else if !tt.reject && (getErr != nil || persisted == nil || persisted.Status != StatusActive) {
				t.Errorf("accepted package persisted record = %v, error = %v", persisted, getErr)
			}
			registered, exists := svc.registry.Get(id)
			if exists == tt.reject {
				t.Errorf("registry entry present = %t, want %t", exists, !tt.reject)
			}
			if !tt.reject && exists && registered.Status != StatusActive {
				t.Errorf("registry status = %q, want active", registered.Status)
			}
			wantStarts := 1
			if tt.reject {
				wantStarts = 0
			}
			if got := rt.startCallCount(id); got != wantStarts {
				t.Errorf("runtime Start calls = %d, want %d", got, wantStarts)
			}
			if rt.Running(id) == tt.reject {
				t.Errorf("runtime running = %t, want %t", rt.Running(id), !tt.reject)
			}
		})
	}
}
