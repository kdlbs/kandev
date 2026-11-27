package coordinator

import (
	"strings"
	"testing"
)

func TestUnattendedTurns_ReservedTurnIDColumn_FreshAndUpgraded(t *testing.T) {
	fresh := newTestStore(t)
	assertReservedTurnColumn(t, fresh)

	conn := openSQLitePool(t)
	upgradeFromPhase1(t, conn, false)
	upgraded, err := NewStore(conn, conn)
	if err != nil {
		t.Fatalf("NewStore replay: %v", err)
	}
	assertReservedTurnColumn(t, upgraded)
}

func assertReservedTurnColumn(t *testing.T, store *Store) {
	t.Helper()
	for _, col := range tableColumns(t, store.db, "coordinator_unattended_turns") {
		if strings.HasPrefix(col, "reserved_turn_id ") {
			if !strings.Contains(col, " 0 ") {
				t.Fatalf("reserved_turn_id must be nullable: %q", col)
			}
			return
		}
	}
	t.Fatal("coordinator_unattended_turns.reserved_turn_id is missing")
}
