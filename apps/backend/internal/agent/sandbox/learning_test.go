package sandbox

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func learningFixture(t *testing.T, database *sqlx.DB) (*Service, *Attempt, LearningRequest) {
	t.Helper()
	base := policyFixture(t)
	ceiling := base
	ceiling.ID = "ceiling"
	base.Grants.Filesystem = []contract.FilesystemGrant{{Path: base.Grants.Filesystem[0].Path}}
	service, attempt, file := approvalPolicyFixture(t, database, base, ceiling)
	data := reportFixture()
	data.Records[0].Target = file
	data.Records[0].Operation = "write"
	report, err := service.CaptureReport(t.Context(), "org", attempt.ID, attempt.Version, 0, data)
	require.NoError(t, err)
	service.SetLearningCompiler(func(_ context.Context, current *Attempt, records []DenialRecord) (contract.Grants, error) {
		require.Equal(t, attempt.Snapshot, current.Snapshot)
		require.Equal(t, data.Records, records)
		return contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: file, Writable: true}}}, nil
	})
	return service, attempt, LearningRequest{ReportDigest: report.Digest, RecordIDs: []string{"denial"}, Name: "Learned policy", Scope: "installation"}
}

func TestSandboxLearningPublicationSQLite(t *testing.T) {
	testLearningPublication(t, sqlitePolicyDB(t))
}
func TestSandboxLearningPublicationPostgres(t *testing.T) {
	testLearningPublication(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

// AC-AGENTS-SANDBOX-004.2-5: a reviewed diff creates a revision, not a selection or live grant.
func testLearningPublication(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, request := learningFixture(t, database)
	ctx := operatorContext(t)
	before, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	proposal, err := service.ProposeLearning(ctx, "session", attempt.ID, request)
	require.NoError(t, err)
	require.Equal(t, request.ReportDigest, proposal.ReportDigest)
	require.Equal(t, attempt.Snapshot.Resolved.Revision.ID, proposal.BaseRevisionID)
	require.Equal(t, attempt.Snapshot.Resolved.Ceiling.Digest, proposal.CeilingDigest)
	require.Len(t, proposal.AddedGrants.Filesystem, 1)
	require.NotNil(t, proposal.AddedGrants.Network)
	require.True(t, proposal.AddedGrants.Filesystem[0].Writable)
	require.Equal(t, "operator", proposal.OperatorID)
	require.Empty(t, proposal.PublishedRevisionID)
	_, err = service.GetLatest(ctx, proposal.Policy.ID)
	require.ErrorIs(t, err, ErrNotFound, "proposal grants no authority")
	_, err = service.PublishLearning(ctx, proposal.ID, "stale")
	require.ErrorIs(t, err, ErrConflict)
	revision := publishLearningConcurrently(t, ctx, service, proposal)
	require.Equal(t, proposal.Policy, revision.Policy)
	require.Equal(t, "operator", revision.OperatorID)
	require.NotNil(t, revision.Learning)
	require.NotEmpty(t, revision.Learning.Sources)
	require.Equal(t, "partial", revision.Learning.Integrity)
	require.False(t, revision.Learning.Complete)
	require.Equal(t, proposal.ReportDigest, revision.Learning.ReportDigest)
	require.Equal(t, proposal.SelectedRecords, revision.Learning.Records)
	require.Equal(t, proposal.Provider, revision.Learning.Provider)
	after, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	require.Equal(t, before, after)
	current, err := service.ReadAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, attempt.Snapshot, current.Snapshot)
	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	reloaded.SetSessionAuthorizer(func(context.Context, string) error { return nil })
	stored, err := reloaded.ReadLearning(ctx, proposal.ID)
	require.NoError(t, err)
	require.Equal(t, revision.ID, stored.PublishedRevisionID)
	require.Equal(t, proposal.Digest, stored.Digest)
	require.Equal(t, proposal.SelectedRecords, stored.SelectedRecords)
	_, err = service.PublishLearning(ctx, proposal.ID, proposal.Digest)
	require.ErrorIs(t, err, ErrConflict, "one proposal creates one revision")
	_, err = database.ExecContext(ctx, database.Rebind(`DELETE FROM sandbox_attempts WHERE org_id=? AND attempt_id=?`), "org", attempt.ID)
	require.NoError(t, err)
	saved, err := reloaded.GetRevision(ctx, revision.ID)
	require.NoError(t, err)
	require.Equal(t, revision.Learning, saved.Learning, "revision provenance survives task history deletion")
}

func TestSandboxLearningRejectsUnreviewedOrBroaderChanges(t *testing.T) {
	for _, kind := range []string{"member", "PAT", "foreign session", "unknown record", "stale report", "outside ceiling", "changed report", "revoked source"} {
		t.Run(kind, func(t *testing.T) {
			service, attempt, request := learningFixture(t, sqlitePolicyDB(t))
			ctx := operatorContext(t)
			sessionID := "session"
			switch kind {
			case "member":
				ctx = authn.WithIdentity(t.Context(), authn.Identity{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"})
			case "PAT":
				ctx = authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, TokenID: "token"})
			case "foreign session":
				sessionID = "foreign"
			case "unknown record":
				request.RecordIDs = []string{"foreign"}
			case "stale report":
				request.ReportDigest = "stale"
			case "outside ceiling":
				service.SetLearningCompiler(func(context.Context, *Attempt, []DenialRecord) (contract.Grants, error) {
					return contract.Grants{KandevMCP: true}, nil
				})
			}
			proposal, err := service.ProposeLearning(ctx, sessionID, attempt.ID, request)
			if kind != "changed report" && kind != "revoked source" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if kind == "changed report" {
				report, readErr := service.ReadCapturedReport(ctx, "org", attempt.ID)
				require.NoError(t, readErr)
				_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, report.Version, report.Data)
			} else {
				err = service.Revoke(ctx, proposal.BaseRevisionID, proposal.BaseDigest)
			}
			require.NoError(t, err)
			_, err = service.PublishLearning(ctx, proposal.ID, proposal.Digest)
			require.Error(t, err)
			_, err = service.GetLatest(ctx, proposal.Policy.ID)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func publishLearningConcurrently(t *testing.T, ctx context.Context, service *Service, proposal *LearningProposal) *Revision {
	t.Helper()
	type outcome struct {
		revision *Revision
		err      error
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			revision, err := service.PublishLearning(ctx, proposal.ID, proposal.Digest)
			outcomes <- outcome{revision, err}
		}()
	}
	close(start)
	var winner *Revision
	for range 2 {
		result := <-outcomes
		if result.err != nil {
			require.ErrorIs(t, result.err, ErrConflict)
		} else {
			require.Nil(t, winner)
			winner = result.revision
		}
	}
	require.NotNil(t, winner)
	return winner
}

func TestSandboxLearningDiffOmitsExistingAccess(t *testing.T) {
	service, attempt, request := learningFixture(t, sqlitePolicyDB(t))
	base := attempt.Snapshot.Resolved.Revision.Policy.Grants
	service.SetLearningCompiler(func(_ context.Context, _ *Attempt, records []DenialRecord) (contract.Grants, error) {
		return contract.Grants{Filesystem: append(base.Filesystem, contract.FilesystemGrant{Path: records[0].Target, Writable: true})}, nil
	})
	proposal, err := service.ProposeLearning(operatorContext(t), "session", attempt.ID, request)
	require.NoError(t, err)
	require.Len(t, proposal.AddedGrants.Filesystem, 1)
	require.NotNil(t, proposal.AddedGrants.Network)
	require.Equal(t, proposal.SelectedRecords[0].Target, proposal.AddedGrants.Filesystem[0].Path)
}
