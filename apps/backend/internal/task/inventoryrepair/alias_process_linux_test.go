//go:build linux

package inventoryrepair

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestAliasProcessCensusIncludesOriginalWorkspacePaths(t *testing.T) {
	f := aliasFixture(t)
	old := t.TempDir()
	execSQL(t, f.db, `UPDATE task_environments SET workspace_path=? WHERE id='env'`, old)
	f.plan.Workspaces = []WorkspaceRepair{{EnvironmentID: "env", Path: filepath.Dir(f.plan.Repairs[0].Path), SessionIDs: []string{"session"}}}
	p := observedPlan(t, f)
	if !slices.Contains(processRoots(p), old) {
		t.Fatal("process census omitted original workspace path")
	}
}
