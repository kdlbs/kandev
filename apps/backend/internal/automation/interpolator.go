package automation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// placeholderRe matches every {{token}} in a template, capturing the token
// text. A single pass over the template drives the whole substitution: the
// callback below never re-scans its own output, so a payload value that
// happens to contain literal {{...}} text can never be expanded a second
// time (see InterpolateAgentPrompt's quoting, which depends on this).
var placeholderRe = regexp.MustCompile(`\{\{([a-zA-Z0-9_.-]+)\}\}`)

// InterpolatePrompt replaces {{placeholder}} tokens in a template with values
// from the trigger data, without quoting. Used for run display titles, which
// are plain text shown in the runs list rather than an instruction channel to
// an agent — quoting there would let a fence character leak into a truncated
// title.
func InterpolatePrompt(prompt string, triggerType TriggerType, triggerData json.RawMessage) string {
	return interpolate(prompt, triggerType, triggerData, false)
}

// InterpolateAgentPrompt replaces {{placeholder}} tokens for the prompt sent
// to the agent. On the webhook trigger, every substituted payload value is
// quoted (inline code span, or a fenced block for a value containing a
// newline) so that no payload-derived text can be mistaken for prompt syntax.
// Other trigger types render exactly as InterpolatePrompt — hardening their
// payload-derived tokens is out of scope for this change.
func InterpolateAgentPrompt(prompt string, triggerType TriggerType, triggerData json.RawMessage) string {
	return interpolate(prompt, triggerType, triggerData, triggerType == TriggerTypeWebhook)
}

func interpolate(prompt string, triggerType TriggerType, triggerData json.RawMessage, quoteValues bool) string {
	if prompt == "" || !strings.Contains(prompt, "{{") {
		return prompt
	}

	var data map[string]interface{}
	if err := json.Unmarshal(triggerData, &data); err != nil {
		data = make(map[string]interface{})
	}
	fixed := fixedTriggerPlaceholders(triggerType, data)
	now := time.Now().UTC().Format(time.RFC3339)

	result := placeholderRe.ReplaceAllStringFunc(prompt, func(match string) string {
		token := match[2 : len(match)-2]

		switch token {
		case "trigger.type":
			return string(triggerType)
		case "trigger.timestamp":
			return now
		}
		// A plugin event's payload is an envelope — data["webhook"] is the
		// original third-party payload and data["data"] is the plugin's
		// normalized view — so "webhook."/"data." select a different root
		// object each, not just a namespacing convention. This is the one
		// trigger type where that distinction matters; every other type
		// resolves both prefixes against the same top-level data below.
		if triggerType == TriggerTypePluginEvent {
			if token == "webhook.body" {
				original, _ := data["webhook"].(map[string]interface{})
				raw, _ := json.Marshal(original)
				if !quoteValues {
					return string(raw)
				}
				return quoteFenced(string(raw), 3)
			}
			if value, ok := resolvePluginEventToken(token, data); ok {
				return quoteOrPlain(value, quoteValues)
			}
			return ""
		}
		// webhook.body always means the whole payload, substituted from the
		// original bytes (not a re-marshal of the parsed map, which loses
		// non-object payloads). This carve-out only applies on the webhook
		// trigger; on any other trigger type the token falls through to the
		// data./webhook. path branch below and resolves a top-level "body"
		// field instead — a deliberate no-op preserving today's behavior.
		if triggerType == TriggerTypeWebhook && token == "webhook.body" {
			raw := string(triggerData)
			if !quoteValues {
				return raw
			}
			return quoteFenced(raw, 3)
		}
		if v, ok := fixed[token]; ok {
			return v
		}
		if value, ok := resolveDataOrWebhookToken(token, data); ok {
			return quoteOrPlain(value, quoteValues)
		}
		return ""
	})
	return strings.TrimSpace(result)
}

// dataPathTokenRe matches the "data.<path>" or "webhook.<path>" shape of a
// captured placeholder token (braces already stripped), requiring at least
// one further segment after the prefix.
var dataPathTokenRe = regexp.MustCompile(`^(?:data|webhook)\.(.+)$`)

// resolveDataOrWebhookToken resolves a "data.<path>" or "webhook.<path>"
// token against the parsed payload via lookupPath. Available for any trigger
// type — the prefix is just a namespacing convention, not a type gate.
func resolveDataOrWebhookToken(token string, data map[string]interface{}) (string, bool) {
	m := dataPathTokenRe.FindStringSubmatch(token)
	if m == nil {
		return "", false
	}
	return lookupPath(data, m[1])
}

// pluginEventTokenRe matches "data.<path>" or "webhook.<path>", capturing the
// prefix separately from the suffix: for a plugin event the prefix selects
// which top-level envelope object ("data" or "webhook") is the lookup root.
var pluginEventTokenRe = regexp.MustCompile(`^(data|webhook)\.(.+)$`)

// resolvePluginEventToken resolves a "data.<path>" or "webhook.<path>" token
// against a plugin event's envelope payload — data["data"] or
// data["webhook"] respectively, not the top-level payload itself.
func resolvePluginEventToken(token string, data map[string]interface{}) (string, bool) {
	m := pluginEventTokenRe.FindStringSubmatch(token)
	if m == nil {
		return "", false
	}
	root, _ := data[m[1]].(map[string]interface{})
	return lookupPath(root, m[2])
}

// lookupPath resolves a dot-separated path against the parsed JSON payload.
// Numeric segments index into JSON arrays; non-numeric segments key into
// objects. Returns ("", false) when a segment can't be resolved (missing
// key, out-of-range index, a scalar reached before the path ends, or a JSON
// null). Non-leaf nodes (intermediate objects/arrays) are JSON-marshalled to
// a string via toString.
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

// ResolvePayloadPath resolves a dot path against a raw JSON payload,
// trimming the result and treating a present-but-empty value as unresolved.
// This is the shared trim-then-test semantics used by dedup key resolution
// (webhook.go) and repository selector resolution (orchestrator package).
func ResolvePayloadPath(triggerData json.RawMessage, path string) (value string, ok bool) {
	if path == "" {
		return "", false
	}
	var data map[string]interface{}
	if err := json.Unmarshal(triggerData, &data); err != nil {
		return "", false
	}
	raw, found := lookupPath(data, path)
	if !found {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	return trimmed, trimmed != ""
}

func fixedTriggerPlaceholders(triggerType TriggerType, data map[string]interface{}) map[string]string {
	switch triggerType {
	case TriggerTypeGitHubPR:
		return prPlaceholders(data)
	case TriggerTypeGitHubPush:
		return pushPlaceholders(data)
	case TriggerTypeGitHubCI:
		return ciPlaceholders(data)
	default:
		return nil
	}
}

func prPlaceholders(data map[string]interface{}) map[string]string {
	return map[string]string{
		"pr.number":      toString(data["number"]),
		"pr.title":       toString(data["title"]),
		"pr.url":         toString(data[automationHTMLURLKey]),
		"pr.author":      toString(data[automationAuthorLoginKey]),
		"pr.repo":        toString(data[automationRepoKey]),
		"pr.branch":      toString(data[automationHeadBranchKey]),
		"pr.base_branch": toString(data[automationBaseBranchKey]),
		"pr.body":        toString(data[automationBodyKey]),
	}
}

func pushPlaceholders(data map[string]interface{}) map[string]string {
	return map[string]string{
		"push.branch":  toString(data["branch"]),
		"push.repo":    toString(data[automationRepoKey]),
		"push.sha":     toString(data["sha"]),
		"push.message": toString(data["message"]),
	}
}

func ciPlaceholders(data map[string]interface{}) map[string]string {
	return map[string]string{
		"ci.check_name": toString(data["check_name"]),
		"ci.conclusion": toString(data["conclusion"]),
		"ci.repo":       toString(data[automationRepoKey]),
		"ci.url":        toString(data[automationHTMLURLKey]),
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

// quoteOrPlain quotes value as an inline code span when quoteValues is true,
// otherwise returns it unchanged.
func quoteOrPlain(value string, quoteValues bool) string {
	if !quoteValues {
		return value
	}
	return quoteInline(value)
}

// longestBacktickRun returns the length of the longest run of consecutive
// backtick characters in value.
func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, r := range value {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	return longest
}

// codeFence returns a backtick run one longer than the longest run already
// present in value, floored at floor — so the fence's own delimiters can
// never appear, unescaped, inside the content they wrap.
func codeFence(value string, floor int) string {
	n := longestBacktickRun(value) + 1
	if n < floor {
		n = floor
	}
	return strings.Repeat("`", n)
}

// quoteInline wraps value in a Markdown inline code span. A value containing
// a newline cannot be represented inline, so it falls back to the fenced
// form instead.
func quoteInline(value string) string {
	if strings.Contains(value, "\n") {
		return quoteFenced(value, 3)
	}
	fence := codeFence(value, 1)
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		return fence + " " + value + " " + fence
	}
	return fence + value + fence
}

// quoteFenced wraps value in a Markdown fenced code block.
func quoteFenced(value string, floor int) string {
	fence := codeFence(value, floor)
	return fence + "\n" + value + "\n" + fence
}
