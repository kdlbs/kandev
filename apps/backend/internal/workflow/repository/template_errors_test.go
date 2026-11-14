package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestGetMissingWorkflowTemplateReturnsTypedNotFound(t *testing.T) {
	repo, _ := setupTestRepoWithDB(t)

	_, err := repo.GetTemplate(context.Background(), "missing-template")
	if !errors.Is(err, models.ErrWorkflowTemplateNotFound) {
		t.Fatalf("GetTemplate error = %v, want ErrWorkflowTemplateNotFound", err)
	}
}

func TestGetWorkflowTemplateStorageFailureIsNotNotFound(t *testing.T) {
	repo, db := setupTestRepoWithDB(t)
	if _, err := db.Exec(`DROP TABLE workflow_templates`); err != nil {
		t.Fatalf("drop workflow_templates table: %v", err)
	}

	_, err := repo.GetTemplate(context.Background(), "template-1")
	if err == nil {
		t.Fatal("GetTemplate unexpectedly succeeded after its table was dropped")
	}
	if errors.Is(err, models.ErrWorkflowTemplateNotFound) {
		t.Fatalf("storage error = %v, must not be classified as a missing template", err)
	}
}
