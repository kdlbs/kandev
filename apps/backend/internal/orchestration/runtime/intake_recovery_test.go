package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantBindingRuntimeCannotReadAnotherPrivateConversation(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	a, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	a.ID = "foreign-chief"
	require.NoError(t, s.Personas.CreateAgentInstance(ctx, a))
	require.NoError(t, s.Repo.RegisterOrchestrator(ctx, a.ID, "ws", "chief-of-staff"))
	conversation, err := s.Repo.EnsureAgentConversation(ctx, a)
	require.NoError(t, err)
	require.NoError(t, s.Repo.SelectAssistant(ctx, &models.AssistantBinding{OwnerUserID: "foreign", OrchestratorID: a.ID, WorkspaceID: "ws", ConversationID: conversation.TaskID}, 0))
	s.Tasks.(*testTasks).tasks[conversation.TaskID] = &taskmodels.Task{ID: conversation.TaskID, WorkspaceID: "ws"}
	router, token, runID := assistantRuntimeCaller(t, s, task)
	path := "/api/v1/orchestration/tasks/" + conversation.TaskID
	for _, suffix := range []string{"", "/comments"} {
		require.Equal(t, 404, runtimeRequest(t, router, "GET", path+suffix, token, runID, nil).Code)
	}
	require.Equal(t, 404, runtimeRequest(t, router, "POST", path+"/comments", token, runID, map[string]string{"body": "Unauthorized note"}).Code)
}

func TestAssistantIntakeAtomicRollback(t *testing.T) {
	s, db, task := newRuntime(t)
	_, err := db.Exec(`CREATE TRIGGER fail_intake BEFORE INSERT ON orchestration_intake BEGIN SELECT RAISE(ABORT,'outbox unavailable'); END`)
	require.NoError(t, err)
	result := runtimeRequest(t, assistantRouter(s), "POST", "/api/v1/orchestration/tasks/"+task+"/comments", "", "", map[string]string{"body": "Inspect", "client_message_id": "rollback"})
	require.Equal(t, 400, result.Code)
	rows, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Empty(t, rows)
	revision, err := s.Repo.IntentRevision(context.Background(), task)
	require.NoError(t, err)
	require.Zero(t, revision)
}

func TestAssistantIntakeConcurrentSendAndMigrationReplay(t *testing.T) {
	s, db, task := newRuntime(t)
	router := assistantRouter(s)
	start, results := make(chan struct{}), make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/tasks/"+task+"/comments", "", "", map[string]string{"body": "Inspect", "client_message_id": "concurrent"})
			results <- result.Code
		}()
	}
	close(start)
	require.ElementsMatch(t, []int{200, 201}, []int{<-results, <-results})
	require.NoError(t, s.Repo.Migrate())
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM runs"))
	require.Equal(t, 1, count)
	rows, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 1, rows[0].IntentRevision)
}

func TestAssistantIntakeHistoryCursorHasNoGaps(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	created := time.Now().UTC()
	for i := 0; i < 5; i++ {
		require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: fmt.Sprintf("comment-%d", i), TaskID: task, Body: "message", Source: "user", AuthorType: "user", AuthorID: "owner", CreatedAt: created}))
	}
	router := assistantRouter(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments?limit=2"
	var page struct {
		Comments   []models.TaskComment `json:"comments"`
		NextCursor string               `json:"next_cursor"`
	}
	var ids []string
	for i := 0; i < 3; i++ {
		result := runtimeRequest(t, router, "GET", path+"&before="+page.NextCursor, "", "", nil)
		require.Equal(t, 200, result.Code, result.Body.String())
		require.NoError(t, json.Unmarshal(result.Body.Bytes(), &page))
		for _, c := range page.Comments {
			ids = append(ids, c.ID)
		}
		if i < 2 {
			require.NotEmpty(t, page.NextCursor)
		}
	}
	require.Empty(t, page.NextCursor)
	require.ElementsMatch(t, []string{"comment-0", "comment-1", "comment-2", "comment-3", "comment-4"}, ids)
	require.Equal(t, 400, runtimeRequest(t, router, "GET", path+"&before=foreign", "", "", nil).Code)
}
