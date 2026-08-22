package automation

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// escapeWarningText applies AC-42's four rendering rules to automation.name
// or automation_triggers.type before either is interpolated into a warning
// message. The rules operate on disjoint inputs (control characters below
// U+0080 plus U+0085, exactly U+2028/U+2029, and invalid UTF-8 bytes), so the
// order they are applied in cannot change the result.
func escapeWarningText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, `\x%02x`, s[i])
			i++
			continue
		}
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\u2028' || r == '\u2029':
			fmt.Fprintf(&b, `\u%04x`, r)
		case r <= 0x1F || r == 0x7F || r == 0x85:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// buildWarningsList deduplicates, orders, and renders the warnings collected
// during export into the strings that populate the exported document's
// top-level `warnings` sequence (AC-20) / WARNINGS.txt.
//
// Dedup (per exportWarning.DedupKey) precedes ordering: AC-21's sort must see
// only the surviving, de-duplicated set. Ordering is AC-21: automation name
// ascending (raw, matching AC-8's own automation sort), tiebroken by
// automations.id ascending, then by the warning message ascending — which
// only ever discriminates between two warnings for the very same automation,
// since name+id together already identify it uniquely. Rendering is AC-42:
// `<escaped automation name>: <message>`, where message already carries any
// AC-42-escaped interpolation (trigger type) a caller embedded in it.
func buildWarningsList(warnings []exportWarning) []string {
	deduped := dedupExportWarnings(warnings)
	sort.Slice(deduped, func(i, j int) bool {
		a, b := deduped[i], deduped[j]
		if a.AutomationName != b.AutomationName {
			return a.AutomationName < b.AutomationName
		}
		if a.AutomationID != b.AutomationID {
			return a.AutomationID < b.AutomationID
		}
		return a.Message < b.Message
	})
	lines := make([]string, len(deduped))
	for i, w := range deduped {
		lines[i] = fmt.Sprintf("%s: %s", escapeWarningText(w.AutomationName), w.Message)
	}
	return lines
}

// dedupExportWarnings drops a warning whose (DedupKey, Message) pair has
// already been seen. The scope is per DedupKey — an automation ID for
// automation-scoped messages, a trigger ID for trigger-scoped ones — never
// global: automation names are not unique and trigger types are not unique
// per automation, so two entities that happen to produce byte-identical
// warning strings each keep their own line.
func dedupExportWarnings(warnings []exportWarning) []exportWarning {
	type key struct{ dedupKey, message string }
	seen := make(map[key]bool, len(warnings))
	out := make([]exportWarning, 0, len(warnings))
	for _, w := range warnings {
		k := key{w.DedupKey, w.Message}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, w)
	}
	return out
}
