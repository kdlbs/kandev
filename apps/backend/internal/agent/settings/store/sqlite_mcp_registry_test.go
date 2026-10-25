package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig/registry"
)

func TestMCPRegistryCacheRoundTripAndSearch(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	entries := []registry.Entry{{
		Name:        "com.example/tools",
		Description: "Useful tools",
		Version:     "1.0.0",
		Status:      registry.StatusActive,
		Packages: []registry.Package{{
			RegistryType: "npm",
			Identifier:   "@example/tools",
			Version:      "1.0.0",
			Transport:    registry.Transport{Type: "stdio"},
		}},
	}}
	ctx := context.Background()
	if err := repo.ReplaceMCPRegistryEntries(ctx, entries); err != nil {
		t.Fatalf("ReplaceMCPRegistryEntries: %v", err)
	}
	got, err := repo.ListMCPRegistryEntries(ctx, "tools")
	if err != nil {
		t.Fatalf("ListMCPRegistryEntries: %v", err)
	}
	if len(got) != 1 || got[0].Packages[0].Identifier != "@example/tools" {
		t.Fatalf("registry entries = %#v", got)
	}
	if _, err := repo.GetMCPRegistryEntry(ctx, "com.example/tools@1.0.0"); err != nil {
		t.Fatalf("GetMCPRegistryEntry: %v", err)
	}
}

func TestMCPRegistrySyncStateRoundTrip(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	state := registry.SyncState{Degraded: true, LastError: "stale"}
	if err := repo.SaveMCPRegistrySyncState(ctx, state); err != nil {
		t.Fatalf("SaveMCPRegistrySyncState: %v", err)
	}
	got, err := repo.GetMCPRegistrySyncState(ctx)
	if err != nil {
		t.Fatalf("GetMCPRegistrySyncState: %v", err)
	}
	if !got.Degraded || got.LastError != "stale" {
		t.Fatalf("sync state = %#v", got)
	}
}
