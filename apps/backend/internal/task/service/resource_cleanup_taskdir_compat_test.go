package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestArchiveTaskDirectoryCaptureCompatibility(t *testing.T) {
	for _, scenario := range []string{"checkout-workspace", "legacy-unmarked", "different-workspace"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
			root := filepath.Dir(wt.Path)
			if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("agent output"), 0o600); err != nil {
				t.Fatal(err)
			}
			env, err := repo.GetTaskEnvironmentByTaskID(ctx, wt.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "checkout-workspace":
				env.WorkspacePath = wt.Path
			case "different-workspace":
				env.WorkspacePath = t.TempDir()
			case "legacy-unmarked":
				if err := os.Remove(filepath.Join(root, storageworkspaces.OwnershipMarkerFilename)); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
				t.Fatal(err)
			}
			archiveDirectoryTestTask(t, svc, repo, wt.TaskID, false)
			if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
				t.Fatalf("compatible checkout not reclaimed: %v", err)
			}
			_, err = os.Stat(root)
			if scenario == "checkout-workspace" {
				if !os.IsNotExist(err) {
					t.Fatalf("checkout workspace left parent directory: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unadmitted root removed: %v", err)
			}
		})
	}
}
