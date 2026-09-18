package automation

import "strings"

// EvaluateFilters checks payload data against a webhook trigger's declared
// filter predicates. Every predicate must pass (empty list always passes).
// Predicates run in declaration order and short-circuit on the first
// failure, returning its index; ok is true only when every predicate passed.
func EvaluateFilters(filters []WebhookFilter, data map[string]interface{}) (rejectedIndex int, ok bool) {
	for i, f := range filters {
		if !evaluateFilter(f, data) {
			return i, false
		}
	}
	return -1, true
}

func evaluateFilter(f WebhookFilter, data map[string]interface{}) bool {
	switch f.Op {
	case WebhookFilterOpExists:
		if len(f.Values) != 0 {
			return false
		}
		_, ok := lookupPath(data, f.Path)
		return ok
	case WebhookFilterOpNotExists:
		if len(f.Values) != 0 {
			return false
		}
		_, ok := lookupPath(data, f.Path)
		return !ok
	case WebhookFilterOpEq:
		return evaluateEquality(data, f, true)
	case WebhookFilterOpNe:
		return evaluateEquality(data, f, false)
	case WebhookFilterOpIn:
		return evaluateMembership(data, f, true)
	case WebhookFilterOpNotIn:
		return evaluateMembership(data, f, false)
	case WebhookFilterOpContains:
		return evaluateContains(data, f)
	default:
		// An unrecognized operator can't be evaluated, so it fails closed.
		return false
	}
}

func evaluateEquality(data map[string]interface{}, f WebhookFilter, want bool) bool {
	if len(f.Values) != 1 {
		return false
	}
	value, ok := lookupPath(data, f.Path)
	if !ok {
		return false
	}
	equal := normalizeFilterValue(value) == normalizeFilterValue(f.Values[0])
	return equal == want
}

func evaluateMembership(data map[string]interface{}, f WebhookFilter, want bool) bool {
	value, ok := lookupPath(data, f.Path)
	if !ok {
		return false
	}
	normalized := normalizeFilterValue(value)
	member := false
	for _, v := range f.Values {
		if normalizeFilterValue(v) == normalized {
			member = true
			break
		}
	}
	return member == want
}

func evaluateContains(data map[string]interface{}, f WebhookFilter) bool {
	if len(f.Values) != 1 {
		return false
	}
	value, ok := lookupPath(data, f.Path)
	if !ok {
		return false
	}
	return strings.Contains(normalizeFilterValue(value), normalizeFilterValue(f.Values[0]))
}

// normalizeFilterValue implements the case-insensitive comparison rule
// shared by eq/ne/in/not_in/contains: trim both sides, then lowercase via
// strings.ToLower (a simple lowercase mapping, not Unicode case folding).
func normalizeFilterValue(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
