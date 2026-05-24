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
		"number":       42,
		"title":        "Fix the bug",
		"html_url":     "https://github.com/org/repo/pull/42",
		"author_login": "alice",
		"repo":         "org/repo",
		"head_branch":  "fix-bug",
		"base_branch":  "main",
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
		]
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
