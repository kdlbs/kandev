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
