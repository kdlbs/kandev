package sandbox

import (
	"testing"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPolicyMigrationSQLite(t *testing.T) {
	testPolicyMigration(t, sqlitePolicyDB(t))
}

func TestPolicyMigrationPostgres(t *testing.T) {
	testPolicyMigration(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testPolicyMigration(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	source := attempt.Snapshot.Resolved.Revision
	request := migrationRequestFixture(source)
	binding, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	preview, err := service.PreviewPolicyMigration(ctx, source.ID, request)
	require.NoError(t, err)
	require.Equal(t, source.Policy, preview.Before)
	require.Equal(t, request.Policy.ID, preview.After.ID)
	require.Equal(t, request.Policy.PolicyV2.StaticGrants.Filesystem, preview.After.PolicyV2.StaticGrants.Filesystem)
	require.Empty(t, preview.After.PolicyV2.ApprovalEnvelope.Filesystem, "legacy ceiling headroom must not become an envelope")
	require.True(t, validApprovalDigest(preview.Digest))
	latest, err := service.GetLatest(ctx, source.Policy.ID)
	require.NoError(t, err)
	require.Equal(t, source, latest, "preview grants no authority and writes no revision")
	_, err = service.PublishPolicyMigration(ctx, source.ID, request, "stale")
	require.ErrorIs(t, err, ErrConflict)
	changed := migrationRequestFixture(source)
	changed.Policy.PolicyV2.ApprovalEnvelope.Filesystem = source.Policy.Grants.Filesystem
	_, err = service.PublishPolicyMigration(ctx, source.ID, changed, preview.Digest)
	require.ErrorIs(t, err, ErrConflict, "the reviewed digest binds static access and the explicit envelope")
	revision, err := service.PublishPolicyMigration(ctx, source.ID, request, preview.Digest)
	require.NoError(t, err)
	require.Equal(t, 2, revision.SchemaVersion)
	require.Equal(t, source.Number+1, revision.Number)
	require.Equal(t, "operator", revision.OperatorID)
	require.Equal(t, &PolicyMigrationProvenance{SourceRevisionID: source.ID, SourceDigest: source.Digest, ReviewDigest: preview.Digest}, revision.Migration)
	before, err := service.GetRevision(ctx, source.ID)
	require.NoError(t, err)
	require.Equal(t, source, before)
	current, err := service.ReadAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, attempt, current, "publication must not alter a running attempt")
	selected, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	require.Equal(t, binding, selected, "migration requires a later explicit selection")
	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	stored, err := reloaded.GetRevision(ctx, revision.ID)
	require.NoError(t, err)
	require.Equal(t, revision, stored)
	_, err = service.PublishPolicyMigration(ctx, source.ID, request, preview.Digest)
	require.ErrorIs(t, err, ErrConflict, "a reviewed source head can be migrated only once")
}

func migrationRequestFixture(source *Revision) PolicyMigrationRequest {
	target := source.Policy
	target.Grants = contract.Grants{}
	target.Sources = nil
	target.PolicyV2 = &contract.PolicyV2{SchemaVersion: contract.PolicySchemaV2, StaticGrants: contract.PermissionsV2{
		Filesystem: source.Policy.Grants.Filesystem, EnvironmentKeys: source.Policy.Grants.EnvironmentKeys, KandevMCP: source.Policy.Grants.KandevMCP,
	}}
	return PolicyMigrationRequest{SourceDigest: source.Digest, ExpectedRevision: source.Number, Policy: target}
}
