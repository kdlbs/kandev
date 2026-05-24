package automation

import (
	"encoding/json"
	"fmt"
	"regexp"
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

	// Add generic data.* placeholders for any top-level key.
	for k, v := range data {
		pairs = append(pairs, fmt.Sprintf("{{data.%s}}", k), toString(v))
	}

	result := strings.NewReplacer(pairs...).Replace(prompt)
	return stripUnresolved(result)
}

// unresolvedRe matches leftover {{placeholder}} tokens that weren't replaced.
var unresolvedRe = regexp.MustCompile(`\{\{[a-z_.]+\}\}`)

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
