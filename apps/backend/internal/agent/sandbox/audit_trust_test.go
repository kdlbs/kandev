package sandbox

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAuditTrustStoreSQLite(t *testing.T) { runAuditTrustStore(t, sqlitePolicyDB(t)) }
func TestAuditTrustStorePostgres(t *testing.T) {
	runAuditTrustStore(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func runAuditTrustStore(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database, 2)
	ctx := operatorContext(t)
	initial, err := service.GetAuditTrust(ctx, "session", attempt.ID)
	require.NoError(t, err)
	require.Zero(t, initial.Version)
	require.True(t, initial.Revoked, "a session cannot trust its own embedded signer")
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(public)
	require.NoError(t, err)
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	for _, invalid := range []string{string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})), publicPEM + "private trailing material", "", "unparsed public material", "unexpected prefix\n" + publicPEM} {
		_, err = service.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{PublicKeyPEM: invalid})
		require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	}
	for _, caller := range []context.Context{
		authn.WithIdentity(t.Context(), authn.Identity{UserID: "token", OrgID: "org", Role: authn.RoleAdmin, TokenID: "pat"}),
		authn.WithIdentity(t.Context(), authn.Identity{UserID: "viewer", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"}),
	} {
		_, err = service.SetAuditTrust(caller, "session", attempt.ID, AuditTrustRequest{PublicKeyPEM: publicPEM})
		require.ErrorIs(t, err, ErrAuthority)
	}
	first, err := service.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{PublicKeyPEM: publicPEM})
	require.NoError(t, err)
	digest := sha256.Sum256(der)
	require.Equal(t, hex.EncodeToString(digest[:]), first.Fingerprint, "the generic host must accept a public-key algorithm without a Nono-specific restriction")
	require.Equal(t, "operator", first.Source)
	require.NotEmpty(t, first.OperatorID)
	require.False(t, first.Revoked)
	require.EqualValues(t, 1, first.Version)
	_, err = service.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{PublicKeyPEM: publicPEM})
	require.ErrorIs(t, err, ErrConflict)
	revoked, err := service.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{ExpectedVersion: first.Version, Revoke: true})
	require.NoError(t, err)
	require.True(t, revoked.Revoked)
	require.Empty(t, revoked.PublicKeyPEM)
	require.NotEqual(t, first.ID, revoked.ID)
	restarted, err := NewService(database, true, nil)
	require.NoError(t, err)
	restarted.SetSessionAuthorizer(func(context.Context, string) error { return nil })
	read, err := restarted.GetAuditTrust(ctx, "session", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, revoked, read)
	_, err = restarted.GetAuditTrust(ctx, "another-session", attempt.ID)
	require.Error(t, err)
	var count int
	require.NoError(t, database.Get(&count, `SELECT count(*) FROM sandbox_audit_trust_revisions`))
	require.Equal(t, 2, count, "key revocation must retain immutable historical trust")
	service.SetSessionAuthorizer(func(context.Context, string) error { return ErrNotFound })
	_, err = service.GetAuditTrust(ctx, "session", attempt.ID)
	require.ErrorIs(t, err, ErrNotFound)
}
