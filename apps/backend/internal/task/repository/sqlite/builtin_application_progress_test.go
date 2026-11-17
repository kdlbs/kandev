package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuiltinApplicationProgressPersistsBarrierBeforeRetirement(t *testing.T) {
	for _, scenario := range []string{"current", "new-save", "stale-version", "foreign-source"} {
		t.Run(scenario, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			_, _, err := settingsstore.Provide(repo.db, repo.db, nil)
			require.NoError(t, err)
			ctx := context.Background()
			seedExecutorRunningCleanupTask(t, repo, "task")
			seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, "execution")
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			now := time.Now().UTC()
			_, err = repo.db.Exec(`UPDATE executors_running SET status='running',resumable=TRUE,resume_token='native' WHERE session_id='session'`)
			require.NoError(t, err)
			_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_preferences (task_id,workspace_id,schema_version,revision,packages_json,updated_by,updated_at) VALUES ('task',?,1,3,'{}','human',?)`, task.WorkspaceID, now)
			require.NoError(t, err)
			_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_applications (session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,created_at,updated_at) VALUES ('session','task',?,3,0,'hash','human','request',?,?)`, task.WorkspaceID, now, now)
			require.NoError(t, err)
			app, err := repo.ClaimBuiltinMCPApplication(ctx, "session", "execution", 1)
			require.NoError(t, err)
			require.NotNil(t, app)
			writer, ok := any(repo).(interface {
				AdvanceBuiltinMCPApplication(context.Context, *models.BuiltinMCPApplication, string, string) (bool, error)
			})
			require.True(t, ok, "durable stage progression must precede destructive provider control")
			source := types.ProviderReplacementSource{}
			require.NoError(t, json.Unmarshal([]byte(providerBindingSourceJSON(t)), &source))
			raw, err := json.Marshal(source)
			require.NoError(t, err)
			app.AttemptSource = string(raw)
			switch scenario {
			case "new-save":
				_, err = repo.db.Exec(`UPDATE task_builtin_mcp_preferences SET revision=4 WHERE task_id='task'`)
				require.NoError(t, err)
			case "stale-version":
				app.RowVersion--
			case "foreign-source":
				source.ExecutionID = "foreign"
				raw, err = json.Marshal(source)
				require.NoError(t, err)
				app.AttemptSource = string(raw)
			}
			advanced, err := writer.AdvanceBuiltinMCPApplication(ctx, app, "retiring", "")
			require.NoError(t, err)
			require.Equal(t, scenario == "current", advanced)
			stored, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			if advanced {
				require.Equal(t, true, stored.Metadata[p.ProviderMCPEffectsClosedMetadataKey])
				row, err := repo.GetBuiltinMCPApplication(ctx, "session")
				require.NoError(t, err)
				require.Equal(t, app.AttemptSource, row.AttemptSource)
				require.Equal(t, "retiring", row.Stage)
			} else {
				require.Nil(t, stored.Metadata[p.ProviderMCPEffectsClosedMetadataKey])
			}
		})
	}
}
