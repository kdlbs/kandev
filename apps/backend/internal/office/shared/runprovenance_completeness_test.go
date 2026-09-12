package shared_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// This test implements AC-OFFICE-BUDGET-007.3: it derives the inventory of
// every run-reason string literal in the Office source by reading the
// source with go/ast — never by matching constant identifiers, and never
// from a list of literals maintained alongside it — and fails the build
// when one is classified by neither the attended allowlist
// (AC-OFFICE-BUDGET-007.2, shared.ClassifyRunProvenance) nor the explicit
// unattended list (AC-OFFICE-BUDGET-007.4, attendedForTest/unattendedForTest
// below). It reads plain .go files with go/parser, not golang.org/x/tools/go/packages:
// alias declarations (e.g. RunReasonHeartbeat = shared.RunReasonHeartbeat) are
// recognized syntactically (RHS is not a string *ast.BasicLit) and skipped
// rather than resolved, which is sufficient because the aliased constant's
// package is itself one of the seven enumerated blocks this test scans.
//
// attendedForTest and unattendedForTest are declared here, independently of
// shared.attendedRunReasons and of each other. AC-OFFICE-BUDGET-007.4
// requires the unattended list never be computed as attendedForTest's
// complement — a derived list would make every literal a member of one set
// by construction, and the test could never fail.
var attendedForTest = map[string]bool{
	"task_assigned":               true,
	"task_comment":                true,
	"task_review_requested":       true,
	"task_changes_requested":      true,
	"task_blockers_resolved":      true,
	"task_children_completed":     true,
	"approval_resolved":           true,
	"routine_dispatch_event":      true,
	"manual_resume_after_failure": true,
	"task_mentioned":              true,
	"task_reopened":               true,
	"task_reopened_via_comment":   true,
	"task_unblocked":              true,
	"task_ready_to_close":         true,
	"stage_pending":               true,
	"stage_changes_requested":     true,
	"review_started":              true,
	"approval_started":            true,
	"blockers_resolved":           true,
	"children_completed":          true,
}

var unattendedForTest = map[string]bool{
	"agent_error":           true,
	"budget_alert":          true,
	"heartbeat":             true,
	"routine_dispatch":      true,
	"routine_dispatch_cron": true,
	"routine_trigger":       true,
}

// runReasonBlock names one of AC-OFFICE-BUDGET-007.3's seven enumerated
// declaration blocks, located by relative file path plus an anchor constant
// name already known to live in that block — never by line number.
type runReasonBlock struct {
	label      string
	relPath    string
	anchorName string
}

var runReasonBlocks = []runReasonBlock{
	{"a", "../scheduler/run.go", "RunReasonTaskAssigned"},
	{"b (RunReason*)", "../service/run.go", "RunReasonTaskAssigned"},
	{"b (legacyRunReason*)", "../service/run.go", "legacyRunReasonBlockersResolved"},
	{"c", "./runreasons.go", "RunReasonRoutineDispatch"},
	{"d", "../routing/types.go", "WakeReasonHeartbeat"},
	{"e", "../../runs/commentkeys/commentkeys.go", "TaskCommentReason"},
	{"f", "../onboarding/service.go", "runReasonTaskAssigned"},
	{"g", "../service/failure.go", "RunReasonManualResumeAfterFailure"},
}

// excludedIdentifiers is AC-OFFICE-BUDGET-007.3's exact six-entry exclusion
// list: constants that share an enumerated block with a run reason but are
// not themselves one. Exclusion is by name, never by inferring intent from
// the string value.
var excludedIdentifiers = map[string]bool{
	"RoutineSourceCron":              true, // shared.RoutineSourceCron
	"TaskCommentPrefix":              true, // commentkeys.TaskCommentPrefix
	"EngineDispatchedValue":          true, // commentkeys.EngineDispatchedValue
	"InboxKindAgentRunFailed":        true,
	"InboxKindAgentPausedAfterFails": true,
	"autoPauseReasonPrefix":          true,
}

func thisDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed; cannot locate test file for relative source paths")
	}
	return filepath.Dir(file)
}

// findConstBlock parses path and returns the *ast.GenDecl (Tok == CONST)
// containing a ValueSpec named anchor. Fails the test — never skips or
// returns a smaller inventory — when the file can't be parsed or the
// anchor isn't found, per AC-OFFICE-BUDGET-007.3's "shall fail rather than
// pass on the smaller inventory" when a block was renamed, moved, or deleted.
func findConstBlock(t *testing.T, path, anchor string) *ast.GenDecl {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name == anchor {
					return gen
				}
			}
		}
	}
	t.Fatalf("anchor constant %q not found in %s — enumerated block moved, renamed, "+
		"or deleted; AC-OFFICE-BUDGET-007.3 requires the build fail here rather than "+
		"silently shrink the run-reason inventory", anchor, path)
	return nil
}

// literalsInBlock returns every string-literal constant value declared
// directly in gen (an alias RHS — a *ast.SelectorExpr or *ast.Ident rather
// than a string *ast.BasicLit — contributes no new literal and is skipped,
// per AC-OFFICE-BUDGET-007.3), excluding any name in excludedIdentifiers.
func literalsInBlock(gen *ast.GenDecl) map[string]bool {
	out := map[string]bool{}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if excludedIdentifiers[name.Name] {
				continue
			}
			if i >= len(vs.Values) {
				continue // iota-style repeated spec with no literal of its own
			}
			lit, ok := vs.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue // alias RHS (SelectorExpr/Ident) — no new literal
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			out[val] = true
		}
	}
	return out
}

func TestRunReasonInventory_EveryLiteralIsClassified(t *testing.T) {
	dir := thisDir(t)

	if overlap := intersect(attendedForTest, unattendedForTest); len(overlap) > 0 {
		t.Fatalf("attendedForTest and unattendedForTest are not disjoint: %v", overlap)
	}

	seen := map[string][]string{} // literal -> blocks it was found in
	for _, block := range runReasonBlocks {
		path := filepath.Join(dir, block.relPath)
		gen := findConstBlock(t, path, block.anchorName)
		for literal := range literalsInBlock(gen) {
			seen[literal] = append(seen[literal], block.label)
		}
	}

	if len(seen) == 0 {
		t.Fatal("inventory came back empty — every enumerated block failed to parse " +
			"or contribute a literal; this test cannot have passed by accident")
	}

	for literal, blocks := range seen {
		inAttended := attendedForTest[literal]
		inUnattended := unattendedForTest[literal]
		if !inAttended && !inUnattended {
			t.Errorf("run-reason literal %q (declared in block(s) %v) is classified by "+
				"neither AC-OFFICE-BUDGET-007.2's attended allowlist nor "+
				"AC-OFFICE-BUDGET-007.4's unattended list — classify it in "+
				"shared.attendedRunReasons (and mirror the choice in this test's "+
				"attendedForTest/unattendedForTest) before merging", literal, blocks)
		}
		if inAttended && inUnattended {
			t.Errorf("run-reason literal %q is in both attendedForTest and "+
				"unattendedForTest — the two lists must be disjoint", literal)
		}
	}
}

func intersect(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if b[k] {
			out = append(out, k)
		}
	}
	return out
}
