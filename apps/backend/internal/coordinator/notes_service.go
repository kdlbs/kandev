package coordinator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/authz"
)

// NoteView is the wire shape of a note.
type NoteView struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	Text            string         `json:"text"`
	Citations       []NoteCitation `json:"citations"`
	Provenance      NoteProvenance `json:"provenance"`
	Version         int            `json:"version"`
	Status          NoteStatus     `json:"status"`
	CreatedAt       time.Time      `json:"created_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
	DeletedAt       *time.Time     `json:"deleted_at,omitempty"`
	UseCount        int            `json:"use_count"`
	LastValidatedAt *time.Time     `json:"last_validated_at,omitempty"`
	LastAttemptAt   *time.Time     `json:"last_attempt_at,omitempty"`
	ReferencedBy    *NoteReferrers `json:"referenced_by,omitempty"`
}

// NoteDetail is a note with its versions.
type NoteDetail struct {
	NoteView
	Versions []NoteVersion `json:"versions"`
}

func newNoteView(n *Note, status NoteStatus) NoteView {
	return NoteView{
		ID: n.ID, Type: n.Current.Type, Text: n.Current.Text, Citations: n.Current.Citations,
		Provenance: n.Current.Provenance, Version: n.CurrentVer, Status: status,
		CreatedAt: n.CreatedAt, ExpiresAt: n.ExpiresAt, DeletedAt: n.DeletedAt, UseCount: n.UseCount,
		LastValidatedAt: n.LastValidatedAt, LastAttemptAt: n.LastAttemptAt,
	}
}

// noteCoordinator applies the flag, the authorization (403) and the
// coordinator lookup (404) shared by every note route.
func (s *Service) noteCoordinator(ctx context.Context, workspaceID, coordinatorID string, scope authz.Scope) (*Coordinator, error) {
	if !s.notes {
		return nil, ErrNotFound
	}
	return s.authorizedCoordinator(ctx, workspaceID, coordinatorID, scope)
}

// overCapIDs derives the notes ranked beyond the index cap among those that are
// neither deleted, expired, stale nor unavailable.
func overCapIDs(notes []*Note, at time.Time) map[string]bool {
	over := map[string]bool{}
	rank := 0
	for _, n := range notes {
		switch n.baseStatus(at) {
		case NoteStatusNotValidated, NoteStatusLive:
			rank++
			if rank > noteIndexLines {
				over[n.ID] = true
			}
		}
	}
	return over
}

func (s *Service) noteViews(notes []*Note, at time.Time) []NoteView {
	over := overCapIDs(notes, at)
	views := make([]NoteView, len(notes))
	for i, n := range notes {
		status := n.baseStatus(at)
		if status == NoteStatusLive && over[n.ID] {
			status = NoteStatusOverCap
		}
		views[i] = newNoteView(n, status)
	}
	return views
}

// ListNotes returns the coordinator's notes with the current references of
// each.
func (s *Service) ListNotes(ctx context.Context, workspaceID, coordinatorID string, includeDeleted bool) ([]NoteView, error) {
	c, err := s.noteCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	notes, err := s.store.ListNotes(ctx, c.ID, true)
	if err != nil {
		return nil, err
	}
	all := s.noteViews(notes, s.store.now())
	views := all[:0]
	for _, v := range all {
		if includeDeleted || v.Status != NoteStatusDeleted {
			views = append(views, v)
		}
	}
	return s.attachReferrers(ctx, c.ID, views)
}

// GetNote returns one note with its versions and current references.
func (s *Service) GetNote(ctx context.Context, workspaceID, coordinatorID, noteID string) (*NoteDetail, error) {
	c, err := s.noteCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	return s.noteDetail(ctx, c, noteID)
}

func (s *Service) noteDetail(ctx context.Context, c *Coordinator, noteID string) (*NoteDetail, error) {
	notes, err := s.store.ListNotes(ctx, c.ID, true)
	if err != nil {
		return nil, err
	}
	views := s.noteViews(notes, s.store.now())
	for _, v := range views {
		if v.ID != noteID {
			continue
		}
		versions, err := s.store.ListNoteVersions(ctx, noteID)
		if err != nil {
			return nil, err
		}
		withRefs, err := s.attachReferrers(ctx, c.ID, []NoteView{v})
		if err != nil {
			return nil, err
		}
		return &NoteDetail{NoteView: withRefs[0], Versions: versions}, nil
	}
	return nil, ErrNotFound
}

// NoteVersionDetail returns one version of a note.
func (s *Service) NoteVersionDetail(ctx context.Context, workspaceID, coordinatorID, noteID string, version int) (*NoteVersion, error) {
	c, err := s.noteCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetNote(ctx, c.ID, noteID); err != nil {
		return nil, err
	}
	v, err := s.store.noteVersionOn(ctx, s.store.ro, noteID, version)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// noteFlow carries one mutation through the evaluation order.
type noteFlow struct {
	s    *Service
	c    *Coordinator
	req  *noteRequest
	note *Note // nil for create
	user string
	// content is the resulting content of an edit, create or revert.
	content noteContent
}

// MutateNote runs a create, edit, revert or delete in the fixed evaluation
// order. noteID is empty for a create.
func (s *Service) MutateNote(ctx context.Context, op, workspaceID, coordinatorID, noteID string, body []byte, tooLarge bool) (*NoteView, error) {
	c, err := s.noteCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	f := &noteFlow{s: s, c: c, user: decidingUserID(ctx)}
	if op != noteOpCreate {
		if f.note, err = s.store.GetNote(ctx, c.ID, noteID); err != nil {
			return nil, err
		}
	}
	req, nerr := parseNoteRequest(op, body, tooLarge)
	if nerr != nil {
		return nil, nerr
	}
	f.req = req
	return f.run(ctx, workspaceID)
}

func (f *noteFlow) run(ctx context.Context, workspaceID string) (*NoteView, error) {
	now := f.s.store.now()
	if done, err := f.earlyChecks(ctx, now); done != nil || err != nil {
		return done, err
	}
	if f.req.op == noteOpCreate {
		n, err := f.s.store.countLiveNotesOn(ctx, f.s.store.ro, f.c.ID)
		if err != nil {
			return nil, err
		}
		if n >= maxLiveNotes {
			return nil, noteErr(NoteLimitReached, "")
		}
	}
	if f.req.op == noteOpCreate || f.req.op == noteOpEdit {
		if nerr := f.resolve(ctx); nerr != nil {
			return nil, nerr
		}
	}
	if err := f.s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	return f.commit(ctx)
}

// earlyChecks runs the state, no-op, version and shape checks before any I/O. A non-nil view is a successful early
// outcome (repeated delete or no-op).
func (f *noteFlow) earlyChecks(ctx context.Context, now time.Time) (*NoteView, error) {
	if view, err := f.stateStep(f.note, now); view != nil || err != nil {
		return view, err
	}
	if f.req.op != noteOpDelete {
		if nerr := f.shape(ctx); nerr != nil {
			return nil, nerr
		}
		if f.note != nil && f.equalsCurrent() {
			v := newNoteView(f.note, f.note.baseStatus(now))
			return &v, nil
		}
	}
	if f.note != nil && f.req.version != f.note.CurrentVer {
		return nil, noteConflict(ConflictVersion)
	}
	return nil, nil
}

// stateStep checks deleted/expired state against a note read now or at the commit.
func (f *noteFlow) stateStep(n *Note, now time.Time) (*NoteView, error) {
	if n == nil {
		return nil, nil
	}
	if f.req.op == noteOpDelete {
		if n.DeletedAt != nil {
			v := newNoteView(n, NoteStatusDeleted)
			return &v, nil
		}
		return nil, nil
	}
	if n.DeletedAt != nil {
		return nil, noteConflict(ConflictDeleted)
	}
	if n.expired(now) {
		return nil, noteConflict(ConflictExpired)
	}
	return nil, nil
}

// shape checks content shape and the credential gate.
func (f *noteFlow) shape(ctx context.Context) *NoteError {
	req := f.req
	typ, text, cites := req.typ, req.text, req.cites
	if req.op == noteOpRevert {
		if req.target > f.note.CurrentVer {
			return noteErr(NoteInvalidRequest, fieldTargetVersion)
		}
		v, err := f.s.store.noteVersionOn(ctx, f.s.store.ro, f.note.ID, req.target)
		if err != nil {
			return noteErr(NoteInvalidRequest, fieldTargetVersion)
		}
		typ, text, cites = v.Type, v.Text, v.Citations
		f.content.RevertedFrom = req.target
		f.content.Provenance = v.Provenance
	}
	if !validNoteType(typ) {
		return noteErr(NoteInvalidType, "type")
	}
	text, ok := normalizeNoteText(text)
	if !ok {
		return noteErr(NoteInvalidTextLength, "text")
	}
	norm, nerr := validateCitations(cites)
	if nerr != nil {
		return nerr
	}
	if noteHasCredential(text, norm) {
		return noteCredentialErr(text, norm)
	}
	f.content.Type, f.content.Text, f.content.Citations = typ, text, norm
	return nil
}

func (f *noteFlow) equalsCurrent() bool {
	cur := f.note.Current
	return f.content.Type == cur.Type && f.content.Text == cur.Text &&
		citationSetKey(f.content.Citations) == citationSetKey(cur.Citations)
}

// resolve requires every citation must resolve within the deadline.
func (f *noteFlow) resolve(ctx context.Context) *NoteError {
	checker := f.s.openChecker(ctx, f.c)
	states := runCitationJobs(ctx, checker, f.content.Citations, citationDeadline)
	result, absent := foldCitationStates(states)
	switch {
	case absent >= 0:
		return &NoteError{Code: NoteCitationUnres, Field: fieldCitations, Index: &absent}
	case result == validationUnavailable:
		return noteErr(NoteUnavailable, fieldCitations)
	}
	return nil
}

func noteHasCredential(text string, cites []NoteCitation) bool {
	if ContainsCredential(text) || ContainsCredential(sanitizeNoteLine(text)) {
		return true
	}
	for _, c := range cites {
		for _, field := range c.citationFields() {
			if ContainsCredential(field) {
				return true
			}
		}
	}
	return false
}

// noteCredentialErr names only the field that carried the credential.
func noteCredentialErr(text string, cites []NoteCitation) *NoteError {
	if ContainsCredential(text) {
		return noteErr(NoteCredential, "text")
	}
	return noteErr(NoteCredential, fieldCitations)
}

// commit is one short transaction under the coordinator lock that
// re-runs the state, no-op, version and limit checks against committed state, then writes.
func (f *noteFlow) commit(ctx context.Context) (*NoteView, error) {
	var out *NoteView
	err := f.s.store.withCoordinatorLock(ctx, f.c.ID, func(tx coordinatorExec) error {
		var err error
		out, err = f.commitLocked(ctx, tx)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (f *noteFlow) commitLocked(ctx context.Context, tx coordinatorExec) (*NoteView, error) {
	st := f.s.store
	now := st.now()
	var cur *Note
	if f.note != nil {
		var err error
		if cur, err = st.noteOn(ctx, tx, f.c.ID, f.note.ID); err != nil {
			return nil, err
		}
	}
	if view, err := f.stateStep(cur, now); view != nil || err != nil {
		return view, err
	}
	if cur != nil && f.req.op != noteOpDelete {
		f.note = cur
		if f.equalsCurrent() {
			v := newNoteView(cur, cur.baseStatus(now))
			return &v, nil
		}
	}
	if cur != nil && f.req.version != cur.CurrentVer {
		return nil, noteConflict(ConflictVersion)
	}
	return f.write(ctx, tx, cur, now)
}

func (f *noteFlow) write(ctx context.Context, tx coordinatorExec, cur *Note, now time.Time) (*NoteView, error) {
	st := f.s.store
	switch f.req.op {
	case noteOpCreate:
		return f.s.createLocked(ctx, tx, f.c, f.content, f.user, "", now)
	case noteOpDelete:
		if _, err := st.tombstoneNoteOn(ctx, tx, cur.ID, now); err != nil {
			return nil, err
		}
	default:
		content := f.content
		if content.Provenance.Kind == "" || f.req.op == noteOpEdit {
			content.Provenance = NoteProvenance{Kind: ProvenanceHuman, UserID: f.user}
		}
		if f.req.op == noteOpRevert {
			content.ActorID = f.user
		}
		ok, err := st.appendVersionOn(ctx, tx, cur.ID, cur.CurrentVer, content, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, noteConflict(ConflictVersion)
		}
	}
	n, err := st.noteOn(ctx, tx, f.c.ID, cur.ID)
	if err != nil {
		return nil, err
	}
	v := newNoteView(n, n.baseStatus(now))
	return &v, nil
}

// createLocked recounts under the lock and inserts a new note.
func (s *Service) createLocked(ctx context.Context, tx coordinatorExec, c *Coordinator, content noteContent, user, sourceItem string, now time.Time) (*NoteView, error) {
	count, err := s.store.countLiveNotesOn(ctx, tx, c.ID)
	if err != nil {
		return nil, err
	}
	if count >= maxLiveNotes {
		return nil, noteErr(NoteLimitReached, "")
	}
	if content.Provenance.Kind == "" {
		content.Provenance = NoteProvenance{Kind: ProvenanceHuman, UserID: user}
	}
	id, err := s.store.insertNoteOn(ctx, tx, c, content, sourceItem, now)
	if err != nil {
		return nil, err
	}
	n, err := s.store.noteOn(ctx, tx, c.ID, id)
	if err != nil {
		return nil, err
	}
	v := newNoteView(n, NoteStatusNotValidated)
	return &v, nil
}
