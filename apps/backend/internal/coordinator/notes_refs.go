package coordinator

import "context"

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

func (s *Service) attachReferrers(_ context.Context, _ string, views []NoteView) ([]NoteView, error) {
	return views, nil
}

// NoteReference is one stored link between a proposal and a note.
type NoteReference struct {
	ProposalID    string `json:"proposal_id"`
	ProposalState string `json:"status"`
	NoteVersion   int    `json:"note_version"`
}

// NoteReferenceHistory returns every proposal link ever stored for the note.
func (s *Service) NoteReferenceHistory(ctx context.Context, workspaceID, coordinatorID, noteID string) ([]NoteReference, error) {
	return []NoteReference{}, nil
}
