package managedruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestManagedRuntimePreparationRetainsFilesWhileRunnerOwnsProcess(t *testing.T) {
	var live atomic.Bool
	live.Store(true)
	prepared, err := PreparePrivateRuntime(context.Background(), "@scope/managed-acp@1.2.3", nil,
		func(_ context.Context, command ManagedRuntimeCommand) error {
			command.RetainWhile(live.Load)
			return errors.New("process cleanup pending")
		})
	if err == nil || prepared == nil {
		t.Fatalf("preparation = %v, %v; want retained owner", prepared, err)
	}
	root := filepath.Dir(prepared.ProjectPrefix())
	t.Cleanup(func() { live.Store(false); _ = prepared.Close() })
	if err := prepared.Close(); err == nil {
		t.Fatal("removed private files while a process was owned")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	live.Store(false)
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("owner root after consumer cleanup = %v", err)
	}
}
