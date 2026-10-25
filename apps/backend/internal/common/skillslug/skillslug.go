// Package skillslug is the single source of truth for the office-skill
// slug naming rule shared by the skill service (write-time validation and
// normalization), the local injector, and the Sprites remote delivery
// path. It has no Kandev imports so every layer that needs the rule can
// depend on it without pulling in persistence or runtime code.
package skillslug

import "regexp"

// Prefix marks a slug (and the on-disk directory derived from it) as
// Kandev-owned.
const Prefix = "kandev-"

var wellFormedRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// WellFormed reports whether slug is a non-empty string made up only of
// ASCII letters, digits, underscore, and hyphen. This is the safety
// predicate: a well-formed slug is always safe to use as a single path
// component.
func WellFormed(slug string) bool {
	return wellFormedRe.MatchString(slug)
}

// Canonical reports whether slug is well-formed and carries the Kandev
// prefix. Every canonical slug is well-formed; not every well-formed slug
// is canonical.
func Canonical(slug string) bool {
	return WellFormed(slug) && len(slug) >= len(Prefix) && slug[:len(Prefix)] == Prefix
}

// Normalize returns the canonical form of slug: unchanged if it already
// carries the prefix, prefixed otherwise. Normalize is idempotent.
//
// Callers must check WellFormed(slug) before calling Normalize. Normalize
// does not itself reject a not-well-formed slug — in particular
// Normalize("") returns Prefix, a well-formed value — so validating first
// is load-bearing, not just good practice.
func Normalize(slug string) string {
	if len(slug) >= len(Prefix) && slug[:len(Prefix)] == Prefix {
		return slug
	}
	return Prefix + slug
}
