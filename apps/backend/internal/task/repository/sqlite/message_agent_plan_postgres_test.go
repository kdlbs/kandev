package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresAgentPlanUpsertSerializesAcrossConnections(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	repoA := openPostgresRepo(t)
	secondDB := openSecondPostgresConnection(t, dsn, repoA.db)
	repoB, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatalf("new second postgres repo: %v", err)
	}
	observerDB := openSecondPostgresConnection(t, dsn, repoA.db)
	observer, err := NewWithDB(observerDB, observerDB, nil)
	if err != nil {
		t.Fatalf("new observer postgres repo: %v", err)
	}
	ctx := context.Background()
	seedPostgresSession(
		t, repoA, "task-agent-plan-pg", "session-agent-plan-pg", "turn-agent-plan-pg", time.Now().UTC(),
	)

	initial := postgresAgentPlanMessage("initial snapshot")
	if _, _, created, err := repoA.UpsertAgentPlanMessageWithConversationReceipt(ctx, initial); err != nil {
		t.Fatalf("create initial agent plan: %v", err)
	} else if !created {
		t.Fatal("initial agent plan was not created")
	}

	olderRead := make(chan struct{})
	releaseOlder := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseOlder) }) }
	t.Cleanup(release)
	repoA.agentPlanUpsertAfterRead = func() {
		close(olderRead)
		<-releaseOlder
	}

	type upsertResult struct {
		created bool
		err     error
	}
	olderDone := make(chan upsertResult, 1)
	go func() {
		_, _, created, upsertErr := repoA.UpsertAgentPlanMessageWithConversationReceipt(
			ctx, postgresAgentPlanMessage("older snapshot"),
		)
		olderDone <- upsertResult{created: created, err: upsertErr}
	}()
	select {
	case <-olderRead:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for older upsert to read")
	}

	newerDone := make(chan upsertResult, 1)
	go func() {
		_, _, created, upsertErr := repoB.UpsertAgentPlanMessageWithConversationReceipt(
			ctx, postgresAgentPlanMessage("newer snapshot"),
		)
		newerDone <- upsertResult{created: created, err: upsertErr}
	}()
	waitForPostgresLockWait(t, ctx, observer, "pg_advisory_xact_lock")
	release()

	for name, done := range map[string]<-chan upsertResult{
		"older": olderDone,
		"newer": newerDone,
	} {
		select {
		case result := <-done:
			if result.err != nil {
				t.Fatalf("%s upsert: %v", name, result.err)
			}
			if result.created {
				t.Fatalf("%s upsert unexpectedly created a row", name)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s upsert", name)
		}
	}

	stored, err := repoA.GetMessage(ctx, initial.ID)
	if err != nil {
		t.Fatalf("read final agent plan: %v", err)
	}
	if stored.Content != "newer snapshot" {
		t.Fatalf("final agent plan = %q, want newer snapshot", stored.Content)
	}
}

func postgresAgentPlanMessage(content string) *models.Message {
	const messageID = "47f568f5-c467-51cc-b63c-71fd2253ed93"
	return &models.Message{
		ID:            messageID,
		TaskSessionID: "session-agent-plan-pg",
		TaskID:        "task-agent-plan-pg",
		TurnID:        "turn-agent-plan-pg",
		AuthorType:    models.MessageAuthorAgent,
		Content:       content,
		Type:          models.MessageTypeAgentPlan,
		Metadata: map[string]interface{}{
			"tool_call_id":            "agent-plan:" + messageID,
			"agent_plan_tool_call_id": "source-agent-plan-pg",
		},
	}
}
