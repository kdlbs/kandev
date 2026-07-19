package scriptengine

import (
	"regexp"
	"strings"
)

// ticketPlaceholder keys exposed by TicketContextProvider.
const (
	placeholderTicketID       = "TICKET_ID"
	placeholderTicketURL      = "TICKET_URL"
	placeholderTicketProvider = "TICKET_PROVIDER"
	placeholderTaskTitle      = "TASK_TITLE"
)

// Ticket-provider identifiers surfaced by TicketContextProvider under
// {{TICKET_PROVIDER}}. Kept as constants so ResolveStartupPrompt and its
// tests share the same wire format.
const (
	TicketProviderJira   = "jira"
	TicketProviderLinear = "linear"
)

// placeholderToken matches any `{{...}}` sequence. Used by ResolveStartupPrompt
// to inspect the ORIGINAL line's tokens against the resolver's known keys, so
// substituted values that happen to contain a `{{...}}` literal (e.g. a task
// title like "Investigate {{BUG-123}}") don't trigger a false drop.
var placeholderToken = regexp.MustCompile(`\{\{([^}]*)\}\}`)

// hasUnknownPlaceholder reports whether the line references at least one
// `{{KEY}}` whose KEY is not in the known-vars map. Those are genuine
// unresolved placeholders and their line is dropped by ResolveStartupPrompt.
func hasUnknownPlaceholder(line string, known map[string]string) bool {
	for _, match := range placeholderToken.FindAllStringSubmatch(line, -1) {
		if _, ok := known[match[1]]; !ok {
			return true
		}
	}
	return false
}

// TicketContextProvider builds placeholders for a repository startup prompt
// from a task's title and metadata. TASK_TITLE is always present (even when
// empty). Ticket keys are only set when the metadata carries a matching
// provider — leaving them absent lets ResolveStartupPrompt drop lines whose
// only signal was ticket context.
//
// Jira wins over Linear when both are present so a task copied across
// providers picks the newer coordinate rather than a stale one.
func TicketContextProvider(taskTitle string, metadata map[string]interface{}) PlaceholderProvider {
	return func() map[string]string {
		vars := map[string]string{placeholderTaskTitle: taskTitle}
		if id, url, ok := jiraTicketFrom(metadata); ok {
			vars[placeholderTicketID] = id
			vars[placeholderTicketURL] = url
			vars[placeholderTicketProvider] = TicketProviderJira
			return vars
		}
		if id, url, ok := linearTicketFrom(metadata); ok {
			vars[placeholderTicketID] = id
			vars[placeholderTicketURL] = url
			vars[placeholderTicketProvider] = TicketProviderLinear
			return vars
		}
		return vars
	}
}

func jiraTicketFrom(metadata map[string]interface{}) (id, url string, ok bool) {
	if metadata == nil {
		return "", "", false
	}
	key := getMetaString(metadata, "jira_issue_key")
	if key == "" {
		return "", "", false
	}
	return key, getMetaString(metadata, "jira_issue_url"), true
}

func linearTicketFrom(metadata map[string]interface{}) (id, url string, ok bool) {
	if metadata == nil {
		return "", "", false
	}
	ident := getMetaString(metadata, "linear_issue_identifier")
	if ident == "" {
		return "", "", false
	}
	return ident, getMetaString(metadata, "linear_issue_url"), true
}

// ResolveStartupPrompt substitutes {{PLACEHOLDER}} tokens in a repository
// startup prompt using ticket metadata and the task title. Lines that still
// contain an unresolved `{{...}}` after substitution are removed entirely so
// the caller never sees a raw placeholder in the task description. Leading
// and trailing blank lines are trimmed from the result.
//
// Callers are expected to have already fallen back to the caller-supplied
// description whenever the raw prompt is empty; this helper does not
// distinguish "empty prompt" from "prompt resolved to nothing".
func ResolveStartupPrompt(prompt, taskTitle string, metadata map[string]interface{}) string {
	if prompt == "" {
		return ""
	}
	resolver := NewResolver().WithProvider(TicketContextProvider(taskTitle, metadata))
	// Snapshot the known placeholder keys once so we can decide per-line
	// whether an original {{...}} token has a mapping. Checking against the
	// original line's tokens — not the resolved string — prevents false drops
	// when a substituted value (e.g. a task title like "Investigate
	// {{BUG-123}}") happens to contain a {{...}} literal.
	known := resolver.mergedVars()
	// Normalize CRLF to LF so prompts saved by Windows editors don't leave a
	// stray \r in each resolved line.
	lines := strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if hasUnknownPlaceholder(line, known) {
			continue
		}
		kept = append(kept, resolver.Resolve(line))
	}
	// Trim only newlines — preserve any leading/trailing spaces or tabs a
	// user intentionally put on the first or last kept line (e.g. indented
	// bullet content). Collapse to "" when the whole result is blank so
	// callers that gate on the length see an accurate "no prompt to apply"
	// signal.
	trimmed := strings.Trim(strings.Join(kept, "\n"), "\n")
	if strings.TrimSpace(trimmed) == "" {
		return ""
	}
	return trimmed
}
