package sqlguard

import (
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
