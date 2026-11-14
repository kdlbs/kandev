package routingerr

import (
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"unicode"
)

// MayClassify reports whether Classify could return anything other than its
// default, unmatched classification for this input, without running the rule
// engine. A false result guarantees that Classify returns its phase default;
// a true result only means the rules still have to run.
//
// Structured evidence is never gated: a structured error, an HTTP or ACP
// status, an error kind or provider code, an exit code, a managed runtime
// package, enabled keyword context, or an injected provider failure makes the
// answer true regardless of text.
func MayClassify(in Input) bool {
	if in.StructuredErr != nil || in.HTTPStatus != 0 || in.ACPCode != 0 ||
		in.ErrorKind != "" || in.ProviderCode != "" || in.ExitCode != nil ||
		in.ManagedRuntimePackageSpec != "" || in.KeywordContext.Enabled {
		return true
	}
	if inj := getInjection(); inj != nil {
		if _, ok := inj[in.ProviderID]; ok {
			return true
		}
	}
	return mayMatchText(in.Stderr) || mayMatchText(in.Stdout)
}

// mayMatchText reports whether text contains at least one literal that some
// rule requires. gateTokens is the union of those literals over the whole
// catalogue, so a rule can never match text the gate rejects. When the
// catalogue holds a rule without a provable literal, the gate accepts every
// text.
func mayMatchText(text string) bool {
	if !gateEnabled {
		return true
	}
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.ContainsRune(lower, 'ſ') {
		// Case-insensitive patterns fold U+017F onto "s"; ToLower does not.
		lower = strings.ReplaceAll(lower, "ſ", "s")
	}
	for _, token := range gateTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// gateTokenMinLen is the shortest literal run that may act as a gate token.
// Shorter runs appear in ordinary prose too often to filter anything.
const gateTokenMinLen = 4

// gateTokens is the sorted, de-duplicated union of every rule's required
// literals. gateEnabled is false when some rule has neither a derivable nor a
// declared token, and then the gate rejects nothing.
var gateTokens, gateEnabled = buildGateTokens()

// gateTokensByRuleID supplies tokens for rules that match through a custom
// matcher instead of a pattern, or whose pattern has a match path without a
// usable literal. Each entry lists one token per match path.
var gateTokensByRuleID = map[string][]string{
	// A bare line-leading "429" is one of its match paths; every other path
	// carries "limit".
	"claude.stderr.rate.v1":                {"429", "limit"},
	cursorRetriableResourceExhaustedRuleID: {"retriableerror"},
	cursorRetriableUnavailableRuleID:       {"retriableerror"},
	cursorRetriableConnectionStalledRuleID: {"retriableerror"},
	cursorRetriableStreamResetRuleID:       {"retriableerror"},
}

// buildGateTokens collects the catalogue's required literals and reports
// whether every rule is covered.
func buildGateTokens() ([]string, bool) {
	seen := make(map[string]struct{})
	covered := true
	collect := func(ruleID string, pattern *regexp.Regexp) {
		if !collectRuleTokens(seen, ruleID, pattern) {
			covered = false
		}
	}
	for _, rules := range providerRules {
		for _, r := range rules {
			collect(r.id, r.pattern)
		}
	}
	for _, r := range providerNeutralRules {
		collect(r.id, r.pattern)
	}
	for _, r := range runtimeEnvironmentRules {
		pattern := r.pattern
		if r.match != nil {
			pattern = nil
		}
		collect(r.id, pattern)
	}
	tokens := make([]string, 0, len(seen))
	for token := range seen {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens, covered
}

func collectRuleTokens(seen map[string]struct{}, ruleID string, pattern *regexp.Regexp) bool {
	tokens := patternGateTokens(pattern)
	if len(tokens) == 0 {
		declared, ok := gateTokensByRuleID[ruleID]
		if !ok || len(declared) == 0 {
			return false
		}
		tokens = declared
	}
	for _, token := range tokens {
		seen[strings.ToLower(token)] = struct{}{}
	}
	return true
}

// patternGateTokens returns the literals the gate looks for on behalf of
// pattern, one per required term, or nil when some match path of pattern
// requires no usable literal. Every literal in a pattern is not enough: for
// `x(?:longtoken)?y` or `ab|longtoken` the text "xy" or "ab" matches without
// "longtoken".
func patternGateTokens(pattern *regexp.Regexp) []string {
	terms := patternRequiredTerms(pattern)
	if len(terms) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	for _, term := range terms {
		// Every token of a term is present together, so the longest one alone
		// still covers the term.
		seen[longestToken(term)] = struct{}{}
	}
	tokens := make([]string, 0, len(seen))
	for token := range seen {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}

func patternRequiredTerms(pattern *regexp.Regexp) [][]string {
	if pattern == nil {
		return nil
	}
	re, err := syntax.Parse(pattern.String(), syntax.Perl)
	if err != nil {
		return nil
	}
	return requiredTerms(mergeAdjacentLiterals(re))
}

// mergeAdjacentLiterals joins literal siblings of a concatenation. The parser
// factors alternations around shared prefixes, so `invalid|incorrect` becomes
// "i" followed by two branches; merging keeps tokens whole.
func mergeAdjacentLiterals(re *syntax.Regexp) *syntax.Regexp {
	if re.Op != syntax.OpConcat || len(re.Sub) == 0 {
		for i, sub := range re.Sub {
			if merged := mergeAdjacentLiterals(sub); merged != sub {
				re.Sub[i] = merged
			}
		}
		return re
	}
	merged := &syntax.Regexp{Op: re.Op, Flags: re.Flags, Min: re.Min, Max: re.Max}
	for _, sub := range re.Sub {
		sub = mergeAdjacentLiterals(sub)
		last := len(merged.Sub) - 1
		if sub.Op == syntax.OpLiteral && last >= 0 && merged.Sub[last].Op == syntax.OpLiteral {
			prev := merged.Sub[last]
			merged.Sub[last] = &syntax.Regexp{
				Op:    syntax.OpLiteral,
				Flags: prev.Flags | sub.Flags,
				Rune:  append(append([]rune{}, prev.Rune...), sub.Rune...),
			}
			continue
		}
		merged.Sub = append(merged.Sub, sub)
	}
	return merged
}

// maxRequiredTerms bounds the disjunctive form. Crossing several alternations
// grows exponentially; past this bound the pattern counts as underivable.
const maxRequiredTerms = 64

// requiredTerms returns the literals every match of re is guaranteed to carry,
// as a disjunction of conjunctions: a match contains every token of at least
// one returned term. An empty result means some match path requires no usable
// literal.
func requiredTerms(re *syntax.Regexp) [][]string {
	switch re.Op {
	case syntax.OpLiteral:
		if token := longestLiteralRun(re.Rune); token != "" {
			return [][]string{{token}}
		}
	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			return requiredTerms(re.Sub[0])
		}
	case syntax.OpConcat:
		return concatTerms(re.Sub)
	case syntax.OpAlternate:
		// A match takes exactly one branch, so every branch must be covered.
		var terms [][]string
		for _, sub := range re.Sub {
			branch := requiredTerms(sub)
			if len(branch) == 0 {
				return nil
			}
			terms = append(terms, branch...)
		}
		return terms
	case syntax.OpPlus, syntax.OpRepeat:
		// An element with a minimum of zero is optional.
		if re.Op == syntax.OpPlus || re.Min >= 1 {
			return requiredTerms(re.Sub[0])
		}
	}
	return nil
}

// concatTerms crosses the requirements of a concatenation's elements, since a
// match carries all of them at once.
func concatTerms(subs []*syntax.Regexp) [][]string {
	terms := [][]string{{}}
	required := false
	for _, sub := range subs {
		element := requiredTerms(sub)
		if len(element) == 0 {
			continue
		}
		crossed := crossTerms(terms, element)
		if len(crossed) == 0 {
			return nil
		}
		terms = crossed
		required = true
	}
	if !required {
		return nil
	}
	return terms
}

func crossTerms(terms, elements [][]string) [][]string {
	if len(terms)*len(elements) > maxRequiredTerms {
		return nil
	}
	crossed := make([][]string, 0, len(terms)*len(elements))
	for _, term := range terms {
		for _, element := range elements {
			crossed = append(crossed, mergeTokens(term, element))
		}
	}
	return crossed
}

func longestToken(tokens []string) string {
	longest := ""
	for _, token := range tokens {
		if len(token) > len(longest) {
			longest = token
		}
	}
	return longest
}

func mergeTokens(term, element []string) []string {
	merged := make([]string, 0, len(term)+len(element))
	seen := make(map[string]struct{}, len(term)+len(element))
	for _, token := range append(append([]string{}, term...), element...) {
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		merged = append(merged, token)
	}
	sort.Strings(merged)
	return merged
}

// longestLiteralRun returns the longest ASCII alphanumeric run of at least
// gateTokenMinLen characters in runes, lowercased.
func longestLiteralRun(runes []rune) string {
	best := ""
	var current strings.Builder
	flush := func() {
		if current.Len() > len(best) {
			best = current.String()
		}
		current.Reset()
	}
	for _, r := range runes {
		if !isLiteralRune(r) {
			flush()
			continue
		}
		current.WriteRune(unicode.ToLower(r))
	}
	flush()
	if len(best) < gateTokenMinLen {
		return ""
	}
	return best
}

func isLiteralRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}
