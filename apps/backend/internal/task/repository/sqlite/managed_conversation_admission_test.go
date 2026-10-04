package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.6, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationAdmissionPostgresRollback(t *testing.T) {
	for _, mode := range []string{"create", "update", "repair"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := newManagedAdmissionPostgresRepoPair(t)
			ctx := context.Background()
			require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "rollback-ws", Name: "Rollback"}))
			svc := taskservice.NewAgentConversationService(a, a, nil, nil, nil)
			spec := pluginsdk.ManagedAgentConversationSpec{WorkspaceID: "rollback-ws", InstanceKey: "lead", AgentProfileID: "original-profile", BasePrompt: "original", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
			var before *models.TaskSession
			var taskID string
			if mode != "create" {
				first, _, err := svc.EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
				require.NoError(t, err)
				taskID = first.TaskID
				spec.ExpectedRevision = first.Revision
				before, err = b.GetTaskSession(ctx, first.SessionID)
				require.NoError(t, err)
				if mode == "repair" {
					require.NoError(t, b.DeleteTaskSession(ctx, before))
				}
			}
			_, err := a.DB().ExecContext(ctx, `CREATE FUNCTION fail_managed_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'admission rollback'; END $$`)
			require.NoError(t, err)
			trigger := `CREATE TRIGGER fail_managed_admission BEFORE UPDATE OF metadata ON tasks FOR EACH ROW EXECUTE FUNCTION fail_managed_admission()`
			if mode == "create" {
				trigger = `CREATE TRIGGER fail_managed_admission BEFORE INSERT ON task_sessions FOR EACH ROW EXECUTE FUNCTION fail_managed_admission()`
			}
			_, err = a.DB().ExecContext(ctx, trigger)
			require.NoError(t, err)
			spec.AgentProfileID, spec.BasePrompt = "replacement-profile", "replacement"
			_, _, err = svc.EnsureManaged(ctx, "plugin", "installation", spec, "update", "update-digest")
			require.ErrorContains(t, err, "admission rollback")
			if mode == "create" {
				var tasks, sessions int
				require.NoError(t, b.DB().QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&tasks))
				require.Zero(t, tasks)
				require.NoError(t, b.DB().QueryRowContext(ctx, `SELECT count(*) FROM task_sessions`).Scan(&sessions))
				require.Zero(t, sessions)
			} else {
				stored, err := b.GetTask(ctx, taskID)
				require.NoError(t, err)
				require.Equal(t, "[]", stored.Labels)
				require.Equal(t, "original", stored.Metadata["kandev.base_prompt"])
				require.Equal(t, "1", stored.Metadata[models.MetaKeyManagedConversationRevision])
				if mode == "repair" {
					_, err = b.GetTaskSession(ctx, before.ID)
					require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
				} else {
					current, err := b.GetTaskSession(ctx, before.ID)
					require.NoError(t, err)
					require.Equal(t, before, current)
				}
			}
		})
	}
}
