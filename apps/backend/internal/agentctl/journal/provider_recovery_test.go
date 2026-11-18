package journal

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestMCPProviderReplacementRetirementRequiresNewJournalReader(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	storage, err := Open(Config{Path: path})
	require.NoError(t, err)
	defer func() { _ = storage.Close() }()
	receipt, err := storage.CreateProviderReplacement(ctx, providerReceiptFixture("operation"))
	require.NoError(t, err)
	require.Equal(t, uint32(2), providerJournalDiskVersion(t, storage), "saving a reversible reservation preserves the original format")
	receipt.Phase = ProviderReplacementRetiring
	_, err = storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	require.NoError(t, err)
	require.Equal(t, uint32(3), providerJournalDiskVersion(t, storage), "a reader that ignores provider scopes must not reopen a retired generation")
	require.NoError(t, storage.Close())
	storage, err = Open(Config{Path: path})
	require.NoError(t, err)
	require.Equal(t, uint32(3), providerJournalDiskVersion(t, storage), "reopen must not downgrade the required reader")
	require.NoError(t, storage.Close())
	storage, err = Open(Config{Path: path, ReadOnly: true, ExistingOnly: true})
	require.NoError(t, err)
	require.Equal(t, uint32(3), providerJournalDiskVersion(t, storage))
}

func providerJournalDiskVersion(t *testing.T, storage *Journal) uint32 {
	t.Helper()
	var version uint32
	storage.mu.RLock()
	defer storage.mu.RUnlock()
	require.NoError(t, storage.viewLocked(func(tx *bolt.Tx) error {
		version = binary.BigEndian.Uint32(tx.Bucket(bucketMeta).Get(keyVersion))
		return nil
	}))
	return version
}
