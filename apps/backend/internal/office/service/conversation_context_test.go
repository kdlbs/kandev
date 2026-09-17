package service

import (
	"github.com/kandev/kandev/internal/office/models"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConversationExcerptPreservesUTF8AndSignalsOmission(t *testing.T) {
	text := strings.Repeat("界", 3000)
	got := clipConversationText(text, 1000)
	if !utf8.ValidString(got) || len(got) > 1100 {
		t.Fatalf("invalid or unbounded excerpt: %d bytes", len(got))
	}
	if !strings.Contains(got, "full content remains") {
		t.Fatal("truncation must be visible to the coordinator")
	}
	if got := clipConversationText("short", 1000); got != "short" {
		t.Fatalf("short message changed: %q", got)
	}
}

func TestDelegationRosterIncludesGuidanceAndExcludesForeignOrPausedAgents(t *testing.T) {
	agents := []*models.AgentInstance{
		{ID: "chief", WorkspaceID: "ws", Name: "Chief"},
		{ID: "work", WorkspaceID: "ws", Name: "Work", Role: models.AgentRoleWorker, Settings: `{"delegation_context":"Use for work backend tasks"}`},
		{ID: "foreign", WorkspaceID: "other", Name: "Foreign"},
		{ID: "paused", WorkspaceID: "ws", Name: "Paused", Status: models.AgentStatusPaused},
	}
	got := buildDelegationRoster(agents, "ws", "chief")
	if !strings.Contains(got, "Use for work backend tasks") || !strings.Contains(got, "work") {
		t.Fatal(got)
	}
	if strings.Contains(got, "Foreign") || strings.Contains(got, "Paused") || strings.Contains(got, "Chief") {
		t.Fatal(got)
	}
}

func TestOrchestrationRosterUsesProfilesAndOwnRoutingGuidance(t *testing.T) {
	profiles := []map[string]string{{"id": "personal", "name": "Personal Claude", "environment": "secret-not-to-render"}, {"id": "work", "name": "Work Claude"}}
	got := buildOrchestrationRoster(profiles, "Use work for issue tracking; personal for Garden Notes.")
	if !strings.Contains(got, `"profile_id":"personal"`) || !strings.Contains(got, "Use work for issue tracking") {
		t.Fatalf("missing profile routing: %s", got)
	}
	if strings.Contains(got, "secret-not-to-render") || strings.Contains(got, `"agent_id"`) {
		t.Fatalf("unexpected profile details: %s", got)
	}
}
