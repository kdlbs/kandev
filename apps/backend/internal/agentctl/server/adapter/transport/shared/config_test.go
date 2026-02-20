package shared

import (
	"testing"
	"time"
)

func TestGetPermissionTimeout_Default(t *testing.T) {
	cfg := &Config{}
	got := cfg.GetPermissionTimeout()
	if got != DefaultPermissionTimeout {
		t.Errorf("GetPermissionTimeout() = %v, want %v", got, DefaultPermissionTimeout)
	}
}

func TestGetPermissionTimeout_Custom(t *testing.T) {
	cfg := &Config{PermissionTimeout: 30 * time.Second}
	got := cfg.GetPermissionTimeout()
	if got != 30*time.Second {
		t.Errorf("GetPermissionTimeout() = %v, want %v", got, 30*time.Second)
	}
}

func TestGetPermissionTimeout_Zero(t *testing.T) {
	cfg := &Config{PermissionTimeout: 0}
	got := cfg.GetPermissionTimeout()
	if got != DefaultPermissionTimeout {
		t.Errorf("GetPermissionTimeout() = %v, want %v (default)", got, DefaultPermissionTimeout)
	}
}
