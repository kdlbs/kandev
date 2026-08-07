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

// Validate checks source identity and conflicts without revealing any secret
// values. Callers may use it as an early shape check; Resolve must still run at
// the final composition boundary after all managed runtime definitions exist.
func Validate(definitions []Definition) error {
	_, err := selectDefinitions(definitions)
	return err
}

// Resolve sorts definitions deterministically, deduplicates identical source
// identities, reports every origin participating in a conflict, and reveals
// secrets only after the complete conflict pass succeeds.
func Resolve(ctx context.Context, definitions []Definition, reveal RevealFunc) (map[string]string, error) {
	selected, err := selectDefinitions(definitions)
	if err != nil {
		return nil, err
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
				return nil, &SecretError{Key: definition.Key, Origin: definition.Origin, err: fmt.Errorf("secret resolver unavailable")}
			}
			value, err = reveal(ctx, definition)
			if err != nil {
				return nil, &SecretError{Key: definition.Key, Origin: definition.Origin, err: err}
			}
		}
		resolved[key] = value
	}
	return resolved, nil
}

func selectDefinitions(definitions []Definition) (map[string]Definition, error) {
	ordered := append([]Definition(nil), definitions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Key != ordered[j].Key {
			return ordered[i].Key < ordered[j].Key
		}
		if ordered[i].Origin != ordered[j].Origin {
			return ordered[i].Origin < ordered[j].Origin
		}
		return ordered[i].SecretID < ordered[j].SecretID
	})

	selected := make(map[string]Definition, len(ordered))
	origins := make(map[string]map[string]struct{}, len(ordered))
	conflicts := make(map[string]map[string]struct{})
	for _, definition := range ordered {
		if strings.TrimSpace(definition.Key) == "" {
			continue
		}
		if prior, exists := selected[definition.Key]; exists {
			if definitionIdentity(prior) != definitionIdentity(definition) {
				if conflicts[definition.Key] == nil {
					conflicts[definition.Key] = make(map[string]struct{})
				}
				for origin := range origins[definition.Key] {
					conflicts[definition.Key][origin] = struct{}{}
				}
				conflicts[definition.Key][definition.Origin] = struct{}{}
				continue
			}
			origins[definition.Key][definition.Origin] = struct{}{}
			continue
		}
		selected[definition.Key] = definition
		origins[definition.Key] = map[string]struct{}{definition.Origin: {}}
	}

	if len(conflicts) > 0 {
		keys := make([]string, 0, len(conflicts))
		for key := range conflicts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		key := keys[0]
		conflictOrigins := make([]string, 0, len(conflicts[key]))
		for origin := range conflicts[key] {
			conflictOrigins = append(conflictOrigins, origin)
		}
		sort.Strings(conflictOrigins)
		return nil, &ConflictError{Key: key, Origins: conflictOrigins}
	}
	return selected, nil
}

func definitionIdentity(definition Definition) string {
	if definition.SecretID != "" {
		return "secret:" + definition.SecretID
	}
	hash := sha256.Sum256([]byte(definition.Literal))
	return "literal:" + hex.EncodeToString(hash[:])
}
