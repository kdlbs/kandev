package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypedAttemptReadRejectsInvalidEnvelope(t *testing.T) {
	service, request, _ := typedPolicyCaptureFixture(t, sqlitePolicyDB(t))
	attempt, err := service.CaptureAttempt(operatorContext(t), request)
	require.NoError(t, err)
	require.True(t, attempt.Snapshot.Ceiling.KandevMCP, "a missing ceiling grant must not hide the invalid envelope")
	attempt.Snapshot.PolicyV2.ApprovalEnvelope.KandevMCP = true
	raw, err := json.Marshal(attempt.Snapshot)
	require.NoError(t, err)
	_, err = service.db.Exec(`UPDATE sandbox_attempts SET snapshot_json=? WHERE org_id=? AND attempt_id=?`, string(raw), "org", attempt.ID)
	require.NoError(t, err)
	_, err = service.ReadAttempt(operatorContext(t), "org", attempt.ID)
	require.ErrorIs(t, err, ErrConflict, "stored envelopes must obey the policy model, not just fit inside a ceiling")
}

func TestTypedPolicyRestartRefusesUnknownCurrentRevision(t *testing.T) {
	service, request, _ := typedPolicyCaptureFixture(t, sqlitePolicyDB(t))
	ctx := operatorContext(t)
	attempt, err := service.CaptureAttempt(ctx, request)
	require.NoError(t, err)
	attempt, err = service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	changed := *request.Resolved.Revision
	changed.SchemaVersion = 99
	raw, err := json.Marshal(changed)
	require.NoError(t, err)
	_, err = service.db.Exec(`UPDATE sandbox_policy_revisions SET content_json=? WHERE id=?`, string(raw), changed.ID)
	require.NoError(t, err)
	_, err = service.ResolveRestart(ctx, request.Selection, request.SessionID, request.WorkDir, attempt.ID)
	require.ErrorIs(t, err, ErrIneligible, "a captured digest cannot make an unknown current schema eligible for restart")
}
