package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// NetworkLedgerState is the persisted lifecycle state of one Docker network
// under the docknet reclamation provider. The stable IDs matter: they ship in
// persisted rows and API payloads.
type NetworkLedgerState string

const (
	// NetworkLedgerStateObserved is the fail-open ledger record for a network
	// seen in census but not yet eligible for removal.
	NetworkLedgerStateObserved NetworkLedgerState = "observed"
	// NetworkLedgerStateMarked is the first quarantine phase: the network is
	// a removal candidate and its grace window is running.
	NetworkLedgerStateMarked NetworkLedgerState = "marked"
	// NetworkLedgerStateRemoved records a network the provider deleted.
	NetworkLedgerStateRemoved NetworkLedgerState = "removed"
	// NetworkLedgerStateSkipped records a network whose removal attempt was
	// aborted (fail-closed) before the SDK call.
	NetworkLedgerStateSkipped NetworkLedgerState = "skipped"
)

// NetworkLedgerEntry is one row of the persisted first-seen/quarantine ledger.
// FirstSeenAt is the stable-age evidence for safely-stale classification of
// networks whose only homing signal is a compose project label.
type NetworkLedgerEntry struct {
	NetworkID   string             `json:"network_id"`
	NetworkName string             `json:"network_name"`
	FirstSeenAt time.Time          `json:"first_seen_at"`
	LastSeenAt  time.Time          `json:"last_seen_at"`
	State       NetworkLedgerState `json:"state"`
	MarkedAt    *time.Time         `json:"marked_at,omitempty"`
	DeleteAfter *time.Time         `json:"delete_after,omitempty"`
	RemovedAt   *time.Time         `json:"removed_at,omitempty"`
	LastError   string             `json:"last_error"`
	Metadata    json.RawMessage    `json:"metadata"`
}

func (s *Store) GetNetworkLedgerEntry(ctx context.Context, networkID string) (NetworkLedgerEntry, error) {
	var row networkLedgerRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT network_id, network_name, first_seen_at, last_seen_at, state,
			marked_at, delete_after, removed_at, last_error, metadata
		FROM storage_network_ledger WHERE network_id = ?
	`), networkID)
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkLedgerEntry{}, ErrNotFound
	}
	if err != nil {
		return NetworkLedgerEntry{}, fmt.Errorf("get network ledger entry: %w", err)
	}
	return row.entry(), nil
}

// UpsertNetworkLedgerObserved records a census sighting. The first sighting
// fixes FirstSeenAt; later sightings only advance LastSeenAt so the stable-age
// clock can never be reset by a removal attempt.
func (s *Store) UpsertNetworkLedgerObserved(
	ctx context.Context, networkID, networkName string, at time.Time,
) (NetworkLedgerEntry, error) {
	at = at.UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO storage_network_ledger
			(network_id, network_name, first_seen_at, last_seen_at, state, metadata)
		VALUES (?, ?, ?, ?, ?, '{}')
		ON CONFLICT(network_id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			network_name = excluded.network_name
	`), networkID, networkName, at, at, NetworkLedgerStateObserved)
	if err != nil {
		return NetworkLedgerEntry{}, fmt.Errorf("upsert network ledger observation: %w", err)
	}
	return s.GetNetworkLedgerEntry(ctx, networkID)
}

// TransitionNetworkLedgerEntry moves a ledger row to the next state with an
// explicit timestamp. Removal states are terminal; the ledger never un-deletes.
func (s *Store) TransitionNetworkLedgerEntry(
	ctx context.Context,
	networkID string,
	next NetworkLedgerState,
	markedAt, deleteAfter time.Time,
	metadata json.RawMessage,
	lastError string,
) (NetworkLedgerEntry, error) {
	if !validNetworkLedgerTransition(next, markedAt, deleteAfter) {
		return NetworkLedgerEntry{}, validationError(
			"network ledger transition to %s requires marked timestamps", next,
		)
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO storage_network_ledger
			(network_id, network_name, first_seen_at, last_seen_at, state,
			 marked_at, delete_after, removed_at, last_error, metadata)
		VALUES (?, '', ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(network_id) DO UPDATE SET
			state = excluded.state,
			marked_at = excluded.marked_at,
			delete_after = excluded.delete_after,
			removed_at = excluded.removed_at,
			last_error = excluded.last_error,
			metadata = CASE WHEN excluded.metadata = '' THEN storage_network_ledger.metadata
				ELSE excluded.metadata END
	`), networkID, time.Time{}, time.Time{}, next, markedAt, deleteAfter, time.Time{}, lastError, string(metadata))
	if err != nil {
		return NetworkLedgerEntry{}, fmt.Errorf("transition network ledger entry: %w", err)
	}
	return s.GetNetworkLedgerEntry(ctx, networkID)
}

// RecordNetworkLedgerRemoved marks a network removed (terminal) at the given
// time. This is the only writer for the removed state.
func (s *Store) RecordNetworkLedgerRemoved(
	ctx context.Context, networkID, networkName string, at time.Time, runID string,
) (NetworkLedgerEntry, error) {
	at = at.UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO storage_network_ledger
			(network_id, network_name, first_seen_at, last_seen_at, state,
			 marked_at, delete_after, removed_at, last_error, metadata)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, '', ?)
		ON CONFLICT(network_id) DO UPDATE SET
			state = excluded.state,
			removed_at = COALESCE(storage_network_ledger.removed_at, excluded.removed_at),
			network_name = excluded.network_name
	`), networkID, networkName, at, at, NetworkLedgerStateRemoved, at, at, runID)
	if err != nil {
		return NetworkLedgerEntry{}, fmt.Errorf("record network ledger removal: %w", err)
	}
	return s.GetNetworkLedgerEntry(ctx, networkID)
}

// ListNetworkLedgerEntries returns every non-terminal ledger row in
// first-seen order.
func (s *Store) ListNetworkLedgerEntries(ctx context.Context) ([]NetworkLedgerEntry, error) {
	rows := make([]networkLedgerRow, 0)
	err := s.ro.SelectContext(ctx, &rows, `
		SELECT network_id, network_name, first_seen_at, last_seen_at, state,
			marked_at, delete_after, removed_at, last_error, metadata
		FROM storage_network_ledger
		WHERE state IN ('observed', 'marked', 'skipped')
		ORDER BY first_seen_at ASC, network_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list network ledger entries: %w", err)
	}
	entries := make([]NetworkLedgerEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, row.entry())
	}
	return entries, nil
}

func validNetworkLedgerTransition(next NetworkLedgerState, markedAt, deleteAfter time.Time) bool {
	switch next {
	case NetworkLedgerStateObserved, NetworkLedgerStateSkipped:
		return markedAt.IsZero() && deleteAfter.IsZero()
	case NetworkLedgerStateMarked:
		return !markedAt.IsZero() && !deleteAfter.IsZero()
	default:
		return false
	}
}

type networkLedgerRow struct {
	NetworkID   string     `db:"network_id"`
	NetworkName string     `db:"network_name"`
	FirstSeenAt time.Time  `db:"first_seen_at"`
	LastSeenAt  time.Time  `db:"last_seen_at"`
	State       string     `db:"state"`
	MarkedAt    *time.Time `db:"marked_at"`
	DeleteAfter *time.Time `db:"delete_after"`
	RemovedAt   *time.Time `db:"removed_at"`
	LastError   string     `db:"last_error"`
	Metadata    []byte     `db:"metadata"`
}

func (r networkLedgerRow) entry() NetworkLedgerEntry {
	return NetworkLedgerEntry{
		NetworkID: r.NetworkID, NetworkName: r.NetworkName,
		FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt,
		State: NetworkLedgerState(r.State), MarkedAt: r.MarkedAt,
		DeleteAfter: r.DeleteAfter, RemovedAt: r.RemovedAt,
		LastError: r.LastError, Metadata: json.RawMessage(r.Metadata),
	}
}
