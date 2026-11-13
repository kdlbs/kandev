package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCanonicalEnvironmentRepoRowEligibleRejectsDetachedRoots(t *testing.T) {
	row := &models.TaskEnvironmentRepo{Status: models.TaskEnvironmentRepoStatusDetached, WorktreeID: "wt"}
	if canonicalEnvironmentRepoRowEligible(row, true) {
		t.Fatal("detached root must not satisfy canonical launch inventory")
	}
	row.Status = models.TaskEnvironmentRepoStatusActive
	if !canonicalEnvironmentRepoRowEligible(row, true) {
		t.Fatal("active root should satisfy canonical launch inventory")
	}
}
