package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// TestRecordManualOverrideIfAdmitted_WritesAuditRecordAndWarning pins
// AC-14/AC-53: an admitted manual override, once it reaches a real session,
// stamps a session metadata audit record and writes the AC-14 card warning.
func TestRecordManualOverrideIfAdmitted_WritesAuditRecordAndWarning(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "mo-task", "mo-session", models.TaskSessionStateStarting)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.recordManualOverrideIfAdmitted(ctx, "mo-task", "mo-session", true, 3, true, 2)

	session, err := repo.GetTaskSession(ctx, "mo-session")
	require.NoError(t, err)
	record, ok := session.Metadata[ceilingManualOverrideMetadataKey].(map[string]interface{})
	require.True(t, ok, "audit record was not written: %+v", session.Metadata)
	require.Equal(t, []interface{}{ceilingReasonManualOverride}, toInterfaceSlice(record["reason_codes"]))
	require.EqualValues(t, 2, record[ceilingFieldCeiling])
	require.EqualValues(t, 3, record["population"])
	require.NotEmpty(t, record["recorded_at"])

	require.Len(t, messages.sessionMessages, 1)
	note := messages.sessionMessages[0]
	require.Equal(t, "mo-session", note.sessionID)
	require.Equal(t, metaVariantCeiling, note.metadata[metaKeyVariant])
	require.Equal(t, ceilingReasonManualOverride, note.metadata[ceilingFieldReasonCode])
}

// TestRecordManualOverrideIfAdmitted_PopulationUnknownOmitsPopulationKey pins
// AC-33a: when the population was unknown at admission, the audit record
// omits the population key entirely rather than recording a sentinel, and
// carries the unknown-population reason code alongside the override code.
func TestRecordManualOverrideIfAdmitted_PopulationUnknownOmitsPopulationKey(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "mo-unk-task", "mo-unk-session", models.TaskSessionStateStarting)
	svc.messageCreator = &mockMessageCreator{}

	svc.recordManualOverrideIfAdmitted(ctx, "mo-unk-task", "mo-unk-session", true, 0, false, 2)

	session, err := repo.GetTaskSession(ctx, "mo-unk-session")
	require.NoError(t, err)
	record := session.Metadata[ceilingManualOverrideMetadataKey].(map[string]interface{})
	_, hasPopulation := record["population"]
	require.False(t, hasPopulation, "population must be omitted, not recorded as a sentinel: %+v", record)
	require.Equal(t,
		[]interface{}{ceilingReasonManualOverride, ceilingReasonUnknownPopulation},
		toInterfaceSlice(record["reason_codes"]))
}

// TestRecordManualOverrideIfAdmitted_IdempotentSecondCallDoesNotRewriteOrRewarn
// pins AC-53a: the audit record is written once per session, and a second
// admitted override for the same session neither overwrites the record nor
// sends a second warning.
func TestRecordManualOverrideIfAdmitted_IdempotentSecondCallDoesNotRewriteOrRewarn(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "mo-dup-task", "mo-dup-session", models.TaskSessionStateStarting)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.recordManualOverrideIfAdmitted(ctx, "mo-dup-task", "mo-dup-session", true, 3, true, 2)
	svc.recordManualOverrideIfAdmitted(ctx, "mo-dup-task", "mo-dup-session", true, 9, true, 2)

	session, err := repo.GetTaskSession(ctx, "mo-dup-session")
	require.NoError(t, err)
	record := session.Metadata[ceilingManualOverrideMetadataKey].(map[string]interface{})
	require.EqualValues(t, 3, record["population"], "the first admission's reading must be retained")
	require.Len(t, messages.sessionMessages, 1, "a repeat admission must not warn twice")
}

// TestRecordManualOverrideIfAdmitted_NotOverrideOrNoSessionIsNoOp pins that
// this is a no-op for an ordinary admission (manualOverride=false) and for
// AC-14b's window before a session exists (sessionID empty).
func TestRecordManualOverrideIfAdmitted_NotOverrideOrNoSessionIsNoOp(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "mo-noop-task", "mo-noop-session", models.TaskSessionStateStarting)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.recordManualOverrideIfAdmitted(ctx, "mo-noop-task", "mo-noop-session", false, 3, true, 2)
	svc.recordManualOverrideIfAdmitted(ctx, "mo-noop-task", "", true, 3, true, 2)

	session, err := repo.GetTaskSession(ctx, "mo-noop-session")
	require.NoError(t, err)
	require.Nil(t, session.Metadata[ceilingManualOverrideMetadataKey])
	require.Empty(t, messages.sessionMessages)
}

func toInterfaceSlice(v interface{}) []interface{} {
	switch typed := v.(type) {
	case []interface{}:
		return typed
	case []string:
		out := make([]interface{}, len(typed))
		for i, s := range typed {
			out[i] = s
		}
		return out
	default:
		return nil
	}
}
