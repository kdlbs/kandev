package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// TestNewManagerSizesUpdatesChannelFromDetachedEventLimit pins
// AC-EXECUTORS-SURVIVAL-001.6: the per-instance retained-event limit is the
// updates channel's own buffer, not an incidental hardcoded size, so a
// non-default configured value must be honoured.
func TestNewManagerSizesUpdatesChannelFromDetachedEventLimit(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:            t.TempDir(),
		DetachedEventLimit: 7,
	}, newTestLogger(t))

	if cap(mgr.updatesCh) != 7 {
		t.Fatalf("cap(updatesCh) = %d, want 7 (from DetachedEventLimit)", cap(mgr.updatesCh))
	}
}

// TestNewManagerDefaultsUpdatesChannelCapacityWhenUnset pins the fallback for
// the many call sites (including most of this package's own tests) that
// construct config.InstanceConfig directly without going through
// Config.NewInstanceConfig, so DetachedEventLimit is left at its Go
// zero value rather than the configured default of 100.
func TestNewManagerDefaultsUpdatesChannelCapacityWhenUnset(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))

	if cap(mgr.updatesCh) != defaultUpdatesChannelCapacity {
		t.Fatalf("cap(updatesCh) = %d, want the default %d", cap(mgr.updatesCh), defaultUpdatesChannelCapacity)
	}
}
