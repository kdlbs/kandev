package automation

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExportAutomationIncludesRetryPolicyConfigurationOnly(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Export retry policy",
		RetryPolicy: RetryPolicy{
			Mode:         RetryModeFinite,
			MaxRetries:   "3",
			DelaySeconds: "60",
			Backoff:      RetryBackoffExponential,
			HistoryMode:  RetryHistoryTimeline,
		},
	})

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("export automation: %v", err)
	}
	var doc struct {
		Automations []struct {
			RetryPolicy struct {
				Mode         RetryMode        `yaml:"mode"`
				MaxRetries   string           `yaml:"max_retries"`
				DelaySeconds string           `yaml:"delay_seconds"`
				Backoff      RetryBackoff     `yaml:"backoff"`
				HistoryMode  RetryHistoryMode `yaml:"history_mode"`
			} `yaml:"retry_policy"`
		} `yaml:"automations"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(doc.Automations) != 1 {
		t.Fatalf("exported automations = %d, want 1", len(doc.Automations))
	}
	got := doc.Automations[0].RetryPolicy
	if got.Mode != RetryModeFinite || got.MaxRetries != "3" || got.DelaySeconds != "60" ||
		got.Backoff != RetryBackoffExponential || got.HistoryMode != RetryHistoryTimeline {
		t.Fatalf("exported retry policy = %#v", got)
	}
	for _, runtimeField := range []string{"retry_state", "retry_scheduled_at", "task_id", "session_id", "turn_id"} {
		if strings.Contains(string(body), runtimeField) {
			t.Errorf("export contains runtime field %q: %s", runtimeField, body)
		}
	}
}
