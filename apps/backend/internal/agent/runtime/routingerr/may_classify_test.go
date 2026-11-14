package routingerr

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const defaultPromptRule = "phase.poststart.unknown"

func TestMayClassifyGateCoversEveryRule(t *testing.T) {
	var uncovered []string
	check := func(id string, tokens []string) {
		if len(tokens) == 0 {
			uncovered = append(uncovered, id)
		}
	}
	for _, rules := range providerRules {
		for _, r := range rules {
			check(r.id, ruleGateTokens(r.id, r.pattern))
		}
	}
	for _, r := range providerNeutralRules {
		check(r.id, ruleGateTokens(r.id, r.pattern))
	}
	for _, r := range runtimeEnvironmentRules {
		pattern := r.pattern
		if r.match != nil {
			pattern = nil
		}
		check(r.id, ruleGateTokens(r.id, pattern))
	}
	sort.Strings(uncovered)
	if len(uncovered) > 0 || !gateEnabled {
		t.Fatalf("rules without a provable gate token %v (gate enabled = %v); declare them in gateTokensByRuleID",
			uncovered, gateEnabled)
	}
	if len(gateTokens) == 0 {
		t.Fatal("gate has no tokens")
	}
}

func ruleGateTokens(id string, pattern *regexp.Regexp) []string {
	seen := map[string]struct{}{}
	if !collectRuleTokens(seen, id, pattern) {
		return nil
	}
	tokens := make([]string, 0, len(seen))
	for token := range seen {
		tokens = append(tokens, token)
	}
	return tokens
}

func mustRegexp(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compile %q: %v", pattern, err)
	}
	return re
}

// TestMayClassifyNeverHidesAFixtureMatch runs every string fixture of this
// package through Classify for every provider. Whenever Classify returns
// anything but the default, MayClassify must have said so.
func TestMayClassifyNeverHidesAFixtureMatch(t *testing.T) {
	samples := append(packageStringFixtures(t), extraGateSamples...)
	providers := []string{""}
	for id := range providerRules {
		providers = append(providers, id)
	}
	providers = append(providers, cursorRetriableProviderID)
	matchedRules := map[string]bool{}
	for _, sample := range samples {
		for _, provider := range providers {
			for _, in := range []Input{
				{Phase: PhasePromptSend, ProviderID: provider, Stderr: sample},
				{Phase: PhasePromptSend, ProviderID: provider, Stdout: sample},
			} {
				got := Classify(in)
				if got.ClassifierRule == defaultPromptRule {
					continue
				}
				matchedRules[got.ClassifierRule] = true
				if !MayClassify(in) {
					t.Errorf("MayClassify = false for %q (provider %q), Classify matched %s",
						sample, provider, got.ClassifierRule)
				}
			}
		}
	}
	var unmatched []string
	for _, id := range catalogueRuleIDs() {
		if !matchedRules[id] {
			unmatched = append(unmatched, id)
		}
	}
	if len(unmatched) > 0 {
		t.Fatalf("no fixture exercises rules %v; add a sample to extraGateSamples", unmatched)
	}
}

// extraGateSamples exercises catalogue rules that no other fixture in this
// package reaches through Classify.
var extraGateSamples = []string{
	AntigravityAuthMessage,
	AntigravitySubscriptionMessage,
	AntigravityModelAccessMessage,
	AntigravityQuotaMessage,
	AntigravityThrottleMessage,
	AntigravityProviderMessage,
	AntigravityProviderOverloadedMessage,
	AntigravityUnknownMessage,
	AntigravityOutcomeUnverifiedMessage,
	"bash: claude: command not found",
	"error: model_not_found",
	"npm error code ENOTEMPTY\nnpm error path /tmp/x/.npm/_npx/0a1b2c3d/node_modules",
}

func catalogueRuleIDs() []string {
	var ids []string
	for _, rules := range providerRules {
		for _, r := range rules {
			ids = append(ids, r.id)
		}
	}
	for _, r := range providerNeutralRules {
		ids = append(ids, r.id)
	}
	for _, r := range runtimeEnvironmentRules {
		ids = append(ids, r.id)
	}
	sort.Strings(ids)
	return ids
}

func TestMayClassifyRejectsOrdinaryProse(t *testing.T) {
	for _, text := range []string{
		"",
		"I'll start by reading the repository layout.",
		"Done. The tests pass and the change is ready for review.",
		"Here is the updated function:",
	} {
		if MayClassify(Input{Phase: PhasePromptSend, ProviderID: providerClaudeACP, Stderr: text}) {
			t.Errorf("MayClassify(%q) = true, want false", text)
		}
	}
}

func TestMayClassifyKeepsStructuredEvidence(t *testing.T) {
	exit := 127
	for name, in := range map[string]Input{
		"structured error": {StructuredErr: errors.New("boom")},
		"http status":      {HTTPStatus: 429},
		"acp code":         {ACPCode: -32000},
		"error kind":       {ProviderID: providerClaudeACP, ErrorKind: "overloaded_error"},
		"provider code":    {ProviderCode: "x"},
		"exit code":        {ExitCode: &exit},
		"managed runtime":  {ManagedRuntimePackageSpec: "pkg@1.0.0"},
		"keyword context":  {KeywordContext: KeywordContext{Enabled: true, Diagnostic: true}},
	} {
		in.Phase = PhasePromptSend
		in.Stderr = "plain words"
		if !MayClassify(in) {
			t.Errorf("%s: MayClassify = false, want true", name)
		}
	}
}

func TestMayClassifyFoldsLongS(t *testing.T) {
	token := ""
	for _, candidate := range gateTokens {
		if strings.Contains(candidate, "s") {
			token = candidate
			break
		}
	}
	if token == "" {
		t.Skip("no gate token contains an s")
	}
	text := strings.ReplaceAll(strings.ToUpper(token), "S", "ſ")
	if !MayClassify(Input{Phase: PhasePromptSend, Stderr: "x " + text + " y"}) {
		t.Fatalf("MayClassify rejected a case-folded token %q", text)
	}
}

func TestPatternGateTokensRequireEveryPath(t *testing.T) {
	cases := map[string][]string{
		`x(?:verylongtoken)?y`:         nil,
		`ab|verylongtoken`:             nil,
		`(?i)invalid|incorrect`:        {"correct", "valid"},
		`quota\s+exceeded`:             {"exceeded"},
		`(?:rate|usage)\s+limit(?:ed)`: {"limited"},
		`token{1,}s?`:                  {"toke"},
		`(?:session){0,1}\s+expired`:   {"expired"},
		`a+b*`:                         nil,
	}
	for pattern, want := range cases {
		got := patternGateTokens(mustRegexp(t, pattern))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("patternGateTokens(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func packageStringFixtures(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	seen := map[string]struct{}{}
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			expr, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			if value, ok := foldStringConstant(expr); ok && value != "" {
				seen[value] = struct{}{}
			}
			return true
		})
	}
	samples := make([]string, 0, len(seen))
	for sample := range seen {
		samples = append(samples, sample)
	}
	sort.Strings(samples)
	return samples
}

func foldStringConstant(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(e.Value)
		return value, err == nil
	case *ast.ParenExpr:
		return foldStringConstant(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := foldStringConstant(e.X)
		if !ok {
			return "", false
		}
		right, ok := foldStringConstant(e.Y)
		return left + right, ok
	}
	return "", false
}
