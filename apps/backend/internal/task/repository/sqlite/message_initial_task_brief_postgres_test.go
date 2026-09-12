package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/admission"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5
func TestInitialTaskBriefAdmissionPostgres(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-pg-initial-brief"
		sessionID = "session-pg-initial-brief"
		turnID    = "turn-pg-initial-brief"
		brief     = "Postgres initial task brief"
	)
	seedPostgresSession(t, repo, taskID, sessionID, turnID, time.Now().UTC())
	setInitialTaskBriefDescription(t, repo, taskID, brief)
	writer := requireInitialTaskBriefMessageWriter(t, repo)

	messages := []*models.Message{
		initialTaskBriefMessage(taskID, sessionID, turnID, "pg-initial-brief-a", "instruction a"),
		initialTaskBriefMessage(taskID, sessionID, turnID, "pg-initial-brief-b", "instruction b"),
	}
	candidates := []*admission.InitialTaskBriefCandidate{
		initialTaskBriefCandidate(brief, "Postgres initial task brief\n\ninstruction a"),
		initialTaskBriefCandidate(brief, "Postgres initial task brief\n\ninstruction b"),
	}
	errs := make([]error, len(messages))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range messages {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			errs[index] = writer.CreateMessageWithInitialTaskBrief(ctx, messages[index], candidates[index])
		}(index)
	}
	close(start)
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("postgres initial admission %d: %v", index, err)
		}
	}

	winners := 0
	ordinals := map[int]bool{}
	for index, candidate := range candidates {
		ordinals[messages[index].PromptIndex] = true
		if candidate.Selected {
			winners++
			if messages[index].Content != candidate.Content || messages[index].PromptIndex != 1 {
				t.Fatalf("postgres winner message=%+v candidate=%+v", messages[index], candidate)
			}
			continue
		}
		if messages[index].Content == candidate.Content || messages[index].PromptIndex != 2 {
			t.Fatalf("postgres loser message=%+v candidate=%+v", messages[index], candidate)
		}
	}
	if winners != 1 || len(ordinals) != 2 || !ordinals[1] || !ordinals[2] {
		t.Fatalf("postgres winners=%d ordinals=%v, want one winner and {1, 2}", winners, ordinals)
	}
}
