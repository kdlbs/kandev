package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresCutoverPreservesWorkspaceAdoptionForeignKey(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seed := seedHybridCutoverState(t, db, "adoption")
	repo := NewWithInitializedDB(db, db, nil)
	require.NoError(t, repo.ensureWorkspaceAdoptionsSchema())
	const operation = "11111111-1111-4111-8111-111111111111"
	receipt := models.WorkspaceAdoptionReceipt{OperationID: operation, CreatedAt: time.Now().UTC()}
	receipt.Preview.Selection.Snapshot.TaskEnvironmentID = seed.envID
	receipt.Preview.Proof.MarkerSHA256 = "immutable-retired-marker-provenance"
	encoded, err := json.Marshal(receipt)
	require.NoError(t, err)
	_, err = db.Exec(db.Rebind(`INSERT INTO task_environment_checkout_adoptions(operation_id,task_environment_id,slot_id,receipt_json,created_at) VALUES(?,?,?,?,?)`), operation, seed.envID, "retained-slot", string(encoded), receipt.CreatedAt)
	require.NoError(t, err)
	_, err = NewWithDB(db, db, nil)
	require.NoError(t, err)
	var data, environmentID, referencedTable string
	require.NoError(t, db.QueryRow(db.Rebind(`SELECT receipt_json,task_environment_id FROM task_environment_checkout_adoptions WHERE operation_id=?`), operation).Scan(&data, &environmentID))
	require.Equal(t, string(encoded), data)
	require.Equal(t, seed.envID, environmentID)
	require.NoError(t, db.Get(&referencedTable, `SELECT constraint_row.confrelid::regclass::text FROM pg_constraint constraint_row WHERE constraint_row.conrelid='task_environment_checkout_adoptions'::regclass AND constraint_row.contype='f'`))
	require.Equal(t, "task_environments", referencedTable)
	_, err = db.Exec(db.Rebind(`DELETE FROM task_environments WHERE id=?`), seed.envID)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM task_environment_checkout_adoptions`))
	require.Zero(t, count)
}
