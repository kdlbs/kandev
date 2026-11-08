package coordinator

import (
	"context"
	"errors"
)

// ApprovedNoteItem is an already approved dream item to store as a note. The
// caller maps the item to these fields and owns the approval; this seam grants
// none.
type ApprovedNoteItem struct {
	ItemID    string
	Model     string
	Type      string
	Text      string
	Citations []NoteCitation
}

// CreateApprovedItemNote stores an approved item as a note with the producing
// model's stamp, applying the same validation as a manager create. A repeat of
// the same item with the same content returns the existing note; different
// content for that item is a version conflict. It is reachable from no route
// or tool.
func (s *Service) CreateApprovedItemNote(ctx context.Context, coordinatorID string, item ApprovedNoteItem) (*NoteView, error) {
	if !s.notes || item.ItemID == "" || item.Model == "" {
		return nil, ErrNotFound
	}
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	f := &noteFlow{s: s, c: c, req: &noteRequest{op: noteOpCreate, typ: item.Type, text: item.Text, cites: item.Citations}}
	if nerr := f.shape(ctx); nerr != nil {
		return nil, nerr
	}
	f.content.Provenance = NoteProvenance{Kind: ProvenanceModel, Model: item.Model}
	if nerr := f.resolve(ctx); nerr != nil {
		return nil, nerr
	}
	var out *NoteView
	err = s.store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		now := s.store.now()
		existing, err := s.store.noteBySourceItemOn(ctx, tx, c.ID, item.ItemID)
		if err == nil {
			f.note = existing
			if !f.equalsCurrent() {
				return noteConflict(ConflictVersion)
			}
			v := newNoteView(existing, existing.baseStatus(now))
			out = &v
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		out, err = s.createLocked(ctx, tx, c, f.content, "", item.ItemID, now)
		return err
	})
	return out, err
}
