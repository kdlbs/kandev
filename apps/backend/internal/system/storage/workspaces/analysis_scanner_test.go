package workspaces

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/system/storage/filescan"
)

func TestAnalyzeUsesConfiguredBoundedScanner(t *testing.T) {
	provider, root, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	for index, name := range []string{"first-task_abc", "second-task_def"} {
		workspace := createOwnedCandidate(t, root, name, OwnershipMarker{
			TaskID: name, TaskDirName: name, LayoutVersion: LayoutVersionSemantic,
		})
		if err := os.WriteFile(filepath.Join(workspace, "data"), []byte(name), 0o600); err != nil {
			t.Fatalf("write workspace %d: %v", index, err)
		}
	}
	var completed atomic.Int32
	provider.config.Scanner = filescan.NewLimiter(1)
	provider.config.OnProgress = func(progress filescan.Progress) {
		if progress.Phase == filescan.RootCompleted {
			completed.Add(1)
		}
	}

	if _, err := provider.Analyze(context.Background()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if completed.Load() != 2 {
		t.Fatalf("completed roots = %d, want two scanner roots", completed.Load())
	}
}
