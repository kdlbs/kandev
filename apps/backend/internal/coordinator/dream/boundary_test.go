package dream

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedImports is every non-standard import the package may have. None of
// them is a writer of context, standing orders, notes or settings.
var allowedImports = map[string]bool{
	"github.com/google/uuid":                               true,
	"go.uber.org/zap":                                      true,
	"github.com/kandev/kandev/internal/coordinator":        true,
	"github.com/kandev/kandev/internal/coordinator/replay": true,
}

// storeWriters are the coordinator store and service methods that change what
// a turn reads; a dream must never call one.
var storeWriters = []string{
	"UpdateCoordinator", "UpdateContext", "SetContext", "CreateStandingOrder", "RetireStandingOrder",
	"UpdateStandingOrder", "AddNote", "RetireNote", "UpdateNote", "SetAutonomy", "SetPaused",
	"InsertProposal", "InsertPendingChange", "SetShadowDreamEnabled", "CreateCoordinator", "DeleteCoordinator",
}

func productionFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	return out
}

func TestDreamPackageImportsNoWriterOfWhatATurnReads(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
				continue
			}
			if !allowedImports[path] {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}

func TestDreamPackageCallsNoStoreWriterOfWhatATurnReads(t *testing.T) {
	banned := map[string]bool{}
	for _, m := range storeWriters {
		banned[m] = true
	}
	fset := token.NewFileSet()
	for _, name := range productionFiles(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && banned[sel.Sel.Name] {
				t.Errorf("%s calls %s", name, sel.Sel.Name)
			}
			return true
		})
	}
}
