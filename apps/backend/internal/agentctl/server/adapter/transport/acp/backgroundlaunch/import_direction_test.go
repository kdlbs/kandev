package backgroundlaunch_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// AC-69(b): the recogniser registry package must stay a leaf that imports
// nothing from the probe, the parked projection, task/orchestrator code, or
// the frontend. That is the structural half of "register a new agent,
// nothing else changes" (D7) — a registry that reached into its own
// consumers would make every future vendor recogniser depend on whatever
// those consumers happen to import. TestStampBackgroundShellWork_* in the
// acp package covers the observable half.
func TestPackageImports_ExcludeProbeProjectionOrchestratorAndFrontend(t *testing.T) {
	forbidden := []string{
		"internal/orchestrator",
		"internal/task",
		"/probe",
		"apps/web",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob source files: %v", err)
	}

	var productionFiles []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			productionFiles = append(productionFiles, f)
		}
	}
	if len(productionFiles) == 0 {
		t.Fatalf("no production source files found in backgroundlaunch package")
	}

	fset := token.NewFileSet()
	for _, file := range productionFiles {
		f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, forbid := range forbidden {
				if strings.Contains(path, forbid) {
					t.Errorf("%s imports %q, which contains forbidden path segment %q — the registry must stay a leaf package", file, path, forbid)
				}
			}
		}
	}
}
