package configloader

import (
	"testing"
)

func TestWriteReadMemoryEntry(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	content := "Alice is the frontend lead. Prefers TypeScript."
	err := writer.WriteMemoryEntry("default", "ceo", "knowledge", "people-alice", content)
	if err != nil {
		t.Fatalf("WriteMemoryEntry() error: %v", err)
	}

	got, err := writer.ReadMemoryEntry("default", "ceo", "knowledge", "people-alice")
	if err != nil {
		t.Fatalf("ReadMemoryEntry() error: %v", err)
	}
	if got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestListMemoryEntries(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	// Write two entries in the same layer.
	if err := writer.WriteMemoryEntry("default", "ceo", "operating", "style", "Be concise."); err != nil {
		t.Fatalf("write style: %v", err)
	}
	if err := writer.WriteMemoryEntry("default", "ceo", "operating", "tone", "Professional tone."); err != nil {
		t.Fatalf("write tone: %v", err)
	}

	entries, err := writer.ListMemoryEntries("default", "ceo", "operating")
	if err != nil {
		t.Fatalf("ListMemoryEntries() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	keys := make(map[string]bool)
	for _, e := range entries {
		keys[e.Key] = true
		if e.Layer != "operating" {
			t.Errorf("entry layer = %q, want %q", e.Layer, "operating")
		}
	}
	if !keys["style"] || !keys["tone"] {
		t.Error("missing expected memory entries")
	}
}

func TestDeleteMemoryEntry(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	if err := writer.WriteMemoryEntry("default", "ceo", "knowledge", "fact", "test"); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := writer.DeleteMemoryEntry("default", "ceo", "knowledge", "fact"); err != nil {
		t.Fatalf("DeleteMemoryEntry() error: %v", err)
	}

	_, err := writer.ReadMemoryEntry("default", "ceo", "knowledge", "fact")
	if err == nil {
		t.Error("expected error reading deleted memory entry")
	}
}

func TestListMemoryEntriesEmptyLayer(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	// Listing a non-existent layer should return empty, not error.
	entries, err := writer.ListMemoryEntries("default", "ceo", "sessions")
	if err != nil {
		t.Fatalf("ListMemoryEntries() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestDeleteNonexistentMemoryEntry(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	// Should not error when deleting a non-existent entry.
	if err := writer.DeleteMemoryEntry("default", "ceo", "knowledge", "nonexistent"); err != nil {
		t.Fatalf("DeleteMemoryEntry() should not error: %v", err)
	}
}
