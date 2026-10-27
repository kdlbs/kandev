package sandbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxAttemptCaptureRechecksAuthorityAtInsert(t *testing.T) {
	for _, kind := range []string{"unchanged", "revoked", "executor-changed", "agent-changed", "active-execution"} {
		t.Run(kind, func(t *testing.T) {
			service, original, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
			ctx := operatorContext(t)
			service.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) { return TargetScope{OrgID: "org"}, nil })
			selection := SelectionContext{OrgID: "org", ExecutorID: original.Snapshot.Resolved.ExecutorID, AgentProfileID: "agent-profile"}
			resolved, err := service.Resolve(ctx, selection)
			require.NoError(t, err)
			request := CaptureRequest{Selection: selection, Resolved: resolved, SessionID: original.Snapshot.SessionID, ExecutionID: original.Snapshot.ExecutionID, WorkDir: original.Snapshot.WorkDir, Launch: original.Snapshot.Launch.Clone(), Provider: original.Snapshot.Provider}
			request.Launch.AttemptID = "next-attempt"
			request.Launch.Generation++
			if kind != "active-execution" {
				_, err = service.TransitionAttempt(ctx, "org", original.ID, original.Version, AttemptStopped, nil)
				require.NoError(t, err)
			}
			snapshot, err := service.captureSnapshot(ctx, request)
			require.NoError(t, err)
			switch kind {
			case "revoked":
				require.NoError(t, service.Revoke(ctx, resolved.Revision.ID, resolved.Revision.Digest))
			case "executor-changed":
				binding, readErr := service.GetBinding(ctx, ScopeExecutor, selection.ExecutorID)
				require.NoError(t, readErr)
				_, err = service.PutBinding(ctx, *binding, binding.Version)
				require.NoError(t, err)
			case "agent-changed":
				_, err = service.PutBinding(ctx, Binding{Scope: ScopeAgent, TargetID: selection.AgentProfileID, RevisionID: resolved.Revision.ID}, 0)
				require.NoError(t, err)
			}
			err = service.insertCapturedAttempt(ctx, request, snapshot)
			if kind == "unchanged" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrConflict)
			_, err = service.ReadAttempt(ctx, "org", request.Launch.AttemptID)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}
