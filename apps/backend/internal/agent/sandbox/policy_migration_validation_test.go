package sandbox

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPolicyMigrationRefusesChangedIdentityOrMeaning(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	source, err := service.Publish(operatorContext(t), policyFixture(t), 0)
	require.NoError(t, err)
	for name, change := range map[string]func(*PolicyMigrationRequest){
		"different policy":    func(r *PolicyMigrationRequest) { r.Policy.ID = "other" },
		"different workspace": func(r *PolicyMigrationRequest) { r.Policy.WorkspaceID = "other" },
		"different provider":  func(r *PolicyMigrationRequest) { r.Policy.ProviderKey = "other" },
		"different install":   func(r *PolicyMigrationRequest) { r.Policy.InstallationID = "other" },
		"legacy payload":      func(r *PolicyMigrationRequest) { r.Policy.PolicyV2 = nil },
		"unknown version":     func(r *PolicyMigrationRequest) { r.Policy.PolicyV2.SchemaVersion = 3 },
		"mixed grants":        func(r *PolicyMigrationRequest) { r.Policy.Grants.KandevMCP = true },
		"invalid envelope":    func(r *PolicyMigrationRequest) { r.Policy.PolicyV2.ApprovalEnvelope.KandevMCP = true },
	} {
		t.Run(name, func(t *testing.T) {
			request := migrationRequestFixture(source)
			change(&request)
			_, previewErr := service.PreviewPolicyMigration(operatorContext(t), source.ID, request)
			require.ErrorIs(t, previewErr, contract.ErrInvalidPolicy)
		})
	}
}

func TestPolicyMigrationRefusesStaleOrRevokedSource(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		service, err := NewService(sqlitePolicyDB(t), true, nil)
		require.NoError(t, err)
		source, publishErr := service.Publish(operatorContext(t), policyFixture(t), 0)
		require.NoError(t, publishErr)
		request := migrationRequestFixture(source)
		preview, previewErr := service.PreviewPolicyMigration(operatorContext(t), source.ID, request)
		require.NoError(t, previewErr)
		if revoked {
			require.NoError(t, service.Revoke(operatorContext(t), source.ID, source.Digest))
		} else {
			_, err = service.Publish(operatorContext(t), source.Policy, source.Number)
			require.NoError(t, err)
		}
		_, err = service.PublishPolicyMigration(operatorContext(t), source.ID, request, preview.Digest)
		if revoked {
			require.ErrorIs(t, err, ErrIneligible)
		} else {
			require.ErrorIs(t, err, ErrConflict)
		}
	}
}

func TestPolicyMigrationRejectsAgentAuthority(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	ctx := operatorContext(t)
	source, err := service.Publish(ctx, policyFixture(t), 0)
	require.NoError(t, err)
	request := migrationRequestFixture(source)
	preview, err := service.PreviewPolicyMigration(ctx, source.ID, request)
	require.NoError(t, err)
	for _, caller := range []struct {
		name string
		ctx  context.Context
	}{
		{"agent", scope.WithPrincipal(ctx, scope.Principal{CallerTaskID: "task", CallerSessionID: "session"})},
		{"external MCP", origin.WithTrustedExternalTransport(ctx)},
		{"internal dispatch", origin.WithTrustedInternalCall(ctx)},
	} {
		t.Run(caller.name, func(t *testing.T) {
			_, err := service.PreviewPolicyMigration(caller.ctx, source.ID, request)
			require.ErrorIs(t, err, ErrAuthority)
			_, err = service.PublishPolicyMigration(caller.ctx, source.ID, request, preview.Digest)
			require.ErrorIs(t, err, ErrAuthority)
		})
	}
}

func TestPolicyMigrationConcurrentSQLite(t *testing.T) {
	testPolicyMigrationConcurrent(t, sqlitePolicyDB(t))
}

func TestPolicyMigrationConcurrentPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(4)
	testPolicyMigrationConcurrent(t, database)
}

func testPolicyMigrationConcurrent(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	ctx := operatorContext(t)
	source, err := service.Publish(ctx, policyFixture(t), 0)
	require.NoError(t, err)
	request := migrationRequestFixture(source)
	preview, err := service.PreviewPolicyMigration(ctx, source.ID, request)
	require.NoError(t, err)
	results, start := make(chan error, 2), make(chan struct{})
	for range 2 {
		go func() {
			<-start
			_, publishErr := service.PublishPolicyMigration(ctx, source.ID, request, preview.Digest)
			results <- publishErr
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first == nil {
		require.ErrorIs(t, second, ErrConflict)
	} else {
		require.ErrorIs(t, first, ErrConflict)
		require.NoError(t, second)
	}
	latest, err := service.GetLatest(ctx, source.Policy.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), latest.Number)
	require.Equal(t, source.ID, latest.Migration.SourceRevisionID)
}
