package scheduler

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func TestFindMentionedAgentsKeepsEscapedExtractMentionsLiteral(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ctx := context.Background()
	for _, agent := range []*models.AgentInstance{
		{ID: "agent-qa", WorkspaceID: "ws-1", Name: "QA", Status: models.AgentStatusIdle},
		{
			ID: "550e8400-e29b-41d4-a716-446655440000", WorkspaceID: "ws-1",
			Name: "Profile", Status: models.AgentStatusIdle,
		},
	} {
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent %s: %v", agent.ID, err)
		}
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	ss := NewSchedulerService(repo, log, service.NewService(service.ServiceOptions{Repo: repo, Logger: log}))

	sourceExcerpt := `### Agent Message Comments

Selected comment from Worker:
> \@QA, read \&commat;QA, and open [the profile](agent:\/\/550e8400-e29b-41d4-a716-446655440000).
> agent:\&#x2F;\&#x2F;550e8400-e29b-41d4-a716-446655440000

---`
	mentioned, err := ss.FindMentionedAgents(ctx, "ws-1", sourceExcerpt)
	if err != nil {
		t.Fatalf("resolve escaped source mentions: %v", err)
	}
	if len(mentioned) != 0 {
		t.Fatalf("source excerpt mentions = %v, want none", mentioned)
	}

	userFeedback := "User feedback:\n> Please ask @QA, and verify this again."
	mentioned, err = ss.FindMentionedAgents(ctx, "ws-1", userFeedback)
	if err != nil {
		t.Fatalf("resolve user feedback mention: %v", err)
	}
	if len(mentioned) != 1 || mentioned[0] != "agent-qa" {
		t.Fatalf("user feedback mentions = %v, want [agent-qa]", mentioned)
	}
}
