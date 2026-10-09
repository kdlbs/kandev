package automation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInterpolatePrompt_Scheduled(t *testing.T) {
	data, _ := json.Marshal(map[string]string{"source": "scheduled", "timestamp": "2026-03-08T12:00:00Z"})
	result := InterpolatePrompt("Run at {{trigger.timestamp}} by {{trigger.type}}", TriggerTypeScheduled, data)
	if !strings.Contains(result, "scheduled") {
		t.Errorf("expected trigger type in result, got %q", result)
	}
	// timestamp is generated at call time, just verify it's replaced
	if strings.Contains(result, "{{trigger.timestamp}}") {
		t.Error("expected {{trigger.timestamp}} to be replaced")
	}
}

func TestInterpolatePrompt_PR(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"number":                42,
		"title":                 "Fix the bug",
		"html_url":              "https://github.com/org/repo/pull/42",
		"author_login":          "alice",
		automationRepoKey:       "org/repo",
		"head_branch":           "fix-bug",
		automationBaseBranchKey: defaultBranchMain,
	})
	prompt := "Review PR #{{pr.number}} '{{pr.title}}' by {{pr.author}} in {{pr.repo}}"
	result := InterpolatePrompt(prompt, TriggerTypeGitHubPR, data)
	if !strings.Contains(result, "#42") {
		t.Errorf("expected PR number, got %q", result)
	}
	if !strings.Contains(result, "Fix the bug") {
		t.Errorf("expected PR title, got %q", result)
	}
	if !strings.Contains(result, "alice") {
		t.Errorf("expected author, got %q", result)
	}
}

func TestInterpolatePrompt_Webhook(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"action": "deploy",
		"env":    "production",
	})
	prompt := "Webhook received: {{webhook.body}}, action={{data.action}}"
	result := InterpolatePrompt(prompt, TriggerTypeWebhook, data)
	if strings.Contains(result, "{{webhook.body}}") {
		t.Error("expected {{webhook.body}} to be replaced")
	}
	if !strings.Contains(result, "deploy") {
		t.Errorf("expected 'deploy' in result, got %q", result)
	}
}

func TestInterpolatePrompt_Empty(t *testing.T) {
	result := InterpolatePrompt("", TriggerTypeScheduled, json.RawMessage(`{}`))
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestInterpolatePrompt_NoPlaceholders(t *testing.T) {
	result := InterpolatePrompt("plain text", TriggerTypeScheduled, json.RawMessage(`{}`))
	if result != "plain text" {
		t.Errorf("expected 'plain text', got %q", result)
	}
}

func TestInterpolatePrompt_WebhookNestedPath(t *testing.T) {
	// Real-world webhook payloads carry nested fields like
	// pull_request.number, commits[0].message, alert.severity. Authors
	// should be able to template those directly into the prompt.
	body := []byte(`{
		"action": "opened",
		"pull_request": {
			"number": 17,
			"title": "Add webhook support",
			"user": {"login": "carol"}
		},
		"commits": [
			{"message": "first"},
			{"message": "second"}
		],
		"x-request-id": "abc-123"
	}`)

	cases := []struct {
		name     string
		template string
		want     string
	}{
		{"nested object", "PR #{{webhook.pull_request.number}}", "PR #17"},
		{"deeply nested", "by {{webhook.pull_request.user.login}}", "by carol"},
		{"array index", "first commit: {{webhook.commits.0.message}}", "first commit: first"},
		{"second array index", "second: {{webhook.commits.1.message}}", "second: second"},
		{"data alias", "action={{data.action}}", "action=opened"},
		{"data nested", "title={{data.pull_request.title}}", "title=Add webhook support"},
		{"missing path drops", "x={{webhook.missing.field}}", "x="},
		{"out-of-range index drops", "x={{webhook.commits.99.message}}", "x="},
		{"kebab-case key", "id={{webhook.x-request-id}}", "id=abc-123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InterpolatePrompt(tc.template, TriggerTypeWebhook, body)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// D1: a payload value containing backticks, a newline, and a literal
// "{{data.x}}" token must survive as inert quoted text — never expanded a
// second time, and never able to break out of its own fence.
func TestInterpolateAgentPrompt_HostilePayloadValueStaysInert(t *testing.T) {
	body := []byte(`{"x": "line one ` + "```" + ` and {{data.x}}\nline two"}`)
	result := InterpolateAgentPrompt("value: {{data.x}}", TriggerTypeWebhook, body)

	if strings.Contains(result, "{{data.x}}") == false {
		t.Fatalf("expected the literal token text to survive unexpanded inside the fence, got %q", result)
	}
	// The value contains a newline, so quoteInline falls back to a fenced
	// block; the fence itself must be strictly longer than the longest
	// backtick run already present in the value (3 backticks -> 4-backtick fence).
	if !strings.Contains(result, "````\n") {
		t.Fatalf("expected a 4-backtick fence to escape the payload's own 3-backtick run, got %q", result)
	}
	// The template itself only had one placeholder occurrence; the engine
	// must not have re-scanned its own substituted output for a second pass.
	if strings.Count(result, "value:") != 1 {
		t.Fatalf("expected exactly one substitution pass, got %q", result)
	}
}

// A1: the default webhook prompt renders the payload via {{webhook.body}}
// even when the payload has no top-level "body" field — it substitutes the
// original bytes, not a lookup of a "body" key.
func TestInterpolateAgentPrompt_DefaultWebhookPrompt_NoTopLevelBodyKey(t *testing.T) {
	info := GetTriggerTypeInfo(TriggerTypeWebhook)
	if info == nil {
		t.Fatal("expected webhook trigger type info to be registered")
	}
	payload := []byte(`{"event":"deploy","env":"production"}`)
	result := InterpolateAgentPrompt(info.DefaultPrompt, TriggerTypeWebhook, payload)

	if !strings.Contains(result, `"event":"deploy"`) {
		t.Fatalf("expected the raw payload to appear in the rendered prompt, got %q", result)
	}
	if strings.Contains(result, "{{webhook.body}}") {
		t.Fatal("expected {{webhook.body}} to be replaced")
	}
}

// A1: {{trigger.type}} and {{trigger.timestamp}} still render for the
// webhook trigger under the quoting/hardening path.
func TestInterpolateAgentPrompt_TriggerTypeAndTimestampStillRender(t *testing.T) {
	result := InterpolateAgentPrompt("{{trigger.type}} at {{trigger.timestamp}}", TriggerTypeWebhook, json.RawMessage(`{}`))
	if !strings.Contains(result, string(TriggerTypeWebhook)) {
		t.Fatalf("expected trigger type in result, got %q", result)
	}
	if strings.Contains(result, "{{trigger.timestamp}}") {
		t.Fatal("expected {{trigger.timestamp}} to be replaced")
	}
}

// A1: a JSON-array-shaped webhook body renders as that array, not coerced
// or unwrapped — {{webhook.body}} always substitutes the original bytes.
func TestInterpolateAgentPrompt_ArrayBodyRendersAsArray(t *testing.T) {
	payload := []byte(`[{"id":1},{"id":2}]`)
	result := InterpolateAgentPrompt("{{webhook.body}}", TriggerTypeWebhook, payload)
	if !strings.Contains(result, `[{"id":1},{"id":2}]`) {
		t.Fatalf("expected the raw JSON array to appear verbatim, got %q", result)
	}
}

// B6: github_pr_merged's default prompt renders {{data.task_id}} as a bare
// id with no code span — prompt hardening is scoped to the webhook trigger
// type only (InterpolateAgentPrompt gates quoting on TriggerTypeWebhook).
func TestInterpolateAgentPrompt_GitHubPRMerged_NoCodeSpan(t *testing.T) {
	info := GetTriggerTypeInfo(TriggerTypeGitHubPRMerged)
	if info == nil {
		t.Fatal("expected github_pr_merged trigger type info to be registered")
	}
	payload := []byte(`{"task_id":"t_01H8XK"}`)
	result := InterpolateAgentPrompt(info.DefaultPrompt, TriggerTypeGitHubPRMerged, payload)

	if !strings.Contains(result, "task id: t_01H8XK") {
		t.Fatalf("expected the bare task id with no code span, got %q", result)
	}
	if strings.Contains(result, "`t_01H8XK`") {
		t.Fatalf("expected no inline code span around the task id for a non-webhook trigger, got %q", result)
	}
}

func TestInterpolatePrompt_PathPlaceholderCoercion(t *testing.T) {
	// Non-string leaf values are coerced through toString. Booleans and
	// integers should round-trip in the obvious way.
	body := []byte(`{"count": 3, "ok": true, "ratio": 0.5}`)
	result := InterpolatePrompt(
		"count={{webhook.count}} ok={{webhook.ok}} ratio={{webhook.ratio}}",
		TriggerTypeWebhook,
		body,
	)
	if result != "count=3 ok=true ratio=0.5" {
		t.Errorf("unexpected coercion: %q", result)
	}
}
