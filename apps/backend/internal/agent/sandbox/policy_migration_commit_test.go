package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPolicyMigrationCommitAuthoritySQLite(t *testing.T) {
	testPolicyMigrationCommitAuthority(t, sqlitePolicyDB(t))
}

func TestPolicyMigrationCommitAuthorityPostgres(t *testing.T) {
	testPolicyMigrationCommitAuthority(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testPolicyMigrationCommitAuthority(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	for _, revoked := range []bool{false, true} {
		policy := policyFixture(t)
		if revoked {
			policy.ID = "revoked-source"
		}
		source, publishErr := service.Publish(operatorContext(t), policy, 0)
		require.NoError(t, publishErr)
		preview, previewErr := service.PreviewPolicyMigration(operatorContext(t), source.ID, migrationRequestFixture(source))
		require.NoError(t, previewErr)
		candidate, raw := draftPolicyMigration(t, source, preview)
		if revoked {
			require.NoError(t, service.Revoke(operatorContext(t), source.ID, source.Digest))
		}
		err = service.insertRevision(operatorContext(t), candidate, raw, source.Number)
		latest, readErr := service.GetLatest(operatorContext(t), source.Policy.ID)
		require.NoError(t, readErr)
		if revoked {
			require.ErrorIs(t, err, ErrConflict, "source revocation after review must prevent the commit")
			require.Equal(t, source.ID, latest.ID, "failed migration must roll back the head update")
		} else {
			require.NoError(t, err)
			require.Equal(t, candidate.ID, latest.ID)
		}
	}
}

func draftPolicyMigration(t *testing.T, source *Revision, preview *PolicyMigrationPreview) (*Revision, []byte) {
	t.Helper()
	candidate := *source
	candidate.ID, candidate.Number, candidate.SchemaVersion, candidate.Policy = "migrated-"+source.ID, 2, 2, preview.After
	candidate.Migration = &PolicyMigrationProvenance{SourceRevisionID: source.ID, SourceDigest: source.Digest, ReviewDigest: preview.Digest}
	content, err := json.Marshal(candidate.Policy)
	require.NoError(t, err)
	digest := sha256.Sum256(content)
	candidate.Digest = hex.EncodeToString(digest[:])
	raw, err := json.Marshal(candidate)
	require.NoError(t, err)
	return &candidate, raw
}

func TestPolicyMigrationPostgresHoldsSourceUntilCommit(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(4)
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	source, err := service.Publish(operatorContext(t), policyFixture(t), 0)
	require.NoError(t, err)
	otherPolicy := policyFixture(t)
	otherPolicy.ID = "other-policy"
	other, err := service.Publish(operatorContext(t), otherPolicy, 0)
	require.NoError(t, err)
	preview, err := service.PreviewPolicyMigration(operatorContext(t), source.ID, migrationRequestFixture(source))
	require.NoError(t, err)
	candidate, raw := draftPolicyMigration(t, source, preview)
	transaction, err := database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = transaction.Rollback() })
	require.NoError(t, insertRevisionTx(t.Context(), transaction, candidate, raw, source.Number))
	probe, err := database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = probe.Rollback() })
	var id string
	err = probe.QueryRowContext(t.Context(), probe.Rebind(`SELECT id FROM sandbox_policy_revisions WHERE id=? FOR UPDATE NOWAIT`), other.ID).Scan(&id)
	require.NoError(t, err, "an unrelated revision remains available")
	err = probe.QueryRowContext(t.Context(), probe.Rebind(`SELECT id FROM sandbox_policy_revisions WHERE id=? FOR UPDATE NOWAIT`), source.ID).Scan(&id)
	var lockError *pgconn.PgError
	require.ErrorAs(t, err, &lockError, "revocation must wait until the migration transaction settles")
	require.Equal(t, "55P03", lockError.Code)
}
