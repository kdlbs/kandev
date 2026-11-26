package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// noteSnapshotEntry is one delivered note line frozen for a session turn.
type noteSnapshotEntry struct {
	NoteID  string `json:"note_id"`
	Version int    `json:"version"`
	Line    string `json:"line"`
}

// noteSnapshot is the frozen note selection of one session turn. A failed
// snapshot is always empty.
type noteSnapshot struct {
	TurnID        string
	CoordinatorID string
	Failed        bool
	Entries       []noteSnapshotEntry
}

// GetNoteSnapshot loads the frozen selection of a session turn.
func (s *Store) GetNoteSnapshot(ctx context.Context, turnID string) (*noteSnapshot, bool, error) {
	var snap noteSnapshot
	var failed int
	var raw string
	err := s.ro.QueryRowContext(ctx, s.db.Rebind(`SELECT session_turn_id, coordinator_id, failed, entries
		FROM coordinator_note_snapshots WHERE session_turn_id = ?`), turnID).
		Scan(&snap.TurnID, &snap.CoordinatorID, &failed, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load note snapshot: %w", err)
	}
	snap.Failed = failed != 0
	if err := json.Unmarshal([]byte(raw), &snap.Entries); err != nil {
		return nil, false, fmt.Errorf("decode note snapshot: %w", err)
	}
	return &snap, true, nil
}

// InsertNoteSnapshot stores the frozen selection unless the turn already has
// one. It reports whether this call stored it.
func (s *Store) InsertNoteSnapshot(ctx context.Context, snap *noteSnapshot, at time.Time) (bool, error) {
	entries := snap.Entries
	if entries == nil {
		entries = []noteSnapshotEntry{}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return false, fmt.Errorf("encode note snapshot: %w", err)
	}
	failed := 0
	if snap.Failed {
		failed = 1
	}
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_note_snapshots
		(session_turn_id, coordinator_id, failed, entries, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (session_turn_id) DO NOTHING`), snap.TurnID, snap.CoordinatorID, failed, string(raw), at)
	if err != nil {
		return false, fmt.Errorf("store note snapshot: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RecordNoteDelivery writes the receipts of an accepted turn and, in the same
// transaction, counts each newly receipted note and extends its expiry. A note
// deleted or not strictly unexpired at the commit time (read inside the lock) gets a receipt only. A
// repeated call for the same turn changes nothing.
func (s *Store) RecordNoteDelivery(ctx context.Context, coordinatorID, turnID string) error {
	snap, found, err := s.GetNoteSnapshot(ctx, turnID)
	if err != nil || !found || snap.CoordinatorID != coordinatorID || len(snap.Entries) == 0 {
		return err
	}
	return s.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		at := s.now().UTC()
		for _, e := range snap.Entries {
			if err := s.deliverNoteOn(ctx, tx, coordinatorID, turnID, e, at); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) deliverNoteOn(ctx context.Context, tx coordinatorExec, coordinatorID, turnID string, e noteSnapshotEntry, at time.Time) error {
	n, err := s.noteOn(ctx, tx, coordinatorID, e.NoteID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	counted := n.DeletedAt == nil && n.ExpiresAt.After(at)
	flag := 0
	if counted {
		flag = 1
	}
	res, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_note_receipts
		(note_id, session_turn_id, note_version, counted, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (note_id, session_turn_id) DO NOTHING`), e.NoteID, turnID, e.Version, flag, at)
	if err != nil {
		return fmt.Errorf("record note receipt: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 || !counted {
		return nil
	}
	expiry := n.ExpiresAt
	if ext := at.Add(noteTTL); ext.After(expiry) {
		expiry = ext
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_notes SET use_count = use_count + 1, expires_at = ?
		WHERE id = ?`), expiry, e.NoteID); err != nil {
		return fmt.Errorf("count note use: %w", err)
	}
	return nil
}

// NoteReceipt is a delivery of a note version to a session turn.
type NoteReceipt struct {
	NoteID    string    `json:"note_id"`
	Version   int       `json:"version"`
	Counted   bool      `json:"counted"`
	CreatedAt time.Time `json:"created_at"`
}

// NoteReceiptsForTurn lists the notes a session turn received.
func (s *Store) NoteReceiptsForTurn(ctx context.Context, turnID string) ([]NoteReceipt, error) {
	rows, err := s.ro.QueryContext(ctx, s.db.Rebind(`SELECT note_id, note_version, counted, created_at
		FROM coordinator_note_receipts WHERE session_turn_id = ? ORDER BY created_at, note_id`), turnID)
	if err != nil {
		return nil, fmt.Errorf("list note receipts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NoteReceipt
	for rows.Next() {
		var r NoteReceipt
		var counted int
		if err := rows.Scan(&r.NoteID, &r.Version, &counted, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Counted, r.CreatedAt = counted != 0, r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
