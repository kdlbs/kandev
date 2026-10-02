package dream

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
)

// allowedImports is every non-standard import the package may have. None of
// them is a writer of context, standing orders, notes or settings.
var allowedImports = map[string]bool{
	"github.com/google/uuid":                               true,
	"go.uber.org/zap":                                      true,
	"github.com/kandev/kandev/internal/coordinator":        true,
	"github.com/kandev/kandev/internal/coordinator/replay": true,
}

// allowedStoreMethods is every coordinator.Store method the package may call.
// Any other Store method, including every writer of context, standing orders,
// notes or settings, fails the boundary test.
var allowedStoreMethods = map[string]bool{
	"ActiveStandingOrders": true, "AcceptedDreamWithHash": true, "CountDreamDecisions": true,
	"CountDreamTurns": true, "DreamWindowDecisions": true, "DreamWindowTurns": true,
	"DreamsWithOpenEpisode": true, "ExpireStaleDreams": true, "FailDream": true, "FinishDream": true,
	"FirstLedgerTurnAt": true, "GetCoordinatorByID": true, "InsertRunningDream": true,
	"InsertSkippedDream": true, "LastAcceptedDream": true, "LastDream": true,
	"MarkDreamEpisodeArchived": true, "NthCompletedTurnFinish": true, "RefreshDream": true,
	"RunningDream": true, "SetDreamEpisodeSession": true, "SetDreamEpisodeTask": true,
	"SettleStaleReplays": true, "ShadowDreamEnabled": true,
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

func TestDreamPackageCallsOnlyAllowedStoreMethods(t *testing.T) {
	storeType := reflect.TypeOf(&coordinator.Store{})
	storeMethods := map[string]bool{}
	for i := 0; i < storeType.NumMethod(); i++ {
		storeMethods[storeType.Method(i).Name] = true
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
			if sel, ok := n.(*ast.SelectorExpr); ok && storeMethods[sel.Sel.Name] && !allowedStoreMethods[sel.Sel.Name] {
				t.Errorf("%s calls Store.%s, which is not on the dream allowlist", name, sel.Sel.Name)
			}
			return true
		})
	}
}
