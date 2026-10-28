package externalcatalog

import (
	"regexp"
	"strings"
	"time"
)

var ruleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Catalog is an immutable, ordered set of compiled rules. The first matching
// rule wins.
type Catalog struct {
	rules []compiledRule
}

// Len reports the number of compiled rules.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.rules)
}

type compiledRule struct {
	id         string
	pattern    *regexp.Regexp
	terms      []string
	minTerms   int
	providers  []string
	phases     []string
	statuses   []int
	context    *contextDTO
	gated      bool
	code       string
	confidence string
	scope      string
	backoff    time.Duration
	reset      string
}

type ruleCompiler struct {
	index int
	id    string
	vocab Vocabulary
}

func (rc ruleCompiler) fail(reason, field string) *LoadError {
	return &LoadError{Reason: reason, Index: rc.index, RuleID: rc.id, Field: field}
}

func compile(doc documentDTO, vocab Vocabulary) (*Catalog, error) {
	if doc.Version == 0 {
		return nil, &LoadError{Reason: ReasonMissingField, Index: -1, Field: "version"}
	}
	if doc.Version != SchemaVersion {
		return nil, documentError(ReasonUnsupportedVersion)
	}
	seen := make(map[string]bool, len(doc.Rules))
	rules := make([]compiledRule, 0, len(doc.Rules))
	for i, dto := range doc.Rules {
		rc := ruleCompiler{index: i, id: dto.ID, vocab: vocab}
		if !ruleIDPattern.MatchString(dto.ID) {
			rc.id = ""
			if dto.ID == "" {
				return nil, rc.fail(ReasonMissingField, "id")
			}
			return nil, rc.fail(ReasonInvalidValue, "id")
		}
		if seen[dto.ID] {
			return nil, rc.fail(ReasonDuplicateID, "id")
		}
		seen[dto.ID] = true
		rule, err := rc.compileRule(dto)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return &Catalog{rules: rules}, nil
}

func (rc ruleCompiler) compileRule(dto ruleDTO) (compiledRule, error) {
	if dto.Match == nil {
		return compiledRule{}, rc.fail(ReasonMissingField, "match")
	}
	if dto.Result == nil {
		return compiledRule{}, rc.fail(ReasonMissingField, "result")
	}
	rule := compiledRule{id: dto.ID}
	if err := rc.compileMatch(*dto.Match, &rule); err != nil {
		return compiledRule{}, err
	}
	if err := rc.compileResult(*dto.Result, &rule); err != nil {
		return compiledRule{}, err
	}
	if err := rc.checkResetContract(*dto.Match, &rule); err != nil {
		return compiledRule{}, err
	}
	return rule, nil
}

func (rc ruleCompiler) compileMatch(m matchDTO, rule *compiledRule) error {
	if m.Pattern == "" && len(m.Terms) == 0 {
		return rc.fail(ReasonMissingField, "match.pattern")
	}
	if m.Pattern != "" {
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return rc.fail(ReasonInvalidPattern, "match.pattern")
		}
		rule.pattern = re
	}
	if err := rc.compileTerms(m, rule); err != nil {
		return err
	}
	providers, ok := nonEmptyUnique(m.Providers)
	if !ok {
		return rc.fail(ReasonInvalidValue, "match.providers")
	}
	rule.providers = providers
	if !allIn(m.Phases, rc.vocab.Phases) || !unique(m.Phases) {
		return rc.fail(ReasonInvalidValue, "match.phases")
	}
	rule.phases = m.Phases
	for _, status := range m.HTTPStatus {
		if status < 100 || status > 599 {
			return rc.fail(ReasonInvalidValue, "match.httpStatus")
		}
	}
	rule.statuses = m.HTTPStatus
	if m.Context != nil {
		if err := rc.checkContext(*m.Context); err != nil {
			return err
		}
		rule.context = m.Context
	}
	rule.gated = len(rule.terms) > 0 || rule.context != nil
	return nil
}

// compileTerms requires an explicit minimum. Without a pattern, at least two
// distinct terms must match so that a bare topic word never classifies.
func (rc ruleCompiler) compileTerms(m matchDTO, rule *compiledRule) error {
	if len(m.Terms) == 0 {
		if m.MinTerms != 0 {
			return rc.fail(ReasonInvalidValue, "match.minTerms")
		}
		return nil
	}
	terms := make([]string, 0, len(m.Terms))
	for _, term := range m.Terms {
		normalized := strings.Join(strings.Fields(strings.ToLower(term)), " ")
		if normalized == "" {
			return rc.fail(ReasonInvalidValue, "match.terms")
		}
		terms = append(terms, normalized)
	}
	if !unique(terms) {
		return rc.fail(ReasonInvalidValue, "match.terms")
	}
	if m.MinTerms == 0 {
		return rc.fail(ReasonMissingField, "match.minTerms")
	}
	floor := 1
	if m.Pattern == "" {
		floor = 2
	}
	if m.MinTerms < floor || m.MinTerms > len(terms) {
		return rc.fail(ReasonInvalidValue, "match.minTerms")
	}
	rule.terms = terms
	rule.minTerms = m.MinTerms
	return nil
}

func (rc ruleCompiler) checkContext(c contextDTO) error {
	lists := []struct {
		field  string
		values []string
	}{
		{"match.context.tiers", c.Tiers},
		{"match.context.accounts", c.Accounts},
		{"match.context.models", c.Models},
		{"match.context.paths", c.Paths},
	}
	for _, list := range lists {
		if _, ok := nonEmptyUnique(list.values); !ok {
			return rc.fail(ReasonInvalidValue, list.field)
		}
	}
	if c.MinConsecutiveFailures < 0 {
		return rc.fail(ReasonInvalidValue, "match.context.minConsecutiveFailures")
	}
	if c.MinOtherPaths < 0 {
		return rc.fail(ReasonInvalidValue, "match.context.minOtherPaths")
	}
	if c.MinOtherModels < 0 {
		return rc.fail(ReasonInvalidValue, "match.context.minOtherModels")
	}
	return nil
}

func (rc ruleCompiler) compileResult(r resultDTO, rule *compiledRule) error {
	if r.Code == "" {
		return rc.fail(ReasonMissingField, "result.code")
	}
	if !contains(rc.vocab.Codes, r.Code) {
		return rc.fail(ReasonInvalidValue, "result.code")
	}
	if r.Confidence == "" {
		return rc.fail(ReasonMissingField, "result.confidence")
	}
	if !contains(rc.vocab.Confidences, r.Confidence) {
		return rc.fail(ReasonInvalidValue, "result.confidence")
	}
	rule.code, rule.confidence = r.Code, r.Confidence
	switch r.Scope {
	case "":
		rule.scope = ScopePathModel
	case ScopePathModel, ScopeModel, ScopeAccount:
		rule.scope = r.Scope
	default:
		return rc.fail(ReasonInvalidValue, "result.scope")
	}
	if r.Backoff != "" {
		backoff, err := time.ParseDuration(r.Backoff)
		if err != nil || backoff <= 0 {
			return rc.fail(ReasonInvalidDuration, "result.backoff")
		}
		rule.backoff = backoff
	}
	switch r.Reset {
	case "":
		rule.reset = ResetInherit
	case ResetInherit, ResetFreeDaily, ResetUntilModelChange:
		rule.reset = r.Reset
	default:
		return rc.fail(ReasonInvalidValue, "result.reset")
	}
	return nil
}

// checkResetContract limits the special resets to the evidence they already
// require in the classifier: a confirmed free daily allowance, or a 404
// missing model.
func (rc ruleCompiler) checkResetContract(m matchDTO, rule *compiledRule) error {
	switch rule.reset {
	case ResetFreeDaily:
		if m.Context == nil || m.Context.FreeDaily == nil || !*m.Context.FreeDaily {
			return rc.fail(ReasonInvalidValue, "result.reset")
		}
	case ResetUntilModelChange:
		if rule.code != codeModelUnavailable || len(m.HTTPStatus) != 1 || m.HTTPStatus[0] != statusNotFound {
			return rc.fail(ReasonInvalidValue, "result.reset")
		}
	}
	return nil
}

func nonEmptyUnique(values []string) ([]string, bool) {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return nil, false
		}
	}
	return values, unique(values)
}

func unique(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func allIn(values, allowed []string) bool {
	for _, value := range values {
		if !contains(allowed, value) {
			return false
		}
	}
	return true
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
