package sqlite

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"testing"
)

type bindingTestGuard struct{ fail bool }

func (g *bindingTestGuard) BindExecutorIn(ctx context.Context, tx *sqlx.Tx, id, kind string, config map[string]string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO binding_test(value) VALUES (1)`); err != nil {
		return err
	}
	if g.fail {
		return errors.New("binding rejected")
	}
	return nil
}

func TestExecutorBindingGuardSharesWriteTransaction(t *testing.T) {
	repo := newRepoForEntityTests(t)
	if _, err := repo.db.Exec(`CREATE TABLE binding_test(value INTEGER)`); err != nil {
		t.Fatal(err)
	}
	guard := &bindingTestGuard{fail: true}
	repo.SetExecutorBindingGuard(guard)
	ctx := context.Background()
	executor := &models.Executor{ID: "bound", Name: "original", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive}
	if err := repo.CreateExecutor(ctx, executor); err == nil {
		t.Fatal("create bypassed guard")
	}
	if _, err := repo.GetExecutor(ctx, executor.ID); err == nil {
		t.Fatal("rejected create persisted")
	}
	guard.fail = false
	if err := repo.CreateExecutor(ctx, executor); err != nil {
		t.Fatal(err)
	}
	guard.fail = true
	executor.Name = "changed"
	if err := repo.UpdateExecutor(ctx, executor); err == nil {
		t.Fatal("update bypassed guard")
	}
	if err := repo.DeleteExecutor(ctx, executor.ID); err == nil {
		t.Fatal("delete bypassed guard")
	}
	got, err := repo.GetExecutor(ctx, executor.ID)
	if err != nil || got.Name != "original" {
		t.Fatalf("rejected mutation persisted: %v %v", got, err)
	}
	var count int
	if err = repo.db.Get(&count, `SELECT COUNT(*) FROM binding_test`); err != nil || count != 1 {
		t.Fatalf("guard writes escaped rollback: %d %v", count, err)
	}
}
