package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxReviewedRestartSQLite(t *testing.T) { testReviewedRestart(t, sqlitePolicyDB(t)) }
func TestSandboxReviewedRestartPostgres(t *testing.T) {
	testReviewedRestart(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func reviewedRestartFixture(t *testing.T, database *sqlx.DB) (*Service, CaptureRequest) {
	t.Helper()
	service, previous, _ := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	_, err := service.TransitionAttempt(ctx, "org", previous.ID, previous.Version, AttemptStopped, nil)
	require.NoError(t, err)
	policy := previous.Snapshot.Resolved.Revision.Policy
	policy.Name = "Reviewed current policy"
	latest, err := service.Publish(ctx, policy, previous.Snapshot.Resolved.Revision.Number)
	require.NoError(t, err)
	binding, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	binding.Executor.DefaultRevisionID = latest.ID
	binding.Executor.EligibleRevisionIDs = append(binding.Executor.EligibleRevisionIDs, latest.ID)
	_, err = service.PutBinding(ctx, *binding, binding.Version)
	require.NoError(t, err)
	selection := SelectionContext{OrgID: "org", ExecutorID: "local"}
	resolved, err := service.Resolve(ctx, selection)
	require.NoError(t, err)
	raw, err := json.Marshal(resolved)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	// Decode the wire request so the baseline fails on behavior, not a missing type.
	wire, err := json.Marshal(map[string]any{"RestartReview": map[string]string{"attempt_id": previous.ID, "selection_digest": hex.EncodeToString(sum[:])}})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(wire, &selection))
	request := CaptureRequest{RestartOf: previous.ID, Selection: selection, Resolved: resolved, SessionID: previous.Snapshot.SessionID, ExecutionID: previous.Snapshot.ExecutionID, WorkDir: previous.Snapshot.WorkDir, Launch: previous.Snapshot.Launch.Clone(), Provider: previous.Snapshot.Provider}
	request.Launch.AttemptID = "reviewed-restart"
	request.Launch.Generation++
	request.Launch.PolicyDigest = latest.Digest
	return service, request
}

func testReviewedRestart(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, request := reviewedRestartFixture(t, database)
	attempt, err := service.CaptureAttempt(operatorContext(t), request)
	require.NoError(t, err, "explicit operator review must admit the current approved revision")
	require.Equal(t, request.Resolved.Revision.ID, attempt.Snapshot.Resolved.Revision.ID)
	raw, err := json.Marshal(attempt.Snapshot)
	require.NoError(t, err)
	var recorded struct {
		Review *struct {
			OperatorID      string `json:"operator_id"`
			SelectionDigest string `json:"selection_digest"`
		} `json:"restart_review"`
	}
	require.NoError(t, json.Unmarshal(raw, &recorded))
	require.NotNil(t, recorded.Review, "the operator decision must be captured before dispatch")
	require.Equal(t, "operator", recorded.Review.OperatorID)
	require.NotEmpty(t, recorded.Review.SelectionDigest)
}

func TestSandboxReviewedRestartRefusesUntrustedCaller(t *testing.T) {
	for name, caller := range map[string]func(context.Context) context.Context{
		"absent": func(context.Context) context.Context { return t.Context() },
		"agent": func(ctx context.Context) context.Context {
			return scope.WithPrincipal(ctx, scope.Principal{CallerTaskID: "task", CallerSessionID: "session"})
		},
		"foreign": func(ctx context.Context) context.Context {
			return authn.WithIdentity(ctx, authn.Identity{UserID: "foreign", OrgID: "foreign", Role: authn.RoleAdmin, SessionID: "browser"})
		},
		"PAT": func(ctx context.Context) context.Context {
			return authn.WithIdentity(ctx, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, TokenID: "token"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			service, request := reviewedRestartFixture(t, sqlitePolicyDB(t))
			_, err := service.CaptureAttempt(caller(operatorContext(t)), request)
			require.ErrorIs(t, err, ErrAuthority)
			_, err = service.ReadAttempt(operatorContext(t), "org", request.Launch.AttemptID)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestSandboxReviewedRestartRechecksBindingsAtInsert(t *testing.T) {
	service, request := reviewedRestartFixture(t, sqlitePolicyDB(t))
	ctx := operatorContext(t)
	snapshot, err := service.captureSnapshot(ctx, request)
	require.NoError(t, err)
	binding, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	_, err = service.PutBinding(ctx, *binding, binding.Version)
	require.NoError(t, err)
	require.ErrorIs(t, service.insertCapturedAttempt(ctx, request, snapshot), ErrConflict)
	_, err = service.CaptureAttempt(ctx, request)
	require.ErrorIs(t, err, ErrConflict, "an old review cannot authorize a changed selection")
}

func TestSandboxReviewedTaskAdmissionDoesNotSaveUnreviewedGuard(t *testing.T) {
	service, request := reviewedRestartFixture(t, sqlitePolicyDB(t))
	ctx := operatorContext(t)
	selection := request.Selection
	selection.TaskID = "task"
	_, err := service.AdmitTask(ctx, selection, "")
	require.ErrorIs(t, err, ErrConflict, "changed context must be reviewed before guard persistence")
	guard, _, err := service.readTaskGuard(ctx, "org", "task", "")
	require.NoError(t, err)
	require.Nil(t, guard)
	resolved, err := service.Resolve(ctx, selection)
	require.NoError(t, err)
	digest, err := resolvedReviewDigest(resolved)
	require.NoError(t, err)
	selection.RestartReview.SelectionDigest = digest
	admitted, err := service.AdmitTask(ctx, selection, "")
	require.NoError(t, err)
	require.Equal(t, resolved.Revision.ID, admitted.Revision.ID)
	guard, _, err = service.readTaskGuard(ctx, "org", "task", "")
	require.NoError(t, err)
	require.Equal(t, resolved.Revision.ID, guard.Revision.ID)
}
