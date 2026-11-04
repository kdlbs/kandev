package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestLegacyPolicyMeaningAndDigest(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	policy := Policy{ID: "policy", Name: "Legacy", InstallationID: "installation", ProviderKey: "fixture-alpha",
		Grants: contract.Grants{
			Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}},
			Network:    []contract.NetworkGrant{{Host: "api.example.test", Port: 443, Protocol: "https"}},
		}}
	revision, err := service.Publish(operatorContext(t), policy, 0)
	require.NoError(t, err)
	require.Equal(t, 1, revision.SchemaVersion)
	require.Equal(t, "2aaf075fb474af2929f82858fd59a7157fe5e61a92e77fb46be41947e045dd46", revision.Digest)
	encoded, err := json.Marshal(revision.Policy)
	require.NoError(t, err)
	require.Equal(t, `{"id":"policy","workspace_id":"","name":"Legacy","installation_id":"installation","provider_key":"fixture-alpha","grants":{"filesystem":[{"reference":"work_dir","writable":true}],"network":[{"host":"api.example.test","port":443,"protocol":"https"}],"kandev_mcp":false},"sources":null}`, string(encoded))
}

func TestTypedPolicyPersistenceSQLite(t *testing.T) {
	testTypedPolicyPersistence(t, sqlitePolicyDB(t))
}

func TestTypedPolicyPersistencePostgres(t *testing.T) {
	testTypedPolicyPersistence(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testTypedPolicyPersistence(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	policy := typedPolicyFixture()
	revision, err := service.Publish(operatorContext(t), policy, 0)
	require.NoError(t, err)
	require.Equal(t, contract.PolicySchemaV2, revision.SchemaVersion)
	require.Equal(t, "api.example.test", revision.Policy.PolicyV2.StaticGrants.TCP[0].Host)
	require.Equal(t, "API.EXAMPLE.TEST", policy.PolicyV2.StaticGrants.TCP[0].Host)
	require.Empty(t, revision.Policy.PolicyV2.ApprovalEnvelope.TCP)
	require.Empty(t, revision.Policy.PolicyV2.ApprovalEnvelope.Filesystem)
	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	stored, err := reloaded.GetRevision(operatorContext(t), revision.ID)
	require.NoError(t, err)
	require.Equal(t, revision, stored)
}

func typedPolicyFixture() Policy {
	return Policy{ID: "typed-policy", Name: "Typed", InstallationID: "installation", ProviderKey: "fixture-alpha",
		PolicyV2: &contract.PolicyV2{SchemaVersion: contract.PolicySchemaV2, StaticGrants: contract.PermissionsV2{
			Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}},
			TCP:        []contract.TCPDestination{{Host: "API.EXAMPLE.TEST", Port: 443, Transport: "tcp"}},
		}}}
}

func TestTypedPolicyPublishRejectsAmbiguousAndUnknownSchemas(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	for _, version := range []int{0, 1, 3} {
		policy := typedPolicyFixture()
		policy.PolicyV2.SchemaVersion = version
		_, err := service.Publish(operatorContext(t), policy, 0)
		require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	}
	policy := typedPolicyFixture()
	policy.Grants.KandevMCP = true
	_, err = service.Publish(operatorContext(t), policy, 0)
	require.ErrorIs(t, err, contract.ErrInvalidPolicy)
}
