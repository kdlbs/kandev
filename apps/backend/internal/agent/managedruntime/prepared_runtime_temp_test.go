//go:build !windows

package managedruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedRuntimeResolvesTrustedTemporaryParent(t *testing.T) {
	parent := t.TempDir()
	realParent := filepath.Join(parent, "real")
	if err := os.Mkdir(realParent, 0700); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(parent, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", linkedParent)
	prepared, err := PreparePrivateRuntime(context.Background(), "managed-acp@1.2.3", nil, func(_ context.Context, command ManagedRuntimeCommand) error {
		return populatePreparedRuntimeCache(command.Args, "managed-acp@1.2.3")
	})
	if err != nil {
		t.Fatalf("prepare under a trusted symlinked temporary parent: %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	}()
	canonicalParent, err := filepath.EvalSymlinks(realParent)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(prepared.ProjectPrefix())) != canonicalParent {
		t.Fatalf("private project = %q, want canonical parent %q", prepared.ProjectPrefix(), canonicalParent)
	}
}
