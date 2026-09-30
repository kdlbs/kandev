package sqlguard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAnalyzeSourceFindsEveryRiskClass(t *testing.T) {
	source := `package fixture

func unsafe(db interface{ Exec(string, ...any) }) {
	db.Exec("SELECT * FROM items WHERE id = ?", "id")
	db.Exec("INSERT OR IGNORE INTO items (id) VALUES (?)", "id")
}

var schema = "CREATE TABLE items (enabled BOOLEAN DEFAULT 1, created_at DATETIME)"
var sqlite = "SELECT datetime('now') FROM sqlite_master WHERE name = 'items'"
`

	findings, err := AnalyzeSource("testdata/unsafe.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	wantRules := []Rule{
		RuleRawPlaceholder,
		RuleConflictSyntax,
		RuleBooleanInteger,
		RuleDateTimeType,
		RuleSQLiteDateFunction,
		RuleSQLiteCatalog,
	}
	for _, rule := range wantRules {
		if !hasRule(findings, rule) {
			t.Errorf("AnalyzeSource() did not report %q; findings = %#v", rule, findings)
		}
	}
}

func TestAnalyzeSourceAcceptsPortableQueries(t *testing.T) {
	source := `package fixture

func safe(db interface {
	Exec(string, ...any)
	Rebind(string) string
}) {
	db.Exec(db.Rebind("SELECT * FROM items WHERE id = ?"), "id")
}

var schema = "CREATE TABLE items (enabled INTEGER DEFAULT 0, created_at {{timestamp}})"
`
	findings, err := AnalyzeSource("testdata/safe.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("AnalyzeSource() findings = %#v, want none", findings)
	}
}

func TestTaskTransferSourcesUsePortableSQL(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
	exemptions, err := LoadExemptions(filepath.Join(backendRoot, "internal/db/sqlguard/exemptions.json"))
	if err != nil {
		t.Fatalf("LoadExemptions() error = %v", err)
	}
	for _, filename := range []string{
		"internal/task/repository/sqlite/task_transfer_profiles.go",
		"internal/task/repository/sqlite/task_transfer_relations.go",
	} {
		source, err := os.ReadFile(filepath.Join(backendRoot, filepath.FromSlash(filename)))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", filename, err)
		}
		findings, err := AnalyzeSource(filename, source, exemptions)
		if err != nil {
			t.Fatalf("AnalyzeSource(%q) error = %v", filename, err)
		}
		if len(findings) != 0 {
			t.Fatalf("AnalyzeSource(%q) findings = %#v, want none", filename, findings)
		}
	}
}

func TestAnalyzeSourceAppliesExactExemptions(t *testing.T) {
	source := `package fixture

var sqlite = "SELECT name FROM sqlite_master WHERE type = 'table'"
`
	exemptions := []Exemption{{
		File:   "internal/db/schema.go",
		Symbol: "sqlite",
		Rule:   RuleSQLiteCatalog,
		Reason: "the central schema probe is the dialect boundary",
	}}
	findings, err := AnalyzeSource("internal/db/schema.go", []byte(source), exemptions)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("AnalyzeSource() findings = %#v, want exemption", findings)
	}

	findings, err = AnalyzeSource("internal/db/other.go", []byte(source), exemptions)
	if err != nil {
		t.Fatalf("AnalyzeSource(other) error = %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("AnalyzeSource(other) findings = none, want exact-file exemption mismatch")
	}
}

func TestValidateExemptionsRejectsBroadAndDuplicateEntries(t *testing.T) {
	for _, exemptions := range [][]Exemption{
		{{File: "internal/*.go", Symbol: "x", Rule: RuleSQLiteCatalog, Reason: "too broad"}},
		{{File: "internal/x.go", Symbol: "x", Rule: "unknown", Reason: "unknown rule"}},
		{
			{File: "internal/x.go", Symbol: "x", Rule: RuleSQLiteCatalog, Reason: "one"},
			{File: "internal/x.go", Symbol: "x", Rule: RuleSQLiteCatalog, Reason: "two"},
		},
	} {
		if err := ValidateExemptions(exemptions); err == nil {
			t.Fatalf("ValidateExemptions(%#v) error = nil", exemptions)
		}
	}
}

func hasRule(findings []Finding, want Rule) bool {
	for _, finding := range findings {
		if finding.Rule == want {
			return true
		}
	}
	return false
}

func TestAnalyzeSourceReportsUsefulLocations(t *testing.T) {
	findings, err := AnalyzeSource("fixture.go", []byte("package p\nfunc f(db interface{ Exec(string, ...any) }) { db.Exec(\"SELECT ?\") }\n"), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if len(findings) == 0 || findings[0].Line == 0 || !strings.Contains(findings[0].Message, "placeholder") {
		t.Fatalf("finding = %#v, want line and placeholder message", findings)
	}
}

func TestAnalyzeSourceFindsStandalonePragma(t *testing.T) {
	source := "package p\nfunc unsafe(db interface{ Exec(string, ...any) }) { db.Exec(\"PRAGMA table_info(items)\") }\n"
	findings, err := AnalyzeSource("fixture.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if !hasRule(findings, RuleSQLiteCatalog) {
		t.Fatalf("AnalyzeSource() findings = %#v, want sqlite-catalog finding", findings)
	}
}

func TestAnalyzeSourceFindsAssignedRawPlaceholder(t *testing.T) {
	source := `package fixture
func unsafe(db interface{ Exec(string, ...any) }) {
	query := "SELECT value FROM settings WHERE key = ?"
	db.Exec(query)
}
`
	findings, err := AnalyzeSource("fixture.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if !hasRule(findings, RuleRawPlaceholder) {
		t.Fatalf("AnalyzeSource() findings = %#v, want assigned raw-placeholder finding", findings)
	}
}

func TestAnalyzeSourceRequiresRebindAfterSQLxIn(t *testing.T) {
	unsafeSource := `package fixture
func unsafe(db interface{ Exec(string, ...any) }) {
	db.Exec(sqlx.In("SELECT value FROM settings WHERE key IN (?)", []string{"x"}))
}
`
	findings, err := AnalyzeSource("fixture.go", []byte(unsafeSource), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource(unsafe) error = %v", err)
	}
	if !hasRule(findings, RuleRawPlaceholder) {
		t.Fatalf("AnalyzeSource(unsafe) findings = %#v, want raw-placeholder finding", findings)
	}

	safeSource := `package fixture
func safe(db interface{ Exec(string, ...any) }) {
	db.Exec(db.Rebind(sqlx.In("SELECT value FROM settings WHERE key IN (?)", []string{"x"})))
}
`
	findings, err = AnalyzeSource("fixture.go", []byte(safeSource), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource(safe) error = %v", err)
	}
	if hasRule(findings, RuleRawPlaceholder) {
		t.Fatalf("AnalyzeSource(safe) findings = %#v, want no raw-placeholder finding", findings)
	}

	assignedSafeSource := `package fixture
func safeAssigned(db interface{ Exec(string, ...any) }) {
	query, args, err := sqlx.In("SELECT value FROM settings WHERE key IN (?)", []string{"x"})
	if err != nil { return }
	db.Exec(db.Rebind(query), args...)
}
`
	findings, err = AnalyzeSource("fixture.go", []byte(assignedSafeSource), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource(assigned safe) error = %v", err)
	}
	if hasRule(findings, RuleRawPlaceholder) {
		t.Fatalf("AnalyzeSource(assigned safe) findings = %#v, want no raw-placeholder finding", findings)
	}
}

func TestAnalyzeSourceFindsBooleanIntegerComparison(t *testing.T) {
	source := `package fixture
var query = "UPDATE settings SET enabled = 1 WHERE key = 'x'"
`
	findings, err := AnalyzeSource("fixture.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if !hasRule(findings, RuleBooleanInteger) {
		t.Fatalf("AnalyzeSource() findings = %#v, want boolean-integer comparison finding", findings)
	}
}

func TestAnalyzeSourceIgnoresPortableAndNonBooleanComparisons(t *testing.T) {
	source := `package fixture
var portable = "UPDATE settings SET enabled = TRUE WHERE status = 1"
var numeric = "UPDATE settings SET retry_count = 1 WHERE status = 0"
`
	findings, err := AnalyzeSource("fixture.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if hasRule(findings, RuleBooleanInteger) {
		t.Fatalf("AnalyzeSource() findings = %#v, want no boolean-integer finding", findings)
	}
}

func TestAnalyzeSourceFindsKnownBooleanColumnComparison(t *testing.T) {
	source := `package fixture
var query = "SELECT id FROM settings WHERE is_enabled != 0"
`
	findings, err := AnalyzeSource("fixture.go", []byte(source), nil)
	if err != nil {
		t.Fatalf("AnalyzeSource() error = %v", err)
	}
	if !hasRule(findings, RuleBooleanInteger) {
		t.Fatalf("AnalyzeSource() findings = %#v, want boolean-integer comparison finding", findings)
	}
}
