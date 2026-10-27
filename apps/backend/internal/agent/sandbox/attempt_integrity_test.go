package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/stretchr/testify/require"
)

func TestSandboxAttemptRejectsCorruptSnapshot(t *testing.T) {
	for _, kind := range []string{"schema", "launch", "org", "digest", "generation", "execution"} {
		t.Run(kind, func(t *testing.T) {
			database := sqlitePolicyDB(t)
			service, attempt, _ := approvalStoreFixture(t, database)
			raw, err := json.Marshal(attempt.Snapshot)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(raw, &fields))
			switch kind {
			case "schema":
				fields["schema_version"] = 99
			case "launch":
				fields["launch"] = nil
			case "org":
				fields["org_id"] = "foreign"
			case "digest":
				fields["launch"].(map[string]any)["policy_digest"] = strings.Repeat("c", 64)
			case "generation":
				fields["launch"].(map[string]any)["generation"] = 2
			case "execution":
				fields["execution_id"] = "foreign"
			}
			raw, err = json.Marshal(fields)
			require.NoError(t, err)
			_, err = db.ExecContext(t.Context(), database, `UPDATE sandbox_attempts SET snapshot_json=? WHERE attempt_id=?`, string(raw), attempt.ID)
			require.NoError(t, err)
			_, err = service.ReadAttempt(t.Context(), "org", attempt.ID)
			require.Error(t, err)
		})
	}
}

func TestSandboxApprovalRevocationPreventsNewGrant(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	ctx := operatorContext(t)
	approval, err := service.RecordApproval(ctx, "org", attempt.ID, ApprovalOperation{RequestID: "request", ProviderSessionID: attempt.Owner.ProviderSessionID, ChildPID: attempt.Owner.ChildPID, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	require.NoError(t, service.Revoke(ctx, attempt.Snapshot.Resolved.Revision.ID, attempt.Snapshot.Resolved.Revision.Digest))
	_, err = service.DecideApproval(ctx, attempt.ID, "request", approval.Version, "allow_once")
	require.Error(t, err)
	_, err = service.DecideApproval(ctx, attempt.ID, "request", approval.Version, "deny")
	require.NoError(t, err)
}

func TestSandboxApprovalRevocationAtDecisionBoundary(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	ctx := operatorContext(t)
	approval, err := service.RecordApproval(ctx, "org", attempt.ID, ApprovalOperation{RequestID: "request", ProviderSessionID: attempt.Owner.ProviderSessionID, ChildPID: attempt.Owner.ChildPID, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error {
		return service.Revoke(ctx, attempt.Snapshot.Resolved.Revision.ID, attempt.Snapshot.Resolved.Revision.Digest)
	})
	_, err = service.DecideApproval(ctx, attempt.ID, "request", approval.Version, "allow_once")
	require.Error(t, err)
}
