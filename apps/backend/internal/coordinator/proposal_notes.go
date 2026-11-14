package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Reasons a proposal's note_ids are refused.
const (
	NoteRefUnavailable  = "notes_unavailable"
	NoteRefTooMany      = "too_many_notes"
	NoteRefMalformed    = "note_malformed"
	NoteRefDuplicate    = "note_duplicate"
	NoteRefUnknown      = "note_unknown"
	NoteRefDeleted      = "note_deleted"
	NoteRefExpired      = "note_expired"
	NoteRefStale        = "note_stale"
	NoteRefNotDelivered = "note_not_delivered"
)

// ProposalNoteOffence is one refused note_ids entry.
type ProposalNoteOffence struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ProposalNotesError refuses a proposal because of its note_ids. Reason is that
// of the first offending entry (or the request-level reason); Offences lists
// every offending entry, and is empty for a request-level reason.
type ProposalNotesError struct {
	Reason   string
	Offences []ProposalNoteOffence
}

func (e *ProposalNotesError) Error() string {
	if len(e.Offences) == 0 {
		return "note_ids rejected: " + e.Reason
	}
	parts := make([]string, len(e.Offences))
	for i, o := range e.Offences {
		parts[i] = o.ID + " (" + o.Reason + ")"
	}
	return "note_ids rejected: " + e.Reason + ": " + strings.Join(parts, ", ")
}

type proposalNotesKey struct{}

// proposalNotesRequest is the note_ids of one proposal call with the notes the
// calling turn's frozen index delivered, by id.
type proposalNotesRequest struct {
	raw       json.RawMessage
	delivered map[string]int
}

// WithProposalNotes carries a proposal call's raw note_ids, and the frozen
// index of the calling session's active turn, to the proposal insert, so every
// proposal kind shares one path. A call without note_ids returns ctx unchanged.
func (s *Service) WithProposalNotes(ctx context.Context, coordinatorID, sessionID string, raw json.RawMessage) context.Context {
	if isEmptyNoteIDs(raw) {
		return ctx
	}
	req := &proposalNotesRequest{raw: raw, delivered: map[string]int{}}
	if s.notes && s.activeTurns != nil && sessionID != "" {
		if turn, err := s.activeTurns.GetActiveTurn(ctx, sessionID); err == nil && turn != nil {
			if snap, ok, err := s.store.GetNoteSnapshot(ctx, turn.ID); err == nil && ok && snap.CoordinatorID == coordinatorID {
				for _, e := range snap.Entries {
					req.delivered[e.NoteID] = e.Version
				}
			}
		}
	}
	return context.WithValue(ctx, proposalNotesKey{}, req)
}

func isEmptyNoteIDs(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return t == "" || t == "null" || t == "[]"
}

// parseProposalNoteIDs applies the request-level checks and returns the raw
// entries (nil for a non-string entry, which is malformed).
func (s *Store) parseProposalNoteIDs(req *proposalNotesRequest) ([]*string, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(req.raw, &entries); err != nil {
		return nil, &ProposalNotesError{Reason: NoteRefMalformed}
	}
	if len(entries) == 0 {
		return nil, nil
	}
	if !s.notesOn {
		return nil, &ProposalNotesError{Reason: NoteRefUnavailable}
	}
	if len(entries) > maxProposalNoteIDs {
		return nil, &ProposalNotesError{Reason: NoteRefTooMany}
	}
	out := make([]*string, len(entries))
	for i, raw := range entries {
		var v string
		if json.Unmarshal(raw, &v) == nil {
			out[i] = &v
		}
	}
	return out, nil
}

// checkProposalNotes validates note_ids inside the proposal transaction
// against the committed state of each note and returns the links to store. It
// returns nil links when the call carries no note_ids.
func (s *Store) checkProposalNotes(ctx context.Context, tx coordinatorExec, coordinatorID string) ([]noteLink, error) {
	req, _ := ctx.Value(proposalNotesKey{}).(*proposalNotesRequest)
	if req == nil {
		return nil, nil
	}
	entries, err := s.parseProposalNoteIDs(req)
	if err != nil || entries == nil {
		return nil, err
	}
	at := s.now().UTC()
	seen := map[string]bool{}
	var links []noteLink
	var offences []ProposalNoteOffence
	for _, e := range entries {
		id, reason := "", NoteRefMalformed
		if e != nil {
			id = *e
			var jerr error
			reason, jerr = s.judgeProposalNote(ctx, tx, coordinatorID, req, id, seen, at)
			if jerr != nil {
				return nil, jerr
			}
			id = strings.ToLower(id)
		}
		if reason != "" {
			offences = append(offences, ProposalNoteOffence{ID: id, Reason: reason})
			continue
		}
		links = append(links, noteLink{NoteID: id, Version: req.delivered[id]})
	}
	if len(offences) > 0 {
		return nil, &ProposalNotesError{Reason: offences[0].Reason, Offences: offences}
	}
	return links, nil
}

type noteLink struct {
	NoteID  string
	Version int
}

// judgeProposalNote returns the first failing reason of one entry, or "".
func (s *Store) judgeProposalNote(ctx context.Context, tx coordinatorExec, coordinatorID string, req *proposalNotesRequest, raw string, seen map[string]bool, at time.Time) (string, error) {
	parsed, err := uuid.Parse(raw)
	if err != nil || len(raw) != 36 {
		return NoteRefMalformed, nil
	}
	id := parsed.String()
	if dup := seen[id]; dup {
		return NoteRefDuplicate, nil
	}
	seen[id] = true
	n, err := s.noteOn(ctx, tx, coordinatorID, id)
	if errors.Is(err, ErrNotFound) {
		return NoteRefUnknown, nil
	}
	if err != nil {
		return "", fmt.Errorf("read proposal note: %w", err)
	}
	switch {
	case n.DeletedAt != nil:
		return NoteRefDeleted, nil
	case n.expired(at):
		return NoteRefExpired, nil
	case n.LastResult == validationStale:
		return NoteRefStale, nil
	}
	if _, ok := req.delivered[id]; !ok {
		return NoteRefNotDelivered, nil
	}
	return "", nil
}

func (s *Store) insertNoteLinks(ctx context.Context, tx coordinatorExec, proposalID string, links []noteLink) error {
	for _, l := range links {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_proposal_notes
			(proposal_id, note_id, note_version) VALUES (?, ?, ?)`), proposalID, l.NoteID, l.Version); err != nil {
			return fmt.Errorf("link proposal note: %w", err)
		}
	}
	return nil
}
