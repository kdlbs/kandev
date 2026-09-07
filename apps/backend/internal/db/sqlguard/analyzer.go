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
	RuleBooleanInteger:     "integer literal for a SQL boolean column",
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
	// These names are the boolean columns used by the built-in SQL schemas.
	// The list is deliberately explicit: fields such as status, priority, and
	// is_ephemeral are integer state values, not SQL BOOLEAN columns.
	booleanComparisonPattern = regexp.MustCompile(`(?i)\b(?:enabled|is_enabled|active|is_active|hidden|is_hidden|deleted|is_deleted|builtin|is_builtin|installed|is_installed|last_ok|poll_enabled|user_modified|cli_passthrough|auto_approve)\b\s*(?:<>|!=|<=|>=|=|<|>)\s*[01]\b`)
	sqliteDatePattern        = regexp.MustCompile(`(?i)\b(?:date|datetime|strftime|julianday)\s*\(`)
	datetimeTypePattern      = regexp.MustCompile(`(?i)(?:^|[\s(,])DATETIME(?:\s|[,);]|$)`)
	pragmaPattern            = regexp.MustCompile(`(?i)\bpragma(?:_[a-z0-9_]+)?\b`)
	sqlPattern               = regexp.MustCompile(`(?i)\b(?:SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|WITH)\b`)
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
	values := topLevelStringValues(file)
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
		{RuleBooleanInteger, sqlText && (booleanIntegerPattern.MatchString(value) || booleanComparisonPattern.MatchString(value)), "use a boolean value or a dialect-rendered boolean default"},
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
	values map[string]queryValue,
	exemptions []Exemption,
	result *analysisResult,
) {
	seen := make(map[string]struct{})
	scopes := make([]functionScope, 0, len(file.Decls))
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		scopes = append(scopes, functionScope{body: function.Body, params: function.Type.Params})
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok {
			scopes = append(scopes, functionScope{body: literal.Body, params: literal.Type.Params})
			return false
		}
		return true
	})
	for _, scope := range scopes {
		functionValues := cloneQueryValues(values)
		for name := range localStringNames(scope.body) {
			delete(functionValues, name)
		}
		for _, field := range scope.params.List {
			for _, name := range field.Names {
				delete(functionValues, name.Name)
			}
		}
		ast.Inspect(scope.body, func(node ast.Node) bool {
			// A function literal introduces its own parameter and assignment
			// scope. It is analyzed separately when it is a top-level value;
			// treating its parameters as the enclosing function's values would
			// make an unrelated package-level query look executable here.
			if literal, ok := node.(*ast.FuncLit); ok {
				_ = literal
				return false
			}
			switch node := node.(type) {
			case *ast.ValueSpec:
				recordStringValueSpec(node, functionValues)
			case *ast.AssignStmt:
				recordStringAssignments(node, functionValues)
			case *ast.RangeStmt:
				analyzeRangeSQLValues(node, functionValues)
			case *ast.CallExpr:
				analyzeExecutorCall(filename, node, fileSet, parents, functionValues, exemptions, seen, result)
			}
			return true
		})
	}
}

type functionScope struct {
	body   *ast.BlockStmt
	params *ast.FieldList
}

func analyzeRangeSQLValues(
	rangeStmt *ast.RangeStmt,
	values map[string]queryValue,
) {
	valueName, ok := rangeStmt.Value.(*ast.Ident)
	if !ok {
		return
	}
	composite, ok := rangeStmt.X.(*ast.CompositeLit)
	if !ok {
		return
	}
	// Keep one unsafe representative for the loop variable. The eventual
	// executor call still decides whether that value is wrapped by Rebind, so
	// a loop over raw query strings does not produce a false positive when the
	// body correctly rebinds each query.
	for _, element := range composite.Elts {
		query, ok := resolveQueryValue(element, values)
		if ok && !query.bound && rawPlaceholder(query.value) {
			query.expression = element
			values[valueName.Name] = query
			return
		}
	}
}

func analyzeExecutorCall(
	filename string,
	call *ast.CallExpr,
	fileSet *token.FileSet,
	parents map[ast.Node]ast.Node,
	values map[string]queryValue,
	exemptions []Exemption,
	seen map[string]struct{},
	result *analysisResult,
) {
	if !isExecutorCall(call) {
		return
	}
	query, value, bound, ok := queryArgument(call, values)
	if !ok || bound || isBoundQuery(query) {
		return
	}
	if pragmaPattern.MatchString(value) {
		position := fileSet.Position(query.Pos())
		symbol := sourceSymbol(query, parents)
		analyzeSQLLiteral(filename, value, symbol, position, exemptions, seen, result, true)
	}
	if rawPlaceholder(value) {
		analyzeRawPlaceholder(filename, query, fileSet, parents, exemptions, seen, result)
	}
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

type queryValue struct {
	expression ast.Expr
	value      string
	bound      bool
}

func topLevelStringValues(file *ast.File) map[string]queryValue {
	values := make(map[string]queryValue)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range general.Specs {
			if valueSpec, ok := specification.(*ast.ValueSpec); ok {
				recordStringValueSpec(valueSpec, values)
			}
		}
	}
	return values
}

func cloneQueryValues(values map[string]queryValue) map[string]queryValue {
	clone := make(map[string]queryValue, len(values))
	for name, value := range values {
		clone[name] = value
	}
	return clone
}

func localStringNames(root ast.Node) map[string]struct{} {
	names := make(map[string]struct{})
	ast.Inspect(root, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		switch node := node.(type) {
		case *ast.ValueSpec:
			for _, name := range node.Names {
				names[name.Name] = struct{}{}
			}
		case *ast.AssignStmt:
			for _, target := range node.Lhs {
				if name, ok := target.(*ast.Ident); ok {
					names[name.Name] = struct{}{}
				}
			}
		}
		return true
	})
	return names
}

func recordStringValueSpec(specification *ast.ValueSpec, values map[string]queryValue) {
	for index, name := range specification.Names {
		if index >= len(specification.Values) {
			continue
		}
		if value, ok := resolveQueryValue(specification.Values[index], values); ok {
			value.expression = specification.Values[index]
			values[name.Name] = value
		}
	}
}

func recordStringAssignments(assignment *ast.AssignStmt, values map[string]queryValue) {
	for index, target := range assignment.Lhs {
		name, ok := target.(*ast.Ident)
		if !ok || index >= len(assignment.Rhs) {
			continue
		}
		if value, ok := resolveQueryValue(assignment.Rhs[index], values); ok {
			value.expression = assignment.Rhs[index]
			values[name.Name] = value
		}
	}
}

func resolveQueryValue(expression ast.Expr, values map[string]queryValue) (queryValue, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		return resolveBasicLiteral(expression)
	case *ast.Ident:
		return resolveIdentifier(expression, values)
	case *ast.BinaryExpr:
		return resolveBinaryQueryValue(expression, values)
	case *ast.ParenExpr:
		return resolveParenthesizedQueryValue(expression, values)
	case *ast.CallExpr:
		return resolveCallQueryValue(expression, values)
	default:
		return queryValue{}, false
	}
}

func resolveBasicLiteral(expression *ast.BasicLit) (queryValue, bool) {
	if expression.Kind != token.STRING {
		return queryValue{}, false
	}
	value, err := strconv.Unquote(expression.Value)
	return queryValue{expression: expression, value: value}, err == nil
}

func resolveIdentifier(expression *ast.Ident, values map[string]queryValue) (queryValue, bool) {
	value, ok := values[expression.Name]
	if ok {
		value.expression = expression
	}
	return value, ok
}

func resolveBinaryQueryValue(expression *ast.BinaryExpr, values map[string]queryValue) (queryValue, bool) {
	if expression.Op != token.ADD {
		return queryValue{}, false
	}
	left, leftOK := resolveQueryValue(expression.X, values)
	right, rightOK := resolveQueryValue(expression.Y, values)
	if !leftOK || !rightOK {
		return queryValue{}, false
	}
	return queryValue{expression: expression, value: left.value + right.value, bound: left.bound && right.bound}, true
}

func resolveParenthesizedQueryValue(expression *ast.ParenExpr, values map[string]queryValue) (queryValue, bool) {
	value, ok := resolveQueryValue(expression.X, values)
	if ok {
		value.expression = expression
	}
	return value, ok
}

func resolveCallQueryValue(expression *ast.CallExpr, values map[string]queryValue) (queryValue, bool) {
	if len(expression.Args) == 0 {
		return queryValue{}, false
	}
	value, ok := resolveQueryValue(expression.Args[0], values)
	if !ok {
		return queryValue{}, false
	}
	switch functionName(expression.Fun) {
	case "Rebind":
		value.bound = true
	case "In":
		// sqlx.In expands '?' placeholders but does not rebind them to
		// the target driver's syntax. It is safe only when wrapped by
		// Rebind at the final database boundary.
		value.bound = false
	default:
		return queryValue{}, false
	}
	value.expression = expression
	return value, true
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

func queryArgument(call *ast.CallExpr, values map[string]queryValue) (ast.Expr, string, bool, bool) {
	for _, argument := range call.Args {
		resolved, ok := resolveQueryValue(argument, values)
		if ok && (sqlPattern.MatchString(resolved.value) || pragmaPattern.MatchString(resolved.value)) {
			return resolved.expression, resolved.value, resolved.bound, true
		}
	}
	return nil, "", false, false
}

func isBoundQuery(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	name := functionName(call.Fun)
	return name == "Rebind"
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
