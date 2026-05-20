package scheduler

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// uuidLinkRe matches UUID-form mentions emitted by rich-text editors:
// `[Display](agent://550e8400-e29b-41d4-a716-446655440000)`. The
// agent's UUID is captured directly, bypassing name matching.
var uuidLinkRe = regexp.MustCompile(`agent://([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

// mentionTokenRe matches `@name` tokens. Allows letters, digits, dashes,
// underscores, and single internal spaces inside an agent name (CEO,
// "Code Reviewer", etc.). The capture is trimmed/lowercased before lookup.
var mentionTokenRe = regexp.MustCompile(`@([A-Za-z][A-Za-z0-9_\- ]{0,63})`)

// FindMentionedAgents extracts @mentions from a comment body and resolves
// each token against agent display names in the workspace. Resolution is
// case-insensitive; all collisions win (two agents sharing a name both
// get woken). Also resolves UUID-form `[Display](agent://uuid)` links
// directly. Returns a deduped list of agent IDs.
//
// HTML entities in the body (`&amp;`, `&#x20;`, etc.) are decoded before
// matching so rich-text editors that escape special chars don't break
// mention detection.
func (ss *SchedulerService) FindMentionedAgents(
	ctx context.Context, workspaceID, body string,
) ([]string, error) {
	if body == "" {
		return nil, nil
	}
	decoded := html.UnescapeString(body)

	// Direct UUID matches.
	uuidSet := map[string]struct{}{}
	for _, m := range uuidLinkRe.FindAllStringSubmatch(decoded, -1) {
		if len(m) > 1 {
			uuidSet[strings.ToLower(m[1])] = struct{}{}
		}
	}

	// Name tokens (lowercase, deduped).
	tokens := map[string]struct{}{}
	for _, m := range mentionTokenRe.FindAllStringSubmatch(decoded, -1) {
		if len(m) > 1 {
			t := strings.TrimSpace(strings.ToLower(m[1]))
			if t != "" {
				tokens[t] = struct{}{}
			}
		}
	}

	if len(uuidSet) == 0 && len(tokens) == 0 {
		return nil, nil
	}

	// Single SELECT for the workspace's agents.
	agents, err := ss.svc.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list agents for mentions: %w", err)
	}

	resolved := map[string]struct{}{}
	for _, a := range agents {
		if _, ok := uuidSet[strings.ToLower(a.ID)]; ok {
			resolved[a.ID] = struct{}{}
			continue
		}
		if _, ok := tokens[strings.ToLower(a.Name)]; ok {
			resolved[a.ID] = struct{}{}
		}
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(resolved))
	for id := range resolved {
		out = append(out, id)
	}
	return out, nil
}
