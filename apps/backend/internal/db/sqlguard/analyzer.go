// Package sqlguard provides a small AST-based check for SQL constructs that
// are unsafe across Kandev's SQLite and PostgreSQL engines.
package sqlguard

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Rule identifies one SQL portability rule.
type Rule string

const (
	RuleSQLiteCatalog      Rule = "sqlite-catalog"
	RuleConflictSyntax     Rule = "conflict-syntax"
	RuleRawPlaceholder     Rule = "raw-placeholder"
	RuleBooleanInteger     Rule = "boolean-integer"
	RuleSQLiteDateFunction Rule = "sqlite-date-function"
	RuleDateTimeType       Rule = "datetime-type"
)

var knownRules = map[Rule]string{
	RuleSQLiteCatalog:      "SQLite catalog or PRAGMA syntax",
	RuleConflictSyntax:     "SQLite conflict syntax",
	RuleRawPlaceholder:     "raw question-mark placeholder",
	RuleBooleanInteger:     "integer default for a BOOLEAN column",
	RuleSQLiteDateFunction: "SQLite-only date function",
	RuleDateTimeType:       "direct DATETIME type",
}

// Exemption allows one exact source file, symbol, and rule to use an
// intentional SQLite-only construct.
type Exemption struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
	Rule   Rule   `json:"rule"`
	Reason string `json:"reason"`
}

// Finding is one portability violation.
type Finding struct {
	File    string
	Line    int
	Column  int
	Symbol  string
	Rule    Rule
	Message string
}

var (
	sqliteCatalogPattern  = regexp.MustCompile(`(?i)\b(?:sqlite_master|sqlite_schema|pragma(?:_[a-z0-9_]+)?)\b`)
	conflictPattern       = regexp.MustCompile(`(?i)\bINSERT\s+OR\s+IGNORE\b`)
	booleanIntegerPattern = regexp.MustCompile(`(?is)\bBOOLEAN\b.{0,100}?\bDEFAULT\s+[01]\b`)
	sqliteDatePattern     = regexp.MustCompile(`(?i)\b(?:date|datetime|strftime|julianday)\s*\(`)
	datetimeTypePattern   = regexp.MustCompile(`(?i)(?:^|[\s(,])DATETIME(?:\s|[,);]|$)`)
	pragmaPattern         = regexp.MustCompile(`(?i)\bpragma(?:_[a-z0-9_]+)?\b`)
	sqlPattern            = regexp.MustCompile(`(?i)\b(?:SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|WITH)\b`)
)

type analysisResult struct {
	findings []Finding
	used     map[string]struct{}
}

// AnalyzeSource checks one Go source file. filename participates in exact
// exemption matching and should be the repository-relative path used by the
// caller.
func AnalyzeSource(filename string, source []byte, exemptions []Exemption) ([]Finding, error) {
	if err := ValidateExemptions(exemptions); err != nil {
		return nil, err
	}
	result, err := analyzeSource(filename, source, exemptions)
	if err != nil {
		return nil, err
	}
	return result.findings, nil
}

func analyzeSource(filename string, source []byte, exemptions []Exemption) (analysisResult, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filename, source, 0)
	if err != nil {
		return analysisResult{}, fmt.Errorf("parse %s: %w", filename, err)
	}
	parents := parentNodes(file)
	values := stringValues(file)
	result := analysisResult{used: make(map[string]struct{})}
	analyzeStringLiterals(filename, file, fileSet, parents, exemptions, &result)
	analyzeExecutorCalls(filename, file, fileSet, parents, values, exemptions, &result)
	return result, nil
}

func analyzeStringLiterals(
	filename string,
	file *ast.File,
	fileSet *token.FileSet,
	parents map[ast.Node]ast.Node,
	exemptions []Exemption,
	result *analysisResult,
) {
	seen := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		symbol := sourceSymbol(literal, parents)
		analyzeSQLLiteral(filename, value, symbol, fileSet.Position(literal.Pos()), exemptions, seen, result)
		return true
	})
}

func analyzeSQLLiteral(
	filename, value, symbol string,
	position token.Position,
	exemptions []Exemption,
	seen map[string]struct{},
	result *analysisResult,
	forceSQL ...bool,
) {
	sqlText := sqlPattern.MatchString(value)
	if len(forceSQL) > 0 && forceSQL[0] {
		sqlText = true
	}
	checks := []struct {
		rule    Rule
		match   bool
		message string
	}{
		{RuleSQLiteCatalog, sqlText && sqliteCatalogPattern.MatchString(value), "SQLite catalog or PRAGMA syntax must stay behind a dialect boundary"},
		{RuleConflictSyntax, sqlText && conflictPattern.MatchString(value), "use portable conflict syntax"},
		{RuleBooleanInteger, sqlText && booleanIntegerPattern.MatchString(value), "use a dialect-rendered boolean default"},
		{RuleSQLiteDateFunction, sqlText && sqliteDatePattern.MatchString(value), "use an internal/db/dialect date helper"},
		{RuleDateTimeType, sqlText && datetimeTypePattern.MatchString(value), "use a dialect-rendered timestamp type"},
	}
	for _, check := range checks {
		if !check.match {
			continue
		}
		key := findingKey(filename, symbol, check.rule, position.Line, position.Column)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if matchesExemption(filename, symbol, check.rule, exemptions) {
			result.used[exemptionKey(Exemption{File: filename, Symbol: symbol, Rule: check.rule})] = struct{}{}
			continue
		}
		result.findings = append(result.findings, Finding{
			File: position.Filename, Line: position.Line, Column: position.Column,
			Symbol: symbol, Rule: check.rule, Message: check.message,
		})
	}
}

func analyzeExecutorCalls(
	filename string,
	file *ast.File,
	fileSet *token.FileSet,
	parents map[ast.Node]ast.Node,
	values map[string]string,
	exemptions []Exemption,
	result *analysisResult,
) {
	seen := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isExecutorCall(call) {
			return true
		}
		query, value, ok := queryArgument(call, values)
		if !ok || isBoundQuery(query) {
			return true
		}
		if pragmaPattern.MatchString(value) {
			position := fileSet.Position(query.Pos())
			symbol := sourceSymbol(query, parents)
			analyzeSQLLiteral(filename, value, symbol, position, exemptions, seen, result, true)
		}
		if !rawPlaceholder(value) {
			return true
		}
		analyzeRawPlaceholder(filename, query, fileSet, parents, exemptions, seen, result)
		return true
	})
}

func analyzeRawPlaceholder(
	filename string,
	query ast.Expr,
	fileSet *token.FileSet,
	parents map[ast.Node]ast.Node,
	exemptions []Exemption,
	seen map[string]struct{},
	result *analysisResult,
) {
	position := fileSet.Position(query.Pos())
	symbol := sourceSymbol(query, parents)
	key := findingKey(filename, symbol, RuleRawPlaceholder, position.Line, position.Column)
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	if matchesExemption(filename, symbol, RuleRawPlaceholder, exemptions) {
		result.used[exemptionKey(Exemption{File: filename, Symbol: symbol, Rule: RuleRawPlaceholder})] = struct{}{}
		return
	}
	result.findings = append(result.findings, Finding{
		File: position.Filename, Line: position.Line, Column: position.Column,
		Symbol: symbol, Rule: RuleRawPlaceholder,
		Message: "rebind SQL placeholders at the final database boundary",
	})
}

// CheckFiles checks a fixed set of Go files and rejects exemptions that did not
// match a finding. Directory traversal belongs to the command, so callers
// cannot accidentally create an implicit directory-wide exemption.
func CheckFiles(files []string, exemptions []Exemption) ([]Finding, error) {
	if err := ValidateExemptions(exemptions); err != nil {
		return nil, err
	}
	findings := make([]Finding, 0)
	used := make(map[string]struct{})
	for _, filename := range files {
		source, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filename, err)
		}
		result, err := analyzeSource(filepath.ToSlash(filename), source, exemptions)
		if err != nil {
			return nil, err
		}
		findings = append(findings, result.findings...)
		for key := range result.used {
			used[key] = struct{}{}
		}
	}
	for _, exemption := range exemptions {
		if _, ok := used[exemptionKey(exemption)]; !ok {
			return nil, fmt.Errorf("unused SQL guard exemption %s", exemptionKey(exemption))
		}
	}
	return findings, nil
}

// ValidateExemptions rejects broad paths, unknown rules, missing reasons, and
// duplicate exact entries.
func ValidateExemptions(exemptions []Exemption) error {
	seen := make(map[string]struct{}, len(exemptions))
	for _, exemption := range exemptions {
		if exemption.File == "" || filepath.IsAbs(exemption.File) || strings.ContainsAny(exemption.File, "*?") || strings.Contains(exemption.File, "..") {
			return fmt.Errorf("SQL guard exemption file must be an exact relative path: %q", exemption.File)
		}
		if strings.Contains(exemption.File, "\\") || exemption.Symbol == "" || strings.ContainsAny(exemption.Symbol, "*?") {
			return fmt.Errorf("SQL guard exemption must have an exact file and symbol: %q", exemption.File)
		}
		if _, ok := knownRules[exemption.Rule]; !ok {
			return fmt.Errorf("SQL guard exemption has unknown rule %q", exemption.Rule)
		}
		if strings.TrimSpace(exemption.Reason) == "" {
			return fmt.Errorf("SQL guard exemption %s has no reason", exemptionKey(exemption))
		}
		key := exemptionKey(exemption)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate SQL guard exemption %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// LoadExemptions loads the exact exemption registry from JSON.
func LoadExemptions(filename string) ([]Exemption, error) {
	source, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read SQL guard exemptions: %w", err)
	}
	var document struct {
		Exemptions []Exemption `json:"exemptions"`
	}
	if err := json.Unmarshal(source, &document); err != nil {
		return nil, fmt.Errorf("parse SQL guard exemptions: %w", err)
	}
	if err := ValidateExemptions(document.Exemptions); err != nil {
		return nil, err
	}
	return document.Exemptions, nil
}

func parentNodes(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var walk func(ast.Node)
	walk = func(parent ast.Node) {
		ast.Inspect(parent, func(node ast.Node) bool {
			if node == nil || node == parent {
				return true
			}
			parents[node] = parent
			walk(node)
			return false
		})
	}
	walk(root)
	return parents
}

func stringValues(file *ast.File) map[string]string {
	values := make(map[string]string)
	ast.Inspect(file, func(node ast.Node) bool {
		valueSpec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for index, name := range valueSpec.Names {
			if index >= len(valueSpec.Values) {
				continue
			}
			if value, ok := literalValue(valueSpec.Values[index], values); ok {
				values[name.Name] = value
			}
		}
		return true
	})
	return values
}

func literalValue(expression ast.Expr, values map[string]string) (string, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expression.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := values[expression.Name]
		return value, ok
	case *ast.BinaryExpr:
		if expression.Op != token.ADD {
			return "", false
		}
		left, leftOK := literalValue(expression.X, values)
		right, rightOK := literalValue(expression.Y, values)
		return left + right, leftOK && rightOK
	case *ast.ParenExpr:
		return literalValue(expression.X, values)
	default:
		return "", false
	}
}

func sourceSymbol(node ast.Node, parents map[ast.Node]ast.Node) string {
	for current := node; current != nil; current = parents[current] {
		switch current := current.(type) {
		case *ast.FuncDecl:
			return current.Name.Name
		case *ast.ValueSpec:
			for _, name := range current.Names {
				return name.Name
			}
		}
	}
	return "file"
}

func isExecutorCall(call *ast.CallExpr) bool {
	switch functionName(call.Fun) {
	case "Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Get", "Select", "NamedExec", "Prepare", "PrepareContext":
		return true
	default:
		return false
	}
}

func queryArgument(call *ast.CallExpr, values map[string]string) (ast.Expr, string, bool) {
	for _, argument := range call.Args {
		value, ok := literalValue(argument, values)
		if ok && (sqlPattern.MatchString(value) || pragmaPattern.MatchString(value)) {
			return argument, value, true
		}
	}
	return nil, "", false
}

func isBoundQuery(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	name := functionName(call.Fun)
	return name == "Rebind" || name == "In"
}

func functionName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expression.Sel.Name
	default:
		return ""
	}
}

func rawPlaceholder(value string) bool {
	return strings.Contains(value, "?") && sqlPattern.MatchString(value)
}

func matchesExemption(filename, symbol string, rule Rule, exemptions []Exemption) bool {
	for _, exemption := range exemptions {
		if exemption.File == filename && exemption.Symbol == symbol && exemption.Rule == rule {
			return true
		}
	}
	return false
}

func exemptionKey(exemption Exemption) string {
	return filepath.ToSlash(exemption.File) + ":" + exemption.Symbol + ":" + string(exemption.Rule)
}

func findingKey(filename, symbol string, rule Rule, line, column int) string {
	return fmt.Sprintf("%s:%s:%s:%d:%d", filename, symbol, rule, line, column)
}
