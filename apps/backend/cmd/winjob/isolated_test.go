package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsolatedIdentityPreservesExactWindowsTicks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launch %literal% [record].json")
	want := isolatedIdentity{PID: 1234, StartedTicks: "639277812345678901"}
	if err := writeIsolatedIdentity(path, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got isolatedIdentity
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("identity = %+v, want %+v", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("publication left %d files, want only the complete record", len(entries))
	}
}

func TestIsolatedIdentityFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(path, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeIsolatedIdentity(path, isolatedIdentity{PID: 1234}); err == nil {
		t.Fatal("publication over a directory succeeded")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "preserved" {
		t.Fatalf("existing destination changed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("failed publication left %d entries, want only existing destination", len(entries))
	}
}
