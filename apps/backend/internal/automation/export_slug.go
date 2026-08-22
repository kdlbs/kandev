package automation

import "strings"

// automationSlugFallback is AC-26's slug for a name that derives to an empty
// string (no ASCII letters or digits at all).
const automationSlugFallback = "automation"

// deriveAutomationSlug renders an automation name into an AC-25/26 zip entry
// slug: lowercase, every character outside [a-z0-9] becomes '-', consecutive
// '-' collapse to one, leading and trailing '-' trim, then truncate to 64
// characters followed by a further trailing-'-' trim. An empty result (the
// name has no ASCII letters or digits at all) falls back to
// automationSlugFallback.
func deriveAutomationSlug(name string) string {
	lower := strings.ToLower(name)

	var b strings.Builder
	b.Grow(len(lower))
	lastDash := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if len(slug) > 64 {
		slug = strings.TrimRight(slug[:64], "-")
	}
	if slug == "" {
		return automationSlugFallback
	}
	return slug
}

// automationSlugSource is what uniqueAutomationSlugs needs per automation to
// derive its base slug (AC-25/26) and, on collision, its AC-27 suffix.
type automationSlugSource struct {
	ID   string
	Name string
}

// uniqueAutomationSlugs resolves each automation's zip-entry slug — the
// "<slug>" in ".kandev/automations/<slug>.yml" — applying AC-27's
// suffix-and-widen collision algorithm: every automation sharing a base slug
// with another is suffixed with the first 8 characters of its id, applied to
// every member of the group so none keeps the bare slug. If any resulting
// name still collides with any other entry in the export — suffixed or
// not — the still-colliding entries' suffixes widen by 8 more characters of
// id, up to the full id, until every entry is unique.
//
// Termination is guaranteed by id uniqueness: once an entry's suffix reaches
// its automation's full id, no other automation can produce that same
// suffixed name, so the number of still-colliding entries can only shrink
// across rounds.
func uniqueAutomationSlugs(sources []automationSlugSource) map[string]string {
	base := make(map[string]string, len(sources))
	for _, s := range sources {
		base[s.ID] = deriveAutomationSlug(s.Name)
	}

	counts := make(map[string]int, len(sources))
	for _, s := range sources {
		counts[base[s.ID]]++
	}

	level := make(map[string]int, len(sources))
	for _, s := range sources {
		if counts[base[s.ID]] > 1 {
			level[s.ID] = 8
		}
	}

	for {
		names := make(map[string][]string, len(sources))
		for _, s := range sources {
			name := suffixedAutomationSlug(base[s.ID], s.ID, level[s.ID])
			names[name] = append(names[name], s.ID)
		}

		collided := false
		for _, ids := range names {
			if len(ids) <= 1 {
				continue
			}
			collided = true
			for _, id := range ids {
				next := level[id] + 8
				if next > len(id) {
					next = len(id)
				}
				level[id] = next
			}
		}
		if !collided {
			break
		}
	}

	result := make(map[string]string, len(sources))
	for _, s := range sources {
		result[s.ID] = suffixedAutomationSlug(base[s.ID], s.ID, level[s.ID])
	}
	return result
}

func suffixedAutomationSlug(base, id string, level int) string {
	if level <= 0 {
		return base
	}
	n := level
	if n > len(id) {
		n = len(id)
	}
	return base + "-" + id[:n]
}
