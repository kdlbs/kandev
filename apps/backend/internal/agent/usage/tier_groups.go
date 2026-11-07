package usage

import (
	"sort"
	"strings"
)

// ConfiguredScopeCovers reports whether an owner- or catalog-defined scope
// covers the model. Such a scope states an individual or known shared
// allowance, so the model never joins a provisional group.
func ConfiguredScopeCovers(configured []RequestScope, model string) bool {
	for _, scope := range configured {
		if scope.Covers(model) {
			return true
		}
	}
	return false
}

// ProvisionalGroupFor returns the provisional group a per-model daily block of
// model shares with partner models blocked in the same minute. The models must
// already be filtered to those without a configured scope. If the models touch
// two different provisional groups the evidence is ambiguous and nothing is
// grouped. An existing provisional group keeps its ID as it grows.
func ProvisionalGroupFor(learned []RequestScope, model string, partners []string) (RequestScope, bool) {
	model = ScopeModelID(model)
	members := map[string]bool{model: true}
	for _, partner := range partners {
		if partner = ScopeModelID(partner); partner != "" && !strings.EqualFold(partner, model) {
			members[partner] = true
		}
	}
	if len(members) < 2 {
		return RequestScope{}, false
	}
	var group RequestScope
	found := false
	for member := range members {
		for _, scope := range learned {
			if scope.Kind != ScopeKindGroup || !scope.Provisional || !scope.coversModelID(member) {
				continue
			}
			if found && scope.ID != group.ID {
				return RequestScope{}, false
			}
			group, found = scope, true
		}
	}
	for _, existing := range group.Models {
		members[existing] = true
	}
	models := make([]string, 0, len(members))
	for member := range members {
		models = append(models, member)
	}
	sort.Strings(models)
	if !found {
		group = RequestScope{ID: ScopeKindGroup + ":" + strings.Join(models, "+")}
	}
	group.Kind, group.Models, group.Provisional = ScopeKindGroup, models, true
	return group, true
}

// WithoutLearnedModelScopes drops learned single-model scopes of the given
// models once they share a group allowance. Scopes whose ID the owner set are
// kept so the owner entry never loses the definition it applies to.
func WithoutLearnedModelScopes(scopes []RequestScope, models []string, ownerIDs map[string]bool) []RequestScope {
	result := make([]RequestScope, 0, len(scopes))
	for _, scope := range scopes {
		if scope.Kind == ScopeKindModel && scope.Learned && !ownerIDs[scope.ID] && coversAny(scope, models) {
			continue
		}
		result = append(result, scope)
	}
	return result
}

func coversAny(scope RequestScope, models []string) bool {
	for _, model := range models {
		if scope.coversModelID(model) {
			return true
		}
	}
	return false
}
