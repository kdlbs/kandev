package state

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

type testStateKeyPager interface {
	ListKeys(context.Context, string, string, string, *string, int) ([]string, bool, error)
}

func TestStoreListKeysSQLite(t *testing.T) {
	checkStoreKeyPages(t, newTestStore(t))
}

func TestStoreListKeysPostgres(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	var version string
	require.NoError(t, conn.QueryRow(`SHOW server_version`).Scan(&version))
	t.Logf("PostgreSQL %s", version)
	store, err := NewStore(db.NewPool(conn, conn))
	require.NoError(t, err)
	checkStoreKeyPages(t, store)
}

func checkStoreKeyPages(t *testing.T, store *Store) {
	t.Helper()
	pager, ok := any(store).(testStateKeyPager)
	require.True(t, ok, "state store must support bounded key-only pages")
	ctx := context.Background()
	keys, more, err := pager.ListKeys(ctx, "p", "legacy", "s", nil, 200)
	require.NoError(t, err)
	require.Empty(t, keys)
	require.False(t, more)
	expected := []string{""}
	for i := range 452 {
		key := fmt.Sprintf("key-%03d", i)
		expected = append(expected, key)
		require.NoError(t, store.Set(ctx, "p", "legacy", "s", key, json.RawMessage(`{"v":true}`)))
	}
	require.NoError(t, store.Set(ctx, "p", "legacy", "s", "", json.RawMessage(`{"v":true}`)))
	for _, identity := range [][3]string{{"other", "legacy", "s"}, {"p", "other", "s"}, {"p", "legacy", "other"}} {
		require.NoError(t, store.Set(ctx, identity[0], identity[1], identity[2], "foreign", json.RawMessage(`{}`)))
	}
	// Enumeration must not decode values or timestamps from legacy entries.
	_, err = store.db.Exec(`UPDATE plugin_state SET value_json='invalid JSON', updated_at='invalid time' WHERE plugin_id='p'`)
	require.NoError(t, err)
	var after *string
	var got []string
	for {
		keys, more, err = pager.ListKeys(ctx, "p", "legacy", "s", after, 200)
		require.NoError(t, err)
		require.LessOrEqual(t, len(keys), 200)
		got = append(got, keys...)
		for _, key := range keys {
			require.NoError(t, store.Delete(ctx, "p", "legacy", "s", key))
		}
		if !more {
			break
		}
		last := keys[len(keys)-1]
		after = &last
		// Recreate the store between pages. Progress lives in the keyset, not memory.
		reopened, openErr := NewStore(db.NewPool(store.db, store.ro))
		require.NoError(t, openErr)
		pager, ok = any(reopened).(testStateKeyPager)
		require.True(t, ok)
	}
	require.Equal(t, expected, got)
	for _, limit := range []int{-1, 0, 201} {
		_, _, err = pager.ListKeys(ctx, "p", "legacy", "s", nil, limit)
		require.Error(t, err)
	}
}
