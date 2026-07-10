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

// unresolvedPlaceholder matches any remaining `{{...}}` sequence after
// substitution. Used by ResolveStartupPrompt to drop lines whose ticket
// placeholders never resolved.
var unresolvedPlaceholder = regexp.MustCompile(`\{\{[^}]*\}\}`)

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
			vars[placeholderTicketProvider] = "jira"
			return vars
		}
		if id, url, ok := linearTicketFrom(metadata); ok {
			vars[placeholderTicketID] = id
			vars[placeholderTicketURL] = url
			vars[placeholderTicketProvider] = "linear"
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
	// Normalize CRLF to LF so prompts saved by Windows editors don't leave a
	// stray \r in each resolved line.
	lines := strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		resolved := resolver.Resolve(line)
		if unresolvedPlaceholder.MatchString(resolved) {
			continue
		}
		kept = append(kept, resolved)
	}
	return strings.Trim(strings.Join(kept, "\n"), "\n \t")
}
