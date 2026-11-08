package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/service"
)

type fakeChecker struct {
	mu     sync.Mutex
	states map[string]CitationState
	calls  int
}

func (f *fakeChecker) Check(_ context.Context, c NoteCitation) CitationState {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if st, ok := f.states[c.canonicalKey()]; ok {
		return st
	}
	return CitationPresent
}

func (f *fakeChecker) Open(context.Context, string, string) CitationChecker { return f }

type notesFixture struct {
	t     *testing.T
	store *Store
	c     *Coordinator
	svc   *Service
	check *fakeChecker
	now   time.Time
}

func newNotesFixture(t *testing.T) *notesFixture {
	t.Helper()
	store := newTestStore(t)
	f := &notesFixture{t: t, store: store, c: newTestCoordinator(t, store, "ws-1"), check: &fakeChecker{states: map[string]CitationState{}},
		now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	f.svc = NewService(store, NewValidator(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t), WithPhase2(true), WithNotes(true))
	f.svc.SetNoteResolver(f.check)
	if _, err := store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, title TEXT, state TEXT,
		archived_at TIMESTAMP, created_at TIMESTAMP, updated_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { f.now = f.now.Add(time.Second); return f.now }
	return f
}

func (f *notesFixture) mutate(op, noteID, body string) (*NoteView, error) {
	return f.svc.MutateNote(context.Background(), op, f.c.WorkspaceID, f.c.ID, noteID, []byte(body), false)
}

func (f *notesFixture) create(text string) *NoteView {
	f.t.Helper()
	v, err := f.mutate(noteOpCreate, "", fmt.Sprintf(`{"type":"project","text":%q,"citations":[]}`, text))
	if err != nil {
		f.t.Fatalf("create: %v", err)
	}
	return v
}

func wantNoteErr(t *testing.T, err error, code, reason string) {
	t.Helper()
	var ne *NoteError
	if !errors.As(err, &ne) || ne.Code != code || ne.Reason != reason {
		t.Fatalf("err = %v, want code %q reason %q", err, code, reason)
	}
}

func TestNoteCreateDefaults(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("  release target is November  ")
	if v.Version != 1 || v.UseCount != 0 || v.Text != "release target is November" || v.Status != NoteStatusNotValidated {
		t.Fatalf("view = %+v", v)
	}
	if v.Provenance.Kind != ProvenanceHuman || !v.ExpiresAt.Equal(v.CreatedAt.Add(noteTTL)) || v.LastValidatedAt != nil {
		t.Fatalf("view = %+v", v)
	}
}

func TestNoteFlagOffIsNotFound(t *testing.T) {
	f := newNotesFixture(t)
	f.svc.notes = false
	_, err := f.mutate(noteOpCreate, "", `{"type":"user","text":"x"}`)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoteForbiddenBeforeNotFound(t *testing.T) {
	f := newNotesFixture(t)
	f.svc.authz = &fakeWorkspaceAuthorizer{err: service.ErrForbidden}
	_, err := f.svc.MutateNote(context.Background(), noteOpEdit, f.c.WorkspaceID, "missing", "missing", []byte(`{}`), false)
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden before not found", err)
	}
}

func TestNoteShapeRejections(t *testing.T) {
	f := newNotesFixture(t)
	cases := []struct{ name, body, code string }{
		{"bad type", `{"type":"x","text":"a"}`, NoteInvalidType},
		{"empty text", `{"type":"user","text":"   "}`, NoteInvalidTextLength},
		{"long text", `{"type":"user","text":"` + strings.Repeat("é", 501) + `"}`, NoteInvalidTextLength},
		{"bad citation", `{"type":"user","text":"a","citations":[{"kind":"pr","repository_id":"r"}]}`, NoteInvalidCitation},
		{"abs path", `{"type":"user","text":"a","citations":[{"kind":"file","repository_id":"r","path":"/etc/x","sha":"` + strings.Repeat("a", 40) + `"}]}`, NoteInvalidCitation},
		{"duplicate", `{"type":"user","text":"a","citations":[{"kind":"pr","repository_id":"r","number":1},{"kind":"pr","repository_id":"r","number":1}]}`, NoteInvalidCitation},
		{"credential text", `{"type":"user","text":"kandev_pat_abc"}`, NoteCredential},
		{"unknown field", `{"type":"user","text":"a","x":1}`, NoteInvalidRequest},
		{"null type", `{"type":null,"text":"a"}`, NoteInvalidRequest},
		{"wrong type", `{"type":"user","text":5}`, NoteInvalidField},
		{"citations not list", `{"type":"user","text":"a","citations":"x"}`, NoteInvalidField},
		{"citation number string", `{"type":"user","text":"a","citations":[{"kind":"pr","repository_id":"r","number":"5"}]}`, NoteInvalidField},
		{"not an object", `[]`, NoteInvalidRequest},
	}
	for _, tc := range cases {
		_, err := f.mutate(noteOpCreate, "", tc.body)
		var ne *NoteError
		if !errors.As(err, &ne) || ne.Code != tc.code {
			t.Errorf("%s: err = %v, want %s", tc.name, err, tc.code)
		}
	}
	if notes, _ := f.store.ListNotes(context.Background(), f.c.ID, true); len(notes) != 0 {
		t.Fatalf("rejected requests saved %d notes", len(notes))
	}
}

func TestNoteCredentialErrorNamesFieldNotContent(t *testing.T) {
	f := newNotesFixture(t)
	_, err := f.mutate(noteOpCreate, "", `{"type":"user","text":"a","citations":[{"kind":"pr","repository_id":"kandev_pat_zzz","number":1}]}`)
	var ne *NoteError
	if !errors.As(err, &ne) || ne.Code != NoteCredential || ne.Field != "citations" || strings.Contains(ne.Error(), "zzz") {
		t.Fatalf("err = %v", err)
	}
}

func TestNoteTooLargeBody(t *testing.T) {
	f := newNotesFixture(t)
	_, err := f.svc.MutateNote(context.Background(), noteOpCreate, f.c.WorkspaceID, f.c.ID, "", nil, true)
	wantNoteErr(t, err, NoteInvalidRequest, "")
}

func TestNoteEditCreatesVersionAndNoOp(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	e, err := f.mutate(noteOpEdit, v.ID, `{"type":"project","text":"two","citations":null,"version":1}`)
	if err != nil || e.Version != 2 || e.Text != "two" {
		t.Fatalf("edit = %+v %v", e, err)
	}
	same, err := f.mutate(noteOpEdit, v.ID, `{"type":"project","text":" two ","citations":[],"version":99}`)
	if err != nil || same.Version != 2 {
		t.Fatalf("no-op with stale version = %+v %v", same, err)
	}
	_, err = f.mutate(noteOpEdit, v.ID, `{"type":"project","text":"three","citations":[],"version":1}`)
	wantNoteErr(t, err, NoteConflict, ConflictVersion)
	_, err = f.mutate(noteOpEdit, v.ID, `{"type":"project","text":"three","version":2}`)
	wantNoteErr(t, err, NoteInvalidRequest, "")
	_, err = f.mutate(noteOpEdit, v.ID, `{"type":"project","text":"three","citations":[],"version":"2"}`)
	wantNoteErr(t, err, NoteInvalidField, "")
	versions, _ := f.store.ListNoteVersions(context.Background(), v.ID)
	if len(versions) != 2 {
		t.Fatalf("versions = %d", len(versions))
	}
}

func TestNoteRevert(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	_, _ = f.mutate(noteOpEdit, v.ID, `{"type":"project","text":"two","citations":[],"version":1}`)
	r, err := f.mutate(noteOpRevert, v.ID, `{"target_version":1,"version":2}`)
	if err != nil || r.Version != 3 || r.Text != "one" {
		t.Fatalf("revert = %+v %v", r, err)
	}
	noop, err := f.mutate(noteOpRevert, v.ID, `{"target_version":3,"version":1}`)
	if err != nil || noop.Version != 3 {
		t.Fatalf("revert to current = %+v %v", noop, err)
	}
	_, err = f.mutate(noteOpRevert, v.ID, `{"target_version":9,"version":3}`)
	wantNoteErr(t, err, NoteInvalidRequest, "")
	_, err = f.mutate(noteOpRevert, v.ID, `{"target_version":0,"version":3}`)
	wantNoteErr(t, err, NoteInvalidField, "")
	versions, _ := f.store.ListNoteVersions(context.Background(), v.ID)
	if versions[2].RevertedFrom != 1 {
		t.Fatalf("revert version = %+v", versions[2])
	}
}

func TestNoteDeleteIsIdempotentTombstone(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	_, err := f.mutate(noteOpDelete, v.ID, `{"version":7}`)
	wantNoteErr(t, err, NoteConflict, ConflictVersion)
	if _, err := f.mutate(noteOpDelete, v.ID, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	if again, err := f.mutate(noteOpDelete, v.ID, `{"version":42}`); err != nil || again.Status != NoteStatusDeleted {
		t.Fatalf("repeat = %+v %v", again, err)
	}
	_, err = f.mutate(noteOpDelete, v.ID, `{}`)
	wantNoteErr(t, err, NoteInvalidRequest, "")
	_, err = f.mutate(noteOpDelete, v.ID, `{"version":1,"x":1}`)
	wantNoteErr(t, err, NoteInvalidRequest, "")
	_, err = f.mutate(noteOpEdit, v.ID, `{"type":"user","text":"z","citations":[],"version":1}`)
	wantNoteErr(t, err, NoteConflict, ConflictDeleted)
	versions, _ := f.store.ListNoteVersions(context.Background(), v.ID)
	if len(versions) != 1 {
		t.Fatalf("delete created a version: %d", len(versions))
	}
	list, _ := f.svc.ListNotes(context.Background(), f.c.WorkspaceID, f.c.ID, false)
	if len(list) != 0 {
		t.Fatalf("default list = %d", len(list))
	}
	all, _ := f.svc.ListNotes(context.Background(), f.c.WorkspaceID, f.c.ID, true)
	if len(all) != 1 {
		t.Fatalf("include deleted = %d", len(all))
	}
}

func TestNoteExpiredCannotEditButCanDelete(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	f.now = f.now.Add(noteTTL + time.Hour)
	_, err := f.mutate(noteOpEdit, v.ID, `{"type":"user","text":"z","citations":[],"version":1}`)
	wantNoteErr(t, err, NoteConflict, ConflictExpired)
	_, err = f.mutate(noteOpRevert, v.ID, `{"target_version":1,"version":1}`)
	wantNoteErr(t, err, NoteConflict, ConflictExpired)
	if _, err := f.mutate(noteOpDelete, v.ID, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
}

func TestNoteLimit(t *testing.T) {
	f := newNotesFixture(t)
	for i := 0; i < maxLiveNotes; i++ {
		f.create(fmt.Sprintf("n%d", i))
	}
	_, err := f.mutate(noteOpCreate, "", `{"type":"user","text":"extra"}`)
	wantNoteErr(t, err, NoteLimitReached, "")
}

func TestNoteCitationResolution(t *testing.T) {
	f := newNotesFixture(t)
	missing := NoteCitation{Kind: CitationTask, ID: "11111111-1111-1111-1111-111111111111"}
	f.check.states[missing.canonicalKey()] = CitationAbsent
	other := NoteCitation{Kind: CitationPR, RepositoryID: "r", Number: 2}
	f.check.states[other.canonicalKey()] = CitationUnavailable
	body := func(cs string) string { return `{"type":"user","text":"a","citations":` + cs + `}` }
	_, err := f.mutate(noteOpCreate, "", body(`[{"kind":"pr","repository_id":"r","number":2},{"kind":"task","id":"11111111-1111-1111-1111-111111111111"}]`))
	var ne *NoteError
	if !errors.As(err, &ne) || ne.Code != NoteCitationUnres || ne.Index == nil || *ne.Index != 1 {
		t.Fatalf("absent wins: %v", err)
	}
	_, err = f.mutate(noteOpCreate, "", body(`[{"kind":"pr","repository_id":"r","number":2}]`))
	wantNoteErr(t, err, NoteUnavailable, "")
	if notes, _ := f.store.ListNotes(context.Background(), f.c.ID, true); len(notes) != 0 {
		t.Fatal("rejected create saved a note")
	}
	f.svc.noteResolver = nil
	_, err = f.mutate(noteOpCreate, "", body(`[{"kind":"pr","repository_id":"r","number":3}]`))
	wantNoteErr(t, err, NoteUnavailable, "")
}

func TestNoteRevertSkipsResolution(t *testing.T) {
	f := newNotesFixture(t)
	v, err := f.mutate(noteOpCreate, "", `{"type":"user","text":"a","citations":[{"kind":"pr","repository_id":"r","number":2}]}`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.mutate(noteOpEdit, v.ID, `{"type":"user","text":"b","citations":[],"version":1}`)
	calls := f.check.calls
	if _, err := f.mutate(noteOpRevert, v.ID, `{"target_version":1,"version":2}`); err != nil {
		t.Fatal(err)
	}
	if f.check.calls != calls {
		t.Fatal("revert resolved citations")
	}
}

func TestNoteConcurrentEditsCommitOne(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.mutate(noteOpEdit, v.ID, fmt.Sprintf(`{"type":"user","text":"edit %d","citations":[],"version":1}`, i))
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("errs = %v", errs)
	}
}

func TestNoteDeletedDuringResolutionConflicts(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	f.svc.noteResolver = hookResolver{open: func() {
		if _, err := f.mutate(noteOpDelete, v.ID, `{"version":1}`); err != nil {
			t.Error(err)
		}
	}}
	_, err := f.mutate(noteOpEdit, v.ID, `{"type":"user","text":"z","citations":[{"kind":"pr","repository_id":"r","number":2}],"version":1}`)
	wantNoteErr(t, err, NoteConflict, ConflictDeleted)
}

type hookResolver struct{ open func() }

func (h hookResolver) Open(context.Context, string, string) CitationChecker {
	h.open()
	return hookChecker{}
}

type hookChecker struct{}

func (hookChecker) Check(context.Context, NoteCitation) CitationState { return CitationPresent }

func TestApprovedItemNoteSeam(t *testing.T) {
	f := newNotesFixture(t)
	item := ApprovedNoteItem{ItemID: "item-1", Model: "model-x", Type: NoteTypeFeedback, Text: "prefers short PRs"}
	v, err := f.svc.CreateApprovedItemNote(context.Background(), f.c.ID, item)
	if err != nil || v.Provenance.Kind != ProvenanceModel || v.Provenance.Model != "model-x" {
		t.Fatalf("seam = %+v %v", v, err)
	}
	again, err := f.svc.CreateApprovedItemNote(context.Background(), f.c.ID, item)
	if err != nil || again.ID != v.ID {
		t.Fatalf("repeat = %+v %v", again, err)
	}
	item.Text = "different"
	_, err = f.svc.CreateApprovedItemNote(context.Background(), f.c.ID, item)
	wantNoteErr(t, err, NoteConflict, ConflictVersion)
	item.ItemID, item.Text = "item-2", "kandev_pat_x"
	_, err = f.svc.CreateApprovedItemNote(context.Background(), f.c.ID, item)
	wantNoteErr(t, err, NoteCredential, "")
	if notes, _ := f.store.ListNotes(context.Background(), f.c.ID, true); len(notes) != 1 {
		t.Fatalf("notes = %d", len(notes))
	}
}
