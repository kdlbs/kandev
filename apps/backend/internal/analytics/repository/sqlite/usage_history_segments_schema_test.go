package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageHistoryPointAndSegmentSchemaReplay(t *testing.T) {
	repo, _ := historyRetainedContractFixture(t)
	ctx := t.Context()
	_, err := repo.db.ExecContext(ctx, `INSERT INTO usage_collection_current_targets(scope_id,subject_id,binding_id,task_id,session_id,external_session_id) VALUES ('schema-scope','schema-subject','binding-a','task','session','transcript')`)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `INSERT INTO usage_collection_source_guards(source_hash,scope_hash,mixed,erased) VALUES ('source-hash','scope-hash',0,0)`)
	require.NoError(t, err)
	_, err = NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE usage_collection_current_targets SET binding_id='binding-b' WHERE scope_id='schema-scope' AND subject_id='schema-subject'`)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE usage_collection_source_guards SET erased=1 WHERE source_hash='source-hash'`)
	require.NoError(t, err)
	var binding string
	require.NoError(t, repo.db.Get(&binding, `SELECT binding_id FROM usage_collection_current_targets WHERE scope_id='schema-scope' AND subject_id='schema-subject'`))
	require.Equal(t, "binding-b", binding)
	var erased int
	require.NoError(t, repo.db.Get(&erased, `SELECT erased FROM usage_collection_source_guards WHERE source_hash='source-hash'`))
	require.Equal(t, 1, erased)
	_, err = repo.db.ExecContext(ctx, `DELETE FROM usage_collection_current_targets WHERE scope_id='schema-scope'`)
	require.NoError(t, err)
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM usage_collection_current_targets WHERE scope_id='schema-scope'`))
	require.Zero(t, count)
	// The privacy fence is control-plane state and survives target-pointer removal.
	require.NoError(t, repo.db.Get(&erased, `SELECT erased FROM usage_collection_source_guards WHERE source_hash='source-hash'`))
	require.Equal(t, 1, erased)
}
