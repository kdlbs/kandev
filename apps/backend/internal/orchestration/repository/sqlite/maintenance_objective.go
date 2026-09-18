package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// The source records an explicit human grant form submission. It is not a
// synthetic chat intake and does not enqueue a model turn or create a worker.
func createMaintenanceObjective(ctx context.Context, tx *sqlx.Tx, b *models.AssistantBinding, grant *models.MaintenanceGrant, candidate *models.ImprovementCandidate) error {
	comment, objective := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	body := fmt.Sprintf("Maintenance grant confirmed for proposal %s in repository %s. Prepare only the approved local files and checks; publication and changes to running services require separate authority.", candidate.ID, grant.Scope.RepositoryID)
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES(?,?,'user',?,?,'maintenance_grant',?)`), comment, b.ConversationID, b.OwnerUserID, body, now)
	if err != nil {
		return err
	}
	row := models.Objective{ID: objective, BindingID: b.ID, WorkspaceID: b.WorkspaceID, SourceCommentID: comment,
		Title: "Prepare a scoped local workflow repair", Mode: "execute", Status: "active", Revision: 1, AcceptanceRevision: 1, CreatedAt: now, UpdatedAt: now,
		Acceptance: []models.Criterion{{ID: "files", Description: "Changes remain within the explicit maintenance file scope."}, {ID: "checks", Description: "The approved positive and negative checks pass for the prepared tree."}, {ID: "local-commit", Description: "A local commit and review receipt exist; no changes are published or deployed."}}, Evidence: []models.Evidence{}}
	if err = tx.GetContext(ctx, &row.IntentRevision, tx.Rebind(`SELECT COALESCE((SELECT revision FROM orchestration_conversation_intents WHERE task_id=?),0)`), b.ConversationID); err != nil {
		return err
	}
	if err = objectiveJSON(&row); err != nil {
		return err
	}
	_, err = tx.NamedExecContext(ctx, `INSERT INTO orchestration_objectives
 (id,binding_id,workspace_id,source_comment_id,title,mode,status,revision,acceptance_revision,intent_revision,acceptance_json,evidence_json,created_at,updated_at)
 VALUES(:id,:binding_id,:workspace_id,:source_comment_id,:title,:mode,:status,:revision,:acceptance_revision,:intent_revision,:acceptance_json,:evidence_json,:created_at,:updated_at)`, row)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_improvements SET objective_id=?,revision=revision+1,updated_at=? WHERE id=? AND binding_id=?`), objective, now, candidate.ID, b.ID)
	return err
}
