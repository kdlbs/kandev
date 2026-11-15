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
// engine. It is a conservative pre-check: a false result guarantees Classify
// would have produced its default Low-confidence error, and a true result only
// means the rules still have to run.
//
// Callers on a per-frame hot path use it to skip classification for text that
// cannot carry any rule evidence. The ACP adapter classifies the text of every
// assistant message chunk, so the classifier has to stay off the per-chunk
// path for ordinary prose.
//
// The guarantee covers Stderr and Stdout only. Structured evidence is never
// gated: an injected provider failure, an HTTP status, or an exit code
// classifies from the Input alone, so any of them makes the answer true
// regardless of text. A loaded operator catalog and the opt-in keyword
// context also match free text, so either one makes the answer true.
func MayClassify(in Input) bool {
	if in.KeywordContext.Enabled && in.KeywordContext.Diagnostic {
		return true
	}
	if hasStructuredClassificationEvidence(in) {
		return true
	}
	if inj := getInjection(); inj != nil {
		if _, ok := inj[in.ProviderID]; ok {
			return true
		}
	}
	if externalCatalog.Load() != nil {
		return true
	}
	if !gateEnabled {
		return true
	}
	if _, ok := gateCustomMatcherProviders[in.ProviderID]; ok {
		return true
	}
	text := in.Stderr + "\n" + in.Stdout
	// Go regexp's Unicode case folds include non-ASCII variants of ASCII
	// letters. Do not reject those through an ASCII literal comparison.
	for _, r := range text {
		if r > unicode.MaxASCII {
			return true
		}
	}
	// Classify reads both raw and sanitized evidence; redaction can introduce
	// placeholder literals, so the prefilter must cover both representations.
	return mayMatchText(text) || mayMatchText(Sanitize(text))
}

func hasStructuredClassificationEvidence(in Input) bool {
	return in.HTTPStatus != 0 || in.ExitCode != nil || in.StructuredErr != nil ||
		in.ACPCode != 0 || in.ErrorKind != "" || in.ProviderCode != "" ||
		in.ManagedRuntimePackageSpec != "" || in.ResetHint != nil
}

// mayMatchText reports whether text contains at least one literal that some
// matching rule requires. gateTokens is the union of those literals over the
// whole catalogue, so a rule can never match text the gate rejects.
//
// The union is derived from the rule patterns themselves (see patternGateTokens)
// rather than hand-listed, because a hand-listed token set rots silently. A
// pattern without a derivable cover disables the gate instead.
func mayMatchText(text string) bool {
	if !gateEnabled {
		return true
	}
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, token := range gateTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// gateTokenMinLen is the shortest literal run that may act as a gate token.
// Shorter runs appear in ordinary prose often enough to make the gate useless,
// so they are dropped; a pattern left without a usable run disables the gate.
const gateTokenMinLen = 4

// gateTokens is the sorted, de-duplicated union of every rule's required
// literals. gateEnabled is false when the catalogue holds a rule the derivation
// cannot cover, in which case the gate stops rejecting anything.
// gateCustomMatcherProviders lists providers whose runtime rules use a custom
// matcher; their text is always classified.
var gateTokens, gateCustomMatcherProviders, gateEnabled = buildGateTokens()

// buildGateTokens collects the catalogue's required literals. It reports false
// when some rule has no derivable token and is not scoped to one provider,
// because the union would then not cover that rule.
func buildGateTokens() ([]string, map[string]struct{}, bool) {
	seen := make(map[string]struct{})
	customProviders := make(map[string]struct{})
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
		if r.pattern == nil && r.match != nil && r.providerID != "" {
			// A provider-scoped custom matcher keeps that provider ungated
			// instead of disabling the gate for every provider.
			customProviders[r.providerID] = struct{}{}
			continue
		}
		collect(r.id, r.pattern)
	}
	tokens := make([]string, 0, len(seen))
	for token := range seen {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens, customProviders, covered
}

// gateTokensByRuleID supplies tokens for rules whose pattern has a match path
// without a usable literal. Each entry lists one token per match path.
var gateTokensByRuleID = map[string][]string{
	// A bare line-leading "429" is one of its match paths; every other path
	// carries "limit".
	"claude.stderr.rate.v1": {"429", "limit"},
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

// patternGateTokens returns every literal the gate may look for on behalf of
// pattern, or nil when no such cover exists: a missing pattern, or a pattern
// with a match path that requires no usable literal.
func patternGateTokens(pattern *regexp.Regexp) []string {
	terms := patternRequiredTerms(pattern)
	if len(terms) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	for _, term := range terms {
		// Every token of a term is guaranteed present together, so keeping the
		// longest is still a cover and keeps the global token set small.
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

// mergeAdjacentLiterals joins literal siblings of a concatenation so derived
// tokens stay whole words instead of parser-factored fragments.
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
	appendMerged := func(sub *syntax.Regexp) {
		if sub.Op == syntax.OpLiteral && len(merged.Sub) > 0 && merged.Sub[len(merged.Sub)-1].Op == syntax.OpLiteral {
			last := merged.Sub[len(merged.Sub)-1]
			merged.Sub[len(merged.Sub)-1] = &syntax.Regexp{
				Op:    syntax.OpLiteral,
				Flags: last.Flags | sub.Flags,
				Rune:  append(append([]rune{}, last.Rune...), sub.Rune...),
			}
			return
		}
		merged.Sub = append(merged.Sub, sub)
	}
	for _, sub := range re.Sub {
		appendMerged(mergeAdjacentLiterals(sub))
	}
	return merged
}

// maxRequiredTerms bounds the disjunctive form; a wider rule reports no cover
// and turns the gate off rather than rejecting text it may match.
const maxRequiredTerms = 64

// requiredTerms returns the literals every match of re is guaranteed to carry,
// as a disjunction of conjunctions. An empty result means no match path
// requires a usable literal.
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
		// A match takes exactly one branch, so an unguarded branch would let a
		// match through with no token at all.
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
		if re.Op == syntax.OpPlus || re.Min >= 1 {
			return requiredTerms(re.Sub[0])
		}
	}
	return nil
}

// concatTerms crosses the requirements of a concatenation's elements and
// reports an unrepresentable product as an absence rather than approximating.
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

// longestLiteralRun returns the longest alphanumeric run of at least
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
