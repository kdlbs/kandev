package coordinator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/authz"
)

// NoteReferrers lists what currently relies on a note.
type NoteReferrers struct {
	Proposals []NoteProposalRef `json:"proposals"`
	Tasks     []NoteTaskRef     `json:"tasks"`
}

// NoteProposalRef is a pending or approving proposal that cites a note.
type NoteProposalRef struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// NoteTaskRef is an open task created from an approved proposal citing a note.
type NoteTaskRef struct {
	ID string `json:"id"`
}

// NoteReference is one stored link between a proposal and a note.
type NoteReference struct {
	ProposalID    string `json:"proposal_id"`
	ProposalState string `json:"status"`
	NoteVersion   int    `json:"note_version"`
}

// noteReferrers reads, from committed state, the open proposals and open
// created tasks of every note of the coordinator, by note id.
func (s *Store) noteReferrers(ctx context.Context, coordinatorID string) (map[string]*NoteReferrers, error) {
	out := map[string]*NoteReferrers{}
	slot := func(id string) *NoteReferrers {
		if out[id] == nil {
			out[id] = &NoteReferrers{Proposals: []NoteProposalRef{}, Tasks: []NoteTaskRef{}}
		}
		return out[id]
	}
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT pn.note_id, p.id, p.status
		FROM coordinator_proposal_notes pn JOIN coordinator_proposals p ON p.id = pn.proposal_id
		WHERE p.coordinator_id = ? AND p.status IN ('pending', 'approving')
		ORDER BY p.created_at, p.id`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("read note proposal references: %w", err)
	}
	for rows.Next() {
		var note string
		var ref NoteProposalRef
		if err := rows.Scan(&note, &ref.ID, &ref.Status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		r := slot(note)
		r.Proposals = append(r.Proposals, ref)
	}
	if err := rows.Close(); err != nil || rows.Err() != nil {
		return nil, fmt.Errorf("read note proposal references: %w", err)
	}
	trows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT pn.note_id, t.id
		FROM coordinator_proposal_notes pn
		JOIN coordinator_proposals p ON p.id = pn.proposal_id
		JOIN tasks t ON t.id = p.task_id
		WHERE p.coordinator_id = ? AND p.status = 'approved' AND t.archived_at IS NULL
			AND t.state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
		ORDER BY t.created_at, t.id`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("read note task references: %w", err)
	}
	defer func() { _ = trows.Close() }()
	seen := map[[2]string]bool{}
	for trows.Next() {
		var note, task string
		if err := trows.Scan(&note, &task); err != nil {
			return nil, err
		}
		if seen[[2]string{note, task}] {
			continue
		}
		seen[[2]string{note, task}] = true
		r := slot(note)
		r.Tasks = append(r.Tasks, NoteTaskRef{ID: task})
	}
	return out, trows.Err()
}

func (s *Service) attachReferrers(ctx context.Context, coordinatorID string, views []NoteView) ([]NoteView, error) {
	refs, err := s.store.noteReferrers(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	for i := range views {
		r := refs[views[i].ID]
		if r == nil {
			r = &NoteReferrers{Proposals: []NoteProposalRef{}, Tasks: []NoteTaskRef{}}
		}
		views[i].ReferencedBy = r
	}
	return views, nil
}

// NoteReferenceHistory returns every proposal link ever stored for the note,
// including after the note was deleted.
func (s *Service) NoteReferenceHistory(ctx context.Context, workspaceID, coordinatorID, noteID string) ([]NoteReference, error) {
	c, err := s.noteCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetNote(ctx, c.ID, noteID); err != nil {
		return nil, err
	}
	rows, err := s.store.ro.QueryContext(ctx, s.store.ro.Rebind(`SELECT p.id, p.status, pn.note_version
		FROM coordinator_proposal_notes pn JOIN coordinator_proposals p ON p.id = pn.proposal_id
		WHERE pn.note_id = ? AND p.coordinator_id = ? ORDER BY p.created_at, p.id`), noteID, c.ID)
	if err != nil {
		return nil, fmt.Errorf("read note reference history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []NoteReference{}
	for rows.Next() {
		var r NoteReference
		if err := rows.Scan(&r.ProposalID, &r.ProposalState, &r.NoteVersion); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
