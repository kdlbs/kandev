package journal

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestIncrementalShellJournalUpgradeCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.bbolt")
	store, err := Open(Config{Path: path})
	require.NoError(t, err)
	_, err = store.Append(context.Background(), Event{SessionID: "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Type: "tool_update", Payload: []byte(`{"type":"tool_update"}`)})
	require.NoError(t, err)
	require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
		var value [4]byte
		binary.BigEndian.PutUint32(value[:], 1)
		return tx.Bucket(bucketMeta).Put(keyVersion, value[:])
	}))
	require.NoError(t, store.Close())
	disk, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	injected := errors.New("injected before migration commit")
	err = disk.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		if validateErr := validateExistingJournal(tx, meta, meta.Get(keyVersion)); validateErr != nil {
			return validateErr
		}
		return injected
	})
	require.ErrorIs(t, err, injected)
	require.NoError(t, disk.View(func(tx *bolt.Tx) error {
		require.EqualValues(t, 1, binary.BigEndian.Uint32(tx.Bucket(bucketMeta).Get(keyVersion)))
		return nil
	}))
	require.NoError(t, disk.Close())
	legacyReader, err := Open(Config{Path: path, ReadOnly: true})
	require.NoError(t, err)
	require.NoError(t, legacyReader.Close())
	store, err = Open(Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.db.View(func(tx *bolt.Tx) error {
		require.EqualValues(t, 2, binary.BigEndian.Uint32(tx.Bucket(bucketMeta).Get(keyVersion)))
		return nil
	}))
	events, _, err := store.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.JSONEq(t, `{"type":"tool_update"}`, string(events[0].Payload))
}
