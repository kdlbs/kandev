package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A host that stopped after the token checkpoint and before the instance was
// recorded resumes from the saved token: it never handshakes again, creates or
// adopts the instance, and publishes ready only after the instance is verified.
func TestRecoveryWithATokenButNoInstanceCreatesTheInstanceAndPublishes(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	fixture.store.record.TransientAuthToken = durableToken
	fixture.store.record.Metadata[MetadataKeyAuthTokenSecret] = "secret-token"
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	defer instances[0].Client.Close()

	require.Equal(t, -1, fixture.log.index("handshake:"+durableNonce), "a recorded token must not be replaced by a new handshake")
	require.GreaterOrEqual(t, fixture.log.index("token_client:"+durableToken), 0)
	require.GreaterOrEqual(t, fixture.log.index("create_instance"), 0, "the missing instance was not created")
	require.Len(t, fixture.store.published, 1)
	persisted, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	require.Equal(t, "ready", persisted.Phase)
	require.EqualValues(t, testPluginExecutorInstancePort, persisted.InstancePort)
}

// A secret store that cannot be read must defer recovery. Only a recorded
// reference that is absent classifies the bootstrap as consumed or incomplete.
func TestUnreadableRecordedCredentialDefersRecoveryWithoutDestroying(t *testing.T) {
	cases := map[string]func(*durableRecoveryFixture){
		"token reference recorded, value unreadable": func(f *durableRecoveryFixture) {
			f.store.record.Metadata[MetadataKeyAuthTokenSecret] = "secret-token"
			f.store.record.TransientAuthToken = ""
			f.store.record.TransientBootstrapNonce = ""
		},
		"nonce reference recorded, value unreadable": func(f *durableRecoveryFixture) {
			f.store.record.Metadata[MetadataKeyBootstrapNonceSecret] = "secret-nonce"
			f.store.record.TransientBootstrapNonce = ""
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDurableRecoveryFixture(t, "provisioned")
			arrange(fixture)
			instances, _ := fixture.recover()
			require.Empty(t, instances)
			require.Nil(t, fixture.operations.destroyRequest, "an unreadable credential destroyed a live environment")
			require.False(t, fixture.store.deleted)
			require.Empty(t, fixture.store.published)
		})
	}
}
