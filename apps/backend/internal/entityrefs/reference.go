// Package entityrefs owns the provider-neutral structural reference contract.
package entityrefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

var ErrInvalidReference = errors.New("invalid entity reference")

var identityNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,127}$`)

const (
	maxReferencesPerMessage = 100
	kandevProviderID        = "kandev"
	kandevTaskKind          = "task"
)

// CanonicalRef constructs the opaque stable identity owned by the registry.
func CanonicalRef(provider, kind, scope, id string) string {
	parts := []string{
		strings.TrimSpace(provider),
		strings.TrimSpace(kind),
		strings.TrimSpace(scope),
		strings.TrimSpace(id),
	}
	for index := range parts {
		parts[index] = url.QueryEscape(parts[index])
	}
	return "mention:v1:" + strings.Join(parts, ":")
}

// NormalizeForSubmission rejects malformed known references and omits unknown versions.
func NormalizeForSubmission(references []apiv1.EntityReference) ([]apiv1.EntityReference, error) {
	return normalize(references, true)
}

// NormalizePersisted decodes JSON-roundtripped metadata and omits malformed or unknown entries.
func NormalizePersisted(raw any) []apiv1.EntityReference {
	if raw == nil {
		return nil
	}
	references, ok := raw.([]apiv1.EntityReference)
	if !ok {
		encoded, err := json.Marshal(raw)
		if err != nil || json.Unmarshal(encoded, &references) != nil {
			return nil
		}
	}
	normalized, _ := normalize(references, false)
	return normalized
}

func normalize(references []apiv1.EntityReference, strict bool) ([]apiv1.EntityReference, error) {
	if len(references) > maxReferencesPerMessage {
		if strict {
			return nil, fmt.Errorf("%w: at most %d references are allowed", ErrInvalidReference, maxReferencesPerMessage)
		}
		references = references[:maxReferencesPerMessage]
	}
	result := make([]apiv1.EntityReference, 0, len(references))
	seen := make(map[string]struct{}, len(references))
	for index, reference := range references {
		if reference.Version != apiv1.EntityReferenceVersion {
			continue
		}
		normalized, ok := normalizeOne(reference)
		if !ok {
			if strict {
				return nil, fmt.Errorf("%w at index %d", ErrInvalidReference, index)
			}
			continue
		}
		if _, duplicate := seen[normalized.Ref]; duplicate {
			continue
		}
		seen[normalized.Ref] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func normalizeOne(reference apiv1.EntityReference) (apiv1.EntityReference, bool) {
	reference.Ref = strings.TrimSpace(reference.Ref)
	reference.Provider = strings.TrimSpace(reference.Provider)
	reference.Kind = strings.TrimSpace(reference.Kind)
	reference.ID = strings.TrimSpace(reference.ID)
	reference.Scope = strings.TrimSpace(reference.Scope)
	reference.Key = normalizeDisplayText(reference.Key)
	reference.Title = normalizeDisplayText(reference.Title)
	reference.URL = strings.TrimSpace(reference.URL)

	if !identityNamePattern.MatchString(reference.Provider) || !identityNamePattern.MatchString(reference.Kind) ||
		!validIdentity(reference.ID) || !validIdentity(reference.Scope) || reference.Title == "" ||
		utf8.RuneCountInString(reference.Title) > 500 || utf8.RuneCountInString(reference.Key) > 200 {
		return apiv1.EntityReference{}, false
	}
	if reference.Ref != CanonicalRef(reference.Provider, reference.Kind, reference.Scope, reference.ID) ||
		!safeURL(reference) {
		return apiv1.EntityReference{}, false
	}
	return reference, true
}

func validIdentity(value string) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 512 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func normalizeDisplayText(value string) string {
	if !utf8.ValidString(value) {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func safeURL(reference apiv1.EntityReference) bool {
	if reference.URL == "" || !utf8.ValidString(reference.URL) || utf8.RuneCountInString(reference.URL) > 2048 {
		return false
	}
	parsed, err := url.Parse(reference.URL)
	if err != nil || parsed.User != nil {
		return false
	}
	if reference.Provider == kandevProviderID && reference.Kind == kandevTaskKind {
		return reference.URL == "/t/"+url.PathEscape(reference.ID)
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
