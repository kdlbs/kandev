package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/stretchr/testify/require"
)

func TestSandboxOwnershipScopeDefaultsToKernelContainer(t *testing.T) {
	require.Equal(t, OwnershipKernelContainer, AttemptSnapshot{}.Scope())
	require.Equal(t, OwnershipProcessTree, AttemptSnapshot{OwnershipScope: OwnershipProcessTree}.Scope())
}

func TestSandboxAttemptOwnershipScopeStoredValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		valid bool
	}{
		{"absent", nil, true},
		{"kernel", OwnershipKernelContainer, true},
		{"process tree", OwnershipProcessTree, true},
		{"unknown", "elsewhere", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := sqlitePolicyDB(t)
			service, attempt, _ := approvalStoreFixture(t, database)
			raw, err := json.Marshal(attempt.Snapshot)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(raw, &fields))
			delete(fields, "ownership_scope")
			if tc.value != nil {
				fields["ownership_scope"] = tc.value
			}
			raw, err = json.Marshal(fields)
			require.NoError(t, err)
			_, err = db.ExecContext(t.Context(), database, `UPDATE sandbox_attempts SET snapshot_json=? WHERE attempt_id=?`, string(raw), attempt.ID)
			require.NoError(t, err)
			read, err := service.ReadAttempt(t.Context(), "org", attempt.ID)
			if tc.valid {
				require.NoError(t, err)
				if tc.value == nil {
					require.Equal(t, OwnershipKernelContainer, read.Snapshot.Scope())
				}
				return
			}
			require.Error(t, err)
		})
	}
}

func processTreeApprovalFixture(t *testing.T) (*Service, *Attempt, string, *sqlx.DB) {
	t.Helper()
	database := sqlitePolicyDB(t)
	service, attempt, file := approvalStoreFixture(t, database)
	raw, err := json.Marshal(attempt.Snapshot)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	fields["ownership_scope"] = OwnershipProcessTree
	raw, err = json.Marshal(fields)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), database, `UPDATE sandbox_attempts SET snapshot_json=? WHERE attempt_id=?`, string(raw), attempt.ID)
	require.NoError(t, err)
	read, err := service.ReadAttempt(t.Context(), "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, OwnershipProcessTree, read.Snapshot.Scope())
	return service, read, file, database
}

func TestSandboxRecordApprovalRefusesProcessTreeAttempt(t *testing.T) {
	service, attempt, file, database := processTreeApprovalFixture(t)
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	_, err := service.RecordApproval(operatorContext(t), "org", attempt.ID, ApprovalOperation{RequestID: "request", ProviderSessionID: attempt.Owner.ProviderSessionID, ChildPID: attempt.Owner.ChildPID, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.ErrorIs(t, err, ErrIneligible)
	var rows int
	require.NoError(t, database.GetContext(t.Context(), &rows, `SELECT COUNT(*) FROM sandbox_approval_requests`))
	require.Zero(t, rows)
}

func TestSandboxRecordApprovalStillRecordsForKernelContainer(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	_, err := service.RecordApproval(operatorContext(t), "org", attempt.ID, ApprovalOperation{RequestID: "request", ProviderSessionID: attempt.Owner.ProviderSessionID, ChildPID: attempt.Owner.ChildPID, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
}
