package inventoryrepair

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepairIgnoresAmbientGitRepository(t *testing.T) {
	f := relocationFixture(t)
	foreign := newFixture(t)
	p := observedPlan(t, f)
	for key, value := range map[string]string{
		"GIT_DIR":                          filepath.Join(foreign.repo, ".git"),
		"GIT_COMMON_DIR":                   filepath.Join(foreign.repo, ".git"),
		"GIT_WORK_TREE":                    foreign.repo,
		"GIT_INDEX_FILE":                   filepath.Join(foreign.repo, ".git", "index"),
		"GIT_OBJECT_DIRECTORY":             filepath.Join(foreign.repo, ".git", "objects"),
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": filepath.Join(foreign.repo, ".git", "objects"),
		"GIT_NAMESPACE":                    "foreign",
		"GIT_CEILING_DIRECTORIES":          p.TasksRoot,
		"GIT_DISCOVERY_ACROSS_FILESYSTEM":  "0",
		"GIT_CONFIG_COUNT":                 "1",
		"GIT_CONFIG_KEY_0":                 "core.worktree",
		"GIT_CONFIG_VALUE_0":               foreign.repo,
	} {
		t.Setenv(key, value)
	}
	if _, err := Preview(context.Background(), p); err != nil {
		t.Fatalf("ambient Git state changed explicit inspection: %v", err)
	}
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}
