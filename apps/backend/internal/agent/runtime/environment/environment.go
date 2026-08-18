// Package environment resolves the source definitions that make up a task's
// effective runtime environment. Keeping source identity until the final
// composition boundary prevents a later runtime default from silently
// replacing a repository-approved value.
package environment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Definition describes one environment value without including its plaintext
// secret value in the definition itself.
type Definition struct {
	Key         string
	Literal     string
	SecretID    string
	Origin      string
	WorkspaceID string
}

// ConflictError identifies an ambiguous environment key without including
// plaintext values or secret identifiers.
type ConflictError struct {
	Key     string
	Origins []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("environment key %q has conflicting definitions from %s", e.Key, strings.Join(e.Origins, ", "))
}

// SecretError identifies a secret that could not be resolved while retaining a
// redacted error boundary for callers and logs.
type SecretError struct {
	Key    string
	Origin string
	err    error
}

func (e *SecretError) Error() string {
	return fmt.Sprintf("environment key %q from %s could not be resolved", e.Key, e.Origin)
}

func (e *SecretError) Unwrap() error { return e.err }

// RevealFunc resolves a secret-backed definition. Literal definitions are not
// passed to the callback.
type RevealFunc func(context.Context, Definition) (string, error)

// Validate checks source identity, tier precedence, and secret-veto
// conflicts without revealing any secret values. Callers may use it as an
// early shape check; Resolve must still run at the final composition
// boundary after all managed runtime definitions exist. Validate emits no
// override record, no log, and no counter increment — the environment
// package holds no logger and stays a pure function of its inputs.
func Validate(definitions []Definition) error {
	_, _, err := selectDefinitions(definitions)
	return err
}

// Resolve sorts definitions deterministically over five named columns,
// merges identical source identities, applies the secret veto and then tier
// precedence to any remaining ambiguity, and reveals secrets only after the
// complete conflict pass succeeds. On any error it returns a nil map and a
// nil []OverrideRecord — all or nothing, even when tier precedence had
// already resolved other keys before the failing key was reached.
func Resolve(ctx context.Context, definitions []Definition, reveal RevealFunc) (map[string]string, []OverrideRecord, error) {
	selected, records, err := selectDefinitions(definitions)
	if err != nil {
		return nil, nil, err
	}

	resolved := make(map[string]string, len(selected))
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		definition := selected[key]
		value := definition.Literal
		if definition.SecretID != "" {
			if reveal == nil {
				return nil, nil, &SecretError{Key: definition.Key, Origin: definition.Origin, err: fmt.Errorf("secret resolver unavailable")}
			}
			value, err = reveal(ctx, definition)
			if err != nil {
				return nil, nil, &SecretError{Key: definition.Key, Origin: definition.Origin, err: err}
			}
		}
		resolved[key] = value
	}
	return resolved, records, nil
}

// selectDefinitions implements the resolution procedure: a deterministic
// five-column sort, per-key identity merging, the secret veto, and tier
// precedence over any remaining ambiguity.
func selectDefinitions(definitions []Definition) (map[string]Definition, []OverrideRecord, error) {
	ordered := append([]Definition(nil), definitions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return definitionLess(ordered[i], ordered[j])
	})

	keyOrder := make([]string, 0, len(ordered))
	buckets := make(map[string][]Definition, len(ordered))
	for _, definition := range ordered {
		if strings.TrimSpace(definition.Key) == "" {
			continue
		}
		if _, exists := buckets[definition.Key]; !exists {
			keyOrder = append(keyOrder, definition.Key)
		}
		buckets[definition.Key] = append(buckets[definition.Key], definition)
	}

	selected := make(map[string]Definition, len(keyOrder))
	var records []OverrideRecord
	var conflictKeys []string
	conflicts := make(map[string]*ConflictError, len(keyOrder))

	for _, key := range keyOrder {
		groups := buildIdentityGroups(buckets[key])
		definition, record, conflictErr := resolveGroups(key, groups)
		if conflictErr != nil {
			conflictKeys = append(conflictKeys, key)
			conflicts[key] = conflictErr
			continue
		}
		selected[key] = definition
		if record != nil {
			records = append(records, *record)
		}
	}

	if len(conflictKeys) > 0 {
		sort.Strings(conflictKeys)
		return nil, nil, conflicts[conflictKeys[0]]
	}

	sort.Slice(records, func(i, j int) bool { return records[i].Key < records[j].Key })
	return selected, records, nil
}

// definitionLess orders definitions by the five named columns — Key, Origin,
// SecretID, WorkspaceID, Literal, all ascending — the total order the
// procedure requires so merged-identity survivorship and revealed values
// never depend on input slice order.
func definitionLess(a, b Definition) bool {
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	if a.Origin != b.Origin {
		return a.Origin < b.Origin
	}
	if a.SecretID != b.SecretID {
		return a.SecretID < b.SecretID
	}
	if a.WorkspaceID != b.WorkspaceID {
		return a.WorkspaceID < b.WorkspaceID
	}
	return a.Literal < b.Literal
}

// identityGroup collects every definition sharing one definitionIdentity for
// a key. definition is the representative — the first under the total order
// — whose Literal/SecretID/WorkspaceID are used for revealing; origins is
// the full set of every origin that contributed to this identity.
type identityGroup struct {
	definition Definition
	origins    map[string]struct{}
}

// buildIdentityGroups groups defs (already ordered by definitionLess) by
// definitionIdentity, preserving first-seen order.
func buildIdentityGroups(defs []Definition) []*identityGroup {
	var groups []*identityGroup
	byIdentity := make(map[string]*identityGroup, len(defs))
	for _, definition := range defs {
		id := definitionIdentity(definition)
		group, ok := byIdentity[id]
		if !ok {
			group = &identityGroup{definition: definition, origins: map[string]struct{}{}}
			byIdentity[id] = group
			groups = append(groups, group)
		}
		group.origins[definition.Origin] = struct{}{}
	}
	return groups
}

// resolveGroups applies steps 4 through 6 of the resolution procedure to one
// key's identity groups: a single surviving identity wins outright; more
// than one triggers the secret veto first, then tier precedence.
func resolveGroups(key string, groups []*identityGroup) (Definition, *OverrideRecord, *ConflictError) {
	if len(groups) == 1 {
		return groups[0].definition, nil, nil
	}
	if hasSecretBackedGroup(groups) {
		return Definition{}, nil, conflictError(key, groups)
	}

	winner, winningTier, ambiguous := strongestGroup(groups)
	if ambiguous {
		return Definition{}, nil, conflictError(key, groups)
	}
	return winner.definition, overrideRecord(key, groups, winner, winningTier), nil
}

func hasSecretBackedGroup(groups []*identityGroup) bool {
	for _, group := range groups {
		if group.definition.SecretID != "" {
			return true
		}
	}
	return false
}

// strongestGroup finds the group whose strongest contributing origin
// classifies into the numerically smallest (strongest) tier. ambiguous is
// true when more than one group ties for that top tier.
func strongestGroup(groups []*identityGroup) (winner *identityGroup, winningTier Tier, ambiguous bool) {
	winner = groups[0]
	winningTier = strongestTier(winner.origins)
	for _, group := range groups[1:] {
		tier := strongestTier(group.origins)
		switch {
		case tier < winningTier:
			winner, winningTier, ambiguous = group, tier, false
		case tier == winningTier:
			ambiguous = true
		}
	}
	return winner, winningTier, ambiguous
}

// strongestTier is the numerically smallest (strongest) tier among origins.
func strongestTier(origins map[string]struct{}) Tier {
	var best Tier
	first := true
	for origin := range origins {
		tier := TierForOrigin(origin)
		if first || tier < best {
			best, first = tier, false
		}
	}
	return best
}

func conflictError(key string, groups []*identityGroup) *ConflictError {
	return &ConflictError{Key: key, Origins: allOrigins(groups)}
}

func overrideRecord(key string, groups []*identityGroup, winner *identityGroup, winningTier Tier) *OverrideRecord {
	losing := make(map[string]Tier)
	for _, group := range groups {
		if group == winner {
			continue
		}
		for origin := range group.origins {
			losing[origin] = TierForOrigin(origin)
		}
	}
	return &OverrideRecord{
		Key:            key,
		WinningOrigins: sortedOriginSet(winner.origins),
		WinningTier:    winningTier,
		LosingOrigins:  sortedOriginTiers(losing),
	}
}

func allOrigins(groups []*identityGroup) []string {
	set := make(map[string]struct{})
	for _, group := range groups {
		for origin := range group.origins {
			set[origin] = struct{}{}
		}
	}
	return sortedOriginSet(set)
}

func sortedOriginSet(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for origin := range set {
		result = append(result, origin)
	}
	sort.Strings(result)
	return result
}

func sortedOriginTiers(tiers map[string]Tier) []OriginTier {
	result := make([]OriginTier, 0, len(tiers))
	for origin, tier := range tiers {
		result = append(result, OriginTier{Origin: origin, Tier: tier})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Origin < result[j].Origin })
	return result
}

func definitionIdentity(definition Definition) string {
	if definition.SecretID != "" {
		return "secret:" + definition.SecretID
	}
	hash := sha256.Sum256([]byte(definition.Literal))
	return "literal:" + hex.EncodeToString(hash[:])
}
