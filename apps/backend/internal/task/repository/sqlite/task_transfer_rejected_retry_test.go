package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestTransferTaskRefusesSameCallerRejectedKey(t *testing.T) {
	for _, result := range []string{taskTransferResultDenied, taskTransferResultFailed} {
		t.Run(result, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			command := taskTransferCommand(task)
			rejected := command
			rejected.Actor.Kind = models.TaskTransferActorRejected
			if err := repo.RecordTaskTransferAttempt(context.Background(), rejected, result); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.TransferTask(context.Background(), command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
				t.Fatalf("reused rejected key: %v, want conflict", err)
			}
			stored, err := repo.GetTask(context.Background(), task.ID)
			if err != nil || stored.WorkspaceID != task.WorkspaceID {
				t.Fatalf("rejected retry mutated task: %+v, %v", stored, err)
			}
			command.IdempotencyKey += "-fresh"
			if _, err := repo.TransferTask(context.Background(), command); err != nil {
				t.Fatalf("fresh key: %v", err)
			}
		})
	}
}

func TestTransferTaskRejectedKeyCannotPoisonAnotherCaller(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	command := taskTransferCommand(task)
	rejected := command
	rejected.Actor = models.TaskTransferActor{Kind: models.TaskTransferActorRejected, ID: "other-human"}
	if err := repo.RecordTaskTransferAttempt(context.Background(), rejected, taskTransferResultDenied); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.TransferTask(context.Background(), command); err != nil {
		t.Fatalf("another caller's denied attempt blocked owner: %v", err)
	}
}

func TestTransferTaskRefusesRejectedConfigurationSessionKey(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	command := taskTransferCommand(task)
	command.Actor.SessionID = "configuration-session"
	rejected := command
	rejected.Actor = models.TaskTransferActor{Kind: models.TaskTransferActorRejected, ID: "configuration-task", SessionID: command.Actor.SessionID}
	if err := repo.RecordTaskTransferAttempt(context.Background(), rejected, taskTransferResultFailed); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.TransferTask(context.Background(), command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("same session reused failed key: %v", err)
	}
}
