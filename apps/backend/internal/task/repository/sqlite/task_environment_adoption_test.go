package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/stretchr/testify/require"
)

func seedWorkspaceAdoptionDB(t *testing.T, r *Repository) models.WorkspaceAdoptionPreview {
	t.Helper()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "task")
	statements := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('adopt-ws','Adopt',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO tasks(id,workspace_id,title,state,created_at,updated_at) VALUES('adopt-task','adopt-ws','Adopt','IN_PROGRESS',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO repositories(id,workspace_id,name,source_type,local_path,created_at,updated_at) VALUES('adopt-repo','adopt-ws','Adopt','local',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{filepath.Join(root, "source")}},
		{`INSERT INTO task_repositories(id,task_id,repository_id,checkout_branch,position,created_at,updated_at) VALUES('adopt-attachment','adopt-task','adopt-repo','feature/adopt',2,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO task_environments(id,task_id,executor_type,status,ownership_generation,task_dir_name,workspace_path,created_at,updated_at) VALUES('adopt-env','adopt-task','worktree','ready',1,'task',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{root}},
		{`INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,branch_slug,position,status,created_at,updated_at) VALUES('adopt-slot','adopt-env','adopt-repo','adopt-wt',?,'feature/adopt','feature-adopt',2,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{filepath.Join(root, "old")}},
		{`INSERT INTO task_sessions(id,queue_incarnation_id,task_id,agent_profile_id,state,task_environment_id,workspace_path,started_at,updated_at) VALUES('adopt-session','adopt-incarnation','adopt-task','profile','WAITING_FOR_INPUT','adopt-env',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{root}},
	}
	for _, st := range statements {
		_, err := r.db.Exec(r.db.Rebind(st.q), st.args...)
		require.NoError(t, err)
	}
	session, err := r.GetTaskSession(ctx, "adopt-session")
	require.NoError(t, err)
	env, err := r.GetTaskEnvironment(ctx, "adopt-env")
	require.NoError(t, err)
	repo, err := r.GetRepository(ctx, "adopt-repo")
	require.NoError(t, err)
	selection := models.WorkspaceAdoptionSelection{Snapshot: models.NewWorkspaceRecoverySelectionSnapshot(session, env, map[string]*models.Repository{repo.ID: repo}), WorkspaceID: "adopt-ws", SessionIncarnation: "adopt-incarnation", SessionWorkspacePath: root, AttachmentID: "adopt-attachment", AttachmentCheckoutBranch: "feature/adopt", AttachmentsDigest: models.WorkspaceAdoptionDigest([]models.WorkspaceAdoptionAttachment{{ID: "adopt-attachment", TaskID: "adopt-task", RepositoryID: "adopt-repo", CheckoutBranch: "feature/adopt", MetadataDigest: models.WorkspaceAdoptionDigest(map[string]interface{}{}), Position: 2}})}
	p := models.WorkspaceAdoptionPreview{Selection: selection, SlotID: "adopt-slot", Proof: models.WorkspaceAdoptionProof{OriginalPath: filepath.Join(root, "old"), CandidatePath: filepath.Join(root, "clone"), Branch: "feature/adopt"}}
	p.Token = p.Digest()
	return p
}

func acquireWorkspaceAdoptionClaim(t *testing.T, r *Repository, p models.WorkspaceAdoptionPreview) *models.TaskEnvironmentRecoveryClaim {
	t.Helper()
	s := p.Selection.Snapshot
	claim, err := r.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{TaskEnvironmentID: s.TaskEnvironmentID, OwnerTaskID: s.TaskID, OwnershipGeneration: s.OwnershipGeneration, SessionID: s.SessionID, OperationID: "11111111-1111-4111-8111-111111111111", ExecutorType: "worktree"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), claim) })
	return claim
}

func assertAdoptionDBUnpublished(t *testing.T, r *Repository, p models.WorkspaceAdoptionPreview) {
	t.Helper()
	var path string
	require.NoError(t, r.db.Get(&path, `SELECT worktree_path FROM task_environment_repos WHERE id='adopt-slot'`))
	require.Equal(t, p.Proof.OriginalPath, path)
	var count int
	require.NoError(t, r.db.Get(&count, `SELECT COUNT(*) FROM task_environment_checkout_adoptions`))
	require.Zero(t, count)
}

func exerciseWorkspaceAdoptionPublication(t *testing.T, r *Repository) {
	p := seedWorkspaceAdoptionDB(t, r)
	claim := acquireWorkspaceAdoptionClaim(t, r, p)
	injected := errors.New("pinned path changed before commit")
	_, err := r.CommitWorkspaceAdoption(context.Background(), p, claim.OperationID, claim, func() error { return injected })
	require.ErrorIs(t, err, injected)
	assertAdoptionDBUnpublished(t, r, p)
	receipt, err := r.CommitWorkspaceAdoption(context.Background(), p, claim.OperationID, claim, func() error { return nil })
	require.NoError(t, err)
	require.NoError(t, r.ValidateWorkspaceAdoption(context.Background(), receipt))
	reopened := NewWithInitializedDB(r.db, r.ro, nil)
	stored, err := reopened.GetWorkspaceAdoption(context.Background(), claim.OperationID)
	require.NoError(t, err)
	require.Equal(t, p.Token, stored.Preview.Token)
	require.NoError(t, reopened.ensureWorkspaceAdoptionsSchema())
	require.NoError(t, reopened.ValidateWorkspaceAdoption(context.Background(), stored))
	wrong := *stored
	wrong.OperationID = "22222222-2222-4222-8222-222222222222"
	require.ErrorIs(t, reopened.FinalizeWorkspaceAdoption(context.Background(), &wrong), models.ErrWorkspaceAdoptionStale)
	require.NoError(t, reopened.FinalizeWorkspaceAdoption(context.Background(), stored))
	var claims int
	require.NoError(t, r.db.Get(&claims, `SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id='adopt-env'`))
	require.Zero(t, claims)
	require.NoError(t, reopened.FinalizeWorkspaceAdoption(context.Background(), stored))

	_, err = r.db.Exec(`UPDATE task_environments SET ownership_generation=2 WHERE id='adopt-env'`)
	require.NoError(t, err)
	require.ErrorIs(t, r.ValidateWorkspaceAdoption(context.Background(), stored), models.ErrWorkspaceAdoptionStale)
}

func TestWorkspaceAdoptionPublicationSQLite(t *testing.T) {
	exerciseWorkspaceAdoptionPublication(t, newRepoForEntityTests(t))
}

func TestWorkspaceAdoptionPublicationPostgres(t *testing.T) {
	r, _, _ := newTaskPostgresRepoPair(t)
	exerciseWorkspaceAdoptionPublication(t, r)
}

func exerciseWorkspaceAdoptionSelectionCAS(t *testing.T, newRepository func(*testing.T) *Repository) {
	for _, mutation := range []string{
		`UPDATE task_sessions SET queue_incarnation_id='new' WHERE id='adopt-session'`,
		`UPDATE task_repositories SET position=3 WHERE id='adopt-attachment'`,
		`UPDATE task_repositories SET base_branch='changed' WHERE id='adopt-attachment'`,
		`UPDATE task_repositories SET metadata='{"pr_number":17}' WHERE id='adopt-attachment'`,
		`UPDATE task_environment_repos SET worktree_branch='foreign' WHERE id='adopt-slot'`,
		`UPDATE task_environments SET workspace_path='foreign' WHERE id='adopt-env'`,
	} {
		t.Run(mutation, func(t *testing.T) {
			r := newRepository(t)
			p := seedWorkspaceAdoptionDB(t, r)
			claim := acquireWorkspaceAdoptionClaim(t, r, p)
			_, err := r.db.Exec(mutation)
			require.NoError(t, err)
			_, err = r.CommitWorkspaceAdoption(context.Background(), p, claim.OperationID, claim, func() error { return nil })
			require.ErrorIs(t, err, models.ErrWorkspaceAdoptionStale)
			assertAdoptionDBUnpublished(t, r, p)
		})
	}
}

func TestWorkspaceAdoptionExclusiveClaimPostgres(t *testing.T) {
	a, b, _ := newTaskPostgresRepoPair(t)
	p := seedWorkspaceAdoptionDB(t, a)
	claim := acquireWorkspaceAdoptionClaim(t, a, p)
	request := models.TaskEnvironmentRecoveryClaimRequest{TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID, OwnershipGeneration: claim.OwnershipGeneration, SessionID: claim.SessionID, OperationID: "22222222-2222-4222-8222-222222222222", ExecutorType: "worktree"}
	_, err := b.AcquireTaskEnvironmentRecoveryClaim(context.Background(), request)
	require.ErrorIs(t, err, recoveryclaim.ErrBusy)
}

func TestWorkspaceAdoptionSelectionCASSQLite(t *testing.T) {
	exerciseWorkspaceAdoptionSelectionCAS(t, newRepoForEntityTests)
}

func TestWorkspaceAdoptionSelectionCASPostgres(t *testing.T) {
	exerciseWorkspaceAdoptionSelectionCAS(t, func(t *testing.T) *Repository { r, _, _ := newTaskPostgresRepoPair(t); return r })
}

func exerciseWorkspaceAdoptionBorrowedConsumer(t *testing.T, r *Repository) {
	p := seedWorkspaceAdoptionDB(t, r)
	claim := acquireWorkspaceAdoptionClaim(t, r, p)
	var executorID string
	require.NoError(t, r.db.Get(&executorID, `SELECT id FROM executors ORDER BY id LIMIT 1`))
	_, err := r.db.Exec(r.db.Rebind(`INSERT INTO executors_running(id,session_id,task_id,executor_id,status,created_at,updated_at) VALUES('adopt-runtime','adopt-session','adopt-task',?,'running',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), executorID)
	require.NoError(t, err)
	_, err = r.CommitWorkspaceAdoption(context.Background(), p, claim.OperationID, claim, func() error { return nil })
	require.ErrorIs(t, err, recoveryclaim.ErrBusy)
	assertAdoptionDBUnpublished(t, r, p)
}

func TestWorkspaceAdoptionBorrowedConsumerSQLite(t *testing.T) {
	exerciseWorkspaceAdoptionBorrowedConsumer(t, newRepoForEntityTests(t))
}
func TestWorkspaceAdoptionBorrowedConsumerPostgres(t *testing.T) {
	r, _, _ := newTaskPostgresRepoPair(t)
	exerciseWorkspaceAdoptionBorrowedConsumer(t, r)
}
