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

	"github.com/kandev/kandev/internal/gitconfigenv"
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
	return fmt.Sprintf("environment key %q from %s references an unavailable secret. Re-select the secret in the %s environment settings", e.Key, e.Origin, e.Origin)
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
	ordinary, indexed := splitIndexedDefinitions(definitions)
	if _, _, err := selectDefinitions(ordinary); err != nil {
		return err
	}
	return validateIndexedDefinitions(indexed)
}

// Resolve sorts definitions deterministically over five named columns,
// merges identical source identities, applies the secret veto and then tier
// precedence to any remaining ambiguity, and reveals secrets only after the
// complete conflict pass succeeds. On any error it returns a nil map and a
// nil []OverrideRecord — all or nothing, even when tier precedence had
// already resolved other keys before the failing key was reached.
func Resolve(ctx context.Context, definitions []Definition, reveal RevealFunc) (map[string]string, []OverrideRecord, error) {
	ordinary, indexed := splitIndexedDefinitions(definitions)
	indexedSelections, err := prepareIndexedDefinitions(indexed)
	if err != nil {
		return nil, nil, err
	}
	selected, records, err := selectDefinitions(ordinary)
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

	indexedEnv, err := resolveIndexedDefinitions(ctx, indexedSelections, reveal)
	if err != nil {
		return nil, nil, err
	}
	for key, value := range indexedEnv {
		resolved[key] = value
	}
	return resolved, records, nil
}

// splitIndexedDefinitions keeps Git's indexed configuration protocol as one
// composed value. Treating GIT_CONFIG_COUNT, GIT_CONFIG_KEY_* and
// GIT_CONFIG_VALUE_* as independent environment keys makes two authoritative
// sources conflict or lets one source hide the other source's entries.
func splitIndexedDefinitions(definitions []Definition) (ordinary, indexed []Definition) {
	ordinary = make([]Definition, 0, len(definitions))
	indexed = make([]Definition, 0)
	for _, definition := range definitions {
		if gitconfigenv.IsIndexedKey(definition.Key) {
			indexed = append(indexed, definition)
			continue
		}
		ordinary = append(ordinary, definition)
	}
	return ordinary, indexed
}

type indexedDefinitionBlock struct {
	origin      string
	workspaceID string
	definitions []Definition
}

type indexedDefinitionSelection struct {
	block    indexedDefinitionBlock
	selected map[string]Definition
}

func indexedDefinitionBlocks(definitions []Definition) []indexedDefinitionBlock {
	if len(definitions) == 0 {
		return nil
	}
	bySource := make(map[string]*indexedDefinitionBlock, len(definitions))
	for _, definition := range definitions {
		identity := definition.Origin + "\x00" + definition.WorkspaceID
		block, ok := bySource[identity]
		if !ok {
			block = &indexedDefinitionBlock{
				origin:      definition.Origin,
				workspaceID: definition.WorkspaceID,
			}
			bySource[identity] = block
		}
		block.definitions = append(block.definitions, definition)
	}
	blocks := make([]indexedDefinitionBlock, 0, len(bySource))
	for _, block := range bySource {
		blocks = append(blocks, *block)
	}
	// Weaker sources are composed first. Within one tier the origin and
	// workspace order is deterministic; the later block is the effective Git
	// configuration source when repeated Git keys are present.
	sort.Slice(blocks, func(i, j int) bool {
		ti, tj := TierForOrigin(blocks[i].origin), TierForOrigin(blocks[j].origin)
		if ti != tj {
			return ti > tj
		}
		if blocks[i].origin != blocks[j].origin {
			return blocks[i].origin < blocks[j].origin
		}
		return blocks[i].workspaceID < blocks[j].workspaceID
	})
	return blocks
}

func validateIndexedDefinitions(definitions []Definition) error {
	_, err := prepareIndexedDefinitions(definitions)
	return err
}

func prepareIndexedDefinitions(definitions []Definition) ([]indexedDefinitionSelection, error) {
	blocks := indexedDefinitionBlocks(definitions)
	if len(blocks) == 0 {
		return nil, nil
	}
	selections := make([]indexedDefinitionSelection, 0, len(blocks))
	composedShape := make(map[string]string)
	for _, block := range blocks {
		selected, _, err := selectDefinitions(block.definitions)
		if err != nil {
			return nil, err
		}
		shape := indexedDefinitionValues(selected, true)
		if _, err := gitconfigenv.Merge(nil, shape); err != nil {
			return nil, fmt.Errorf("validate indexed Git configuration from %s: %w", indexedSourceLabel(block), err)
		}
		selections = append(selections, indexedDefinitionSelection{block: block, selected: selected})
		// Secret-backed counts cannot be shape-validated until reveal. The
		// known portion still needs a combined-limit check before any secret
		// is revealed by Resolve.
		mergedShape, err := gitconfigenv.Merge(composedShape, shape)
		if err != nil {
			return nil, fmt.Errorf("compose indexed Git configuration: %w", err)
		}
		composedShape = mergedShape
	}
	return selections, nil
}

func resolveIndexedDefinitions(ctx context.Context, selections []indexedDefinitionSelection, reveal RevealFunc) (map[string]string, error) {
	resolved := make(map[string]string)
	for _, selection := range selections {
		block := selection.block
		selected := selection.selected
		source := make(map[string]string, len(selected))
		var err error
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
					return nil, &SecretError{Key: definition.Key, Origin: definition.Origin, err: fmt.Errorf("secret resolver unavailable")}
				}
				value, err = reveal(ctx, definition)
				if err != nil {
					return nil, &SecretError{Key: definition.Key, Origin: definition.Origin, err: err}
				}
			}
			source[key] = value
		}
		// Re-parse after revealing secrets. Validate cannot inspect secret
		// values, so the final boundary must still reject a malformed counted
		// block or an invalid combined count.
		if _, err := gitconfigenv.Merge(nil, source); err != nil {
			return nil, fmt.Errorf("resolve indexed Git configuration from %s: %w", indexedSourceLabel(block), err)
		}
		resolved, err = gitconfigenv.Merge(resolved, source)
		if err != nil {
			return nil, fmt.Errorf("compose indexed Git configuration: %w", err)
		}
	}
	return resolved, nil
}

func indexedDefinitionValues(selected map[string]Definition, shapeOnly bool) map[string]string {
	values := make(map[string]string, len(selected))
	for key, definition := range selected {
		value := definition.Literal
		if shapeOnly && definition.SecretID != "" && key != "GIT_CONFIG_COUNT" {
			value = "<secret>"
		}
		// A secret-backed count cannot be shape-validated without revealing it.
		// Leave it absent for the non-revealing preflight; Resolve validates the
		// actual count after the final secret boundary.
		if shapeOnly && definition.SecretID != "" && key == "GIT_CONFIG_COUNT" {
			continue
		}
		values[key] = value
	}
	return values
}

func indexedSourceLabel(block indexedDefinitionBlock) string {
	if block.workspaceID == "" {
		return block.origin
	}
	return block.origin + " workspace " + block.workspaceID
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
