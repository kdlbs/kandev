package sqlite

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTransferTaskPreservesPendingClarificationIdentity(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	ctx := context.Background()
	message := &models.Message{
		ID: "pending-clarification", TaskID: task.ID, TaskSessionID: "session-running", TurnID: "turn-active",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{"pending_id": "pending-request", "status": "pending", "request_id": "request-stable"},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.TransferTask(ctx, taskTransferCommand(task)); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetMessage(ctx, message.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("pending clarification changed: before=%+v after=%+v error=%v", before, after, err)
	}
}
