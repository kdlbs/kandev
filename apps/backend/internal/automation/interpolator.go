package automation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// InterpolatePrompt replaces {{placeholder}} tokens in the prompt template
// with values from the trigger data. Supports nested access via dot notation.
func InterpolatePrompt(prompt string, triggerType TriggerType, triggerData json.RawMessage) string {
	if prompt == "" || !strings.Contains(prompt, "{{") {
		return prompt
	}

	// Parse trigger data into a generic map for lookups.
	var data map[string]interface{}
	if err := json.Unmarshal(triggerData, &data); err != nil {
		data = make(map[string]interface{})
	}

	// Build replacer pairs from common placeholders.
	pairs := []string{
		"{{trigger.type}}", string(triggerType),
		"{{trigger.timestamp}}", time.Now().UTC().Format(time.RFC3339),
	}

	// Add trigger-type-specific placeholders.
	switch triggerType {
	case TriggerTypeGitHubPR:
		pairs = append(pairs, prPlaceholders(data)...)
	case TriggerTypeGitHubPush:
		pairs = append(pairs, pushPlaceholders(data)...)
	case TriggerTypeGitHubCI:
		pairs = append(pairs, ciPlaceholders(data)...)
	case TriggerTypeWebhook:
		pairs = append(pairs, webhookPlaceholders(data)...)
	}

	result := strings.NewReplacer(pairs...).Replace(prompt)
	// Resolve {{data.<path>}} and {{webhook.<path>}} tokens that didn't
	// match the fixed list above. Dot-segments traverse nested objects;
	// numeric segments index arrays (e.g. commits.0.message).
	result = resolvePathPlaceholders(result, data)
	return stripUnresolved(result)
}

// pathPlaceholderRe matches {{data.<path>}} or {{webhook.<path>}} tokens.
// Path segments are dot-separated and may contain letters, digits,
// underscores, dots, and hyphens — matching the JSON-key shapes external
// systems actually emit (e.g. kebab-case headers like x-request-id).
// The hyphen is placed last in the character class to avoid range interpretation.
var pathPlaceholderRe = regexp.MustCompile(`\{\{(data|webhook)\.([a-zA-Z0-9_.-]+)\}\}`)

// resolvePathPlaceholders walks every remaining {{data.<path>}} and
// {{webhook.<path>}} token and substitutes the value at that path in data.
// Missing paths are left in place so stripUnresolved can clear them.
func resolvePathPlaceholders(s string, data map[string]interface{}) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return pathPlaceholderRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := pathPlaceholderRe.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		val, ok := lookupPath(data, parts[2])
		if !ok {
			return match
		}
		return val
	})
}

// lookupPath resolves a dot-separated path against the parsed JSON payload.
// Numeric segments index into JSON arrays; non-numeric segments key into
// objects. Returns ("", false) when a segment can't be resolved (missing
// key, out-of-range index, or a scalar reached before the path ends).
// Non-leaf nodes (intermediate objects/arrays) are JSON-marshalled to a
// string via toString — same convention as {{webhook.body}} returning the
// whole payload.
func lookupPath(data map[string]interface{}, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	var cur interface{} = data
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]interface{}:
			next, ok := node[seg]
			if !ok {
				return "", false
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return "", false
			}
			cur = node[idx]
		default:
			return "", false
		}
	}
	if cur == nil {
		return "", false
	}
	return toString(cur), true
}

// unresolvedRe matches leftover {{placeholder}} tokens that weren't replaced.
// Includes hyphens (placed last to avoid range interpretation) so kebab-case
// keys like x-request-id are stripped rather than leaking into the prompt.
var unresolvedRe = regexp.MustCompile(`\{\{[a-zA-Z0-9_.-]+\}\}`)

// stripUnresolved removes any remaining {{...}} placeholders so they don't
// appear as raw text in the agent prompt.
func stripUnresolved(s string) string {
	return strings.TrimSpace(unresolvedRe.ReplaceAllString(s, ""))
}

func prPlaceholders(data map[string]interface{}) []string {
	return []string{
		"{{pr.number}}", toString(data["number"]),
		"{{pr.title}}", toString(data["title"]),
		"{{pr.url}}", toString(data["html_url"]),
		"{{pr.author}}", toString(data["author_login"]),
		"{{pr.repo}}", toString(data["repo"]),
		"{{pr.branch}}", toString(data["head_branch"]),
		"{{pr.base_branch}}", toString(data["base_branch"]),
		"{{pr.body}}", toString(data["body"]),
	}
}

func pushPlaceholders(data map[string]interface{}) []string {
	return []string{
		"{{push.branch}}", toString(data["branch"]),
		"{{push.repo}}", toString(data["repo"]),
		"{{push.sha}}", toString(data["sha"]),
		"{{push.message}}", toString(data["message"]),
	}
}

func ciPlaceholders(data map[string]interface{}) []string {
	return []string{
		"{{ci.check_name}}", toString(data["check_name"]),
		"{{ci.conclusion}}", toString(data["conclusion"]),
		"{{ci.repo}}", toString(data["repo"]),
		"{{ci.url}}", toString(data["html_url"]),
	}
}

func webhookPlaceholders(data map[string]interface{}) []string {
	raw, _ := json.Marshal(data)
	return []string{
		"{{webhook.body}}", string(raw),
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
