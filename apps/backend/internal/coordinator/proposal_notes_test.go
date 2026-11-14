package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type proposalNotesEnv struct {
	*notesFixture
	turns *fakeActiveTurns
}

func newProposalNotesEnv(t *testing.T) *proposalNotesEnv {
	f := newNotesFixture(t)
	e := &proposalNotesEnv{notesFixture: f, turns: &fakeActiveTurns{turns: map[string]string{"sess": "turn-1"}}}
	f.svc.SetSpendDeps(nil, e.turns, nil)
	return e
}

// propose inserts a proposal of an arbitrary kind with the given raw note_ids.
func (e *proposalNotesEnv) propose(raw string) (*Proposal, error) {
	ctx := context.Background()
	if raw != "" {
		ctx = e.svc.WithProposalNotes(ctx, e.c.ID, "sess", json.RawMessage(raw))
	}
	p := &Proposal{CoordinatorID: e.c.ID, WorkspaceID: e.c.WorkspaceID, Kind: ProposalKindMessage, RawSpec: `{}`}
	return p, e.store.InsertProposalWith(ctx, p, true, nil, nil)
}

func (e *proposalNotesEnv) links(proposalID string) map[string]int {
	rows, err := e.store.db.Query(e.store.db.Rebind(`SELECT note_id, note_version FROM coordinator_proposal_notes WHERE proposal_id = ?`), proposalID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var v int
		_ = rows.Scan(&id, &v)
		out[id] = v
	}
	return out
}

func (e *proposalNotesEnv) proposalCount() int {
	var n int
	_ = e.store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_proposals`).Scan(&n)
	return n
}

func ids(vs ...string) string {
	b, _ := json.Marshal(vs)
	return string(b)
}

func wantReason(t *testing.T, err error, reason string, offending ...string) {
	t.Helper()
	var ne *ProposalNotesError
	if !errors.As(err, &ne) {
		t.Fatalf("want ProposalNotesError %q, got %v", reason, err)
	}
	if ne.Reason != reason {
		t.Fatalf("reason = %q, want %q (%v)", ne.Reason, reason, ne)
	}
	var got []string
	for _, o := range ne.Offences {
		got = append(got, o.ID+":"+o.Reason)
	}
	if strings.Join(got, ",") != strings.Join(offending, ",") {
		t.Fatalf("offences = %v, want %v", got, offending)
	}
}

func TestProposalNotesAcceptedLinksDeliveredVersion(t *testing.T) {
	e := newProposalNotesEnv(t)
	a, b := e.create("a"), e.create("b")
	e.prepare("turn-1")
	for _, raw := range []string{"", "null", "[]"} {
		if p, err := e.propose(raw); err != nil || len(e.links(p.ID)) != 0 {
			t.Fatalf("%q: absent/null/empty means none: %v", raw, err)
		}
	}
	// An edit after the snapshot keeps the delivered version in the link.
	if _, err := e.mutate(noteOpEdit, a.ID, `{"type":"project","text":"a2","citations":[],"version":1}`); err != nil {
		t.Fatal(err)
	}
	p, err := e.propose(ids(a.ID, strings.ToUpper(b.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if l := e.links(p.ID); len(l) != 2 || l[a.ID] != 1 || l[b.ID] != 1 {
		t.Fatalf("links: %v", l)
	}
}

func TestProposalNotesRequestLevelReasons(t *testing.T) {
	e := newProposalNotesEnv(t)
	n := e.create("a")
	e.prepare("turn-1")
	six := ids(n.ID, n.ID, n.ID, n.ID, n.ID, n.ID)
	_, err := e.propose(six)
	wantReason(t, err, NoteRefTooMany)
	_, err = e.propose(`"not a list"`)
	wantReason(t, err, NoteRefMalformed)
	e.svc.notes, e.store.notesOn = false, false
	_, err = e.propose(ids(n.ID))
	wantReason(t, err, NoteRefUnavailable)
	if e.proposalCount() != 0 {
		t.Fatal("refusals must create nothing")
	}
	// An empty list is fine with the flag off.
	if _, err := e.propose("[]"); err != nil {
		t.Fatal(err)
	}
}

func TestProposalNotesPerEntryReasonsAndOrdering(t *testing.T) {
	e := newProposalNotesEnv(t)
	ok := e.create("ok")
	gone := e.create("gone")
	late := e.create("late")
	stale := e.create("stale")
	e.prepare("turn-1")
	undelivered := e.create("later")
	if _, err := e.mutate(noteOpDelete, gone.ID, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	e.expire(late.ID)
	mustExecNote(t, e, `UPDATE coordinator_notes SET last_result = 'stale', last_attempt = 'stale' WHERE id = ?`, stale.ID)
	foreign := "99999999-9999-9999-9999-999999999999"
	cases := []struct {
		name   string
		raw    string
		reason string
		offs   []string
	}{
		{"malformed", ids("zzz"), NoteRefMalformed, []string{"zzz:" + NoteRefMalformed}},
		{"unknown", ids(foreign), NoteRefUnknown, []string{foreign + ":" + NoteRefUnknown}},
		{"deleted", ids(gone.ID), NoteRefDeleted, []string{gone.ID + ":" + NoteRefDeleted}},
		{"expired", ids(late.ID), NoteRefExpired, []string{late.ID + ":" + NoteRefExpired}},
		{"stale", ids(stale.ID), NoteRefStale, []string{stale.ID + ":" + NoteRefStale}},
		{"not delivered", ids(undelivered.ID), NoteRefNotDelivered, []string{undelivered.ID + ":" + NoteRefNotDelivered}},
		{"duplicate judges the later one", ids(ok.ID, strings.ToUpper(ok.ID)), NoteRefDuplicate, []string{ok.ID + ":" + NoteRefDuplicate}},
		{"first offender sets the reason, all are listed", ids(ok.ID, gone.ID, "zzz", stale.ID), NoteRefDeleted,
			[]string{gone.ID + ":" + NoteRefDeleted, "zzz:" + NoteRefMalformed, stale.ID + ":" + NoteRefStale}},
	}
	for _, tc := range cases {
		_, err := e.propose(tc.raw)
		wantReason(t, err, tc.reason, tc.offs...)
	}
	if e.proposalCount() != 0 {
		t.Fatal("refusals must create nothing")
	}
}

func mustExecNote(t *testing.T, e *proposalNotesEnv, q string, args ...any) {
	t.Helper()
	if _, err := e.store.db.Exec(e.store.db.Rebind(q), args...); err != nil {
		t.Fatal(err)
	}
}

func TestProposalNotesUnavailableAndNotRepromptedStatesAreReferenceable(t *testing.T) {
	e := newProposalNotesEnv(t)
	flaky := NoteCitation{Kind: CitationTask, ID: taskCiteID}
	n := e.createCited("flaky", flaky)
	fresh := e.create("fresh")
	e.prepare("turn-1")
	// Unavailable attempt and a never-validated note are not stale.
	mustExecNote(t, e, `UPDATE coordinator_notes SET last_attempt = 'unavailable' WHERE id = ?`, n.ID)
	if _, err := e.propose(ids(n.ID, fresh.ID)); err != nil {
		t.Fatalf("unavailable/not-validated must stay referenceable: %v", err)
	}
}

func TestProposalNotesNotDeliveredWithoutTurnOrSnapshot(t *testing.T) {
	e := newProposalNotesEnv(t)
	n := e.create("a")
	// No snapshot for the active turn.
	_, err := e.propose(ids(n.ID))
	wantReason(t, err, NoteRefNotDelivered, n.ID+":"+NoteRefNotDelivered)
	// No active turn at all.
	e.prepare("turn-1")
	e.turns.turns = map[string]string{}
	_, err = e.propose(ids(n.ID))
	wantReason(t, err, NoteRefNotDelivered, n.ID+":"+NoteRefNotDelivered)
	// A snapshot of another coordinator's turn grants nothing.
	e.turns.turns = map[string]string{"sess": "turn-1"}
	ctx := e.svc.WithProposalNotes(context.Background(), "other-coordinator", "sess", json.RawMessage(ids(n.ID)))
	p := &Proposal{CoordinatorID: e.c.ID, WorkspaceID: e.c.WorkspaceID, Kind: ProposalKindMessage, RawSpec: `{}`}
	wantReason(t, e.store.InsertProposalWith(ctx, p, true, nil, nil), NoteRefNotDelivered, n.ID+":"+NoteRefNotDelivered)
}

func TestProposalNotesDeleteAndProposeSerialize(t *testing.T) {
	e := newProposalNotesEnv(t)
	n := e.create("a")
	e.prepare("turn-1")
	if _, err := e.mutate(noteOpDelete, n.ID, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	_, err := e.propose(ids(n.ID))
	wantReason(t, err, NoteRefDeleted, n.ID+":"+NoteRefDeleted)
}

func TestProposalNotesConcurrentDeleteNeverLeavesLinkToDeleted(t *testing.T) {
	for i := 0; i < 10; i++ {
		e := newProposalNotesEnv(t)
		n := e.create("a")
		e.prepare("turn-1")
		done := make(chan error, 2)
		var p *Proposal
		go func() { var err error; p, err = e.propose(ids(n.ID)); done <- err }()
		go func() { _, err := e.mutate(noteOpDelete, n.ID, `{"version":1}`); done <- err }()
		var proposeErr error
		for j := 0; j < 2; j++ {
			if err := <-done; err != nil {
				proposeErr = err
			}
		}
		linked := p != nil && len(e.links(p.ID)) == 1
		if proposeErr != nil {
			wantReason(t, proposeErr, NoteRefDeleted, n.ID+":"+NoteRefDeleted)
			if e.proposalCount() != 0 {
				t.Fatal("refused proposal left a row")
			}
		} else if !linked {
			t.Fatal("accepted proposal must carry its link")
		}
	}
}

func TestProposalNotesReferrersAndHistory(t *testing.T) {
	e := newProposalNotesEnv(t)
	n := e.create("a")
	e.prepare("turn-1")
	open, err := e.propose(ids(n.ID))
	if err != nil {
		t.Fatal(err)
	}
	done, err := e.propose(ids(n.ID))
	if err != nil {
		t.Fatal(err)
	}
	mustExecNote(t, e, `UPDATE coordinator_proposals SET status = 'rejected' WHERE id = ?`, done.ID)
	refs := e.referrers(n.ID)
	if len(refs.Proposals) != 1 || refs.Proposals[0].ID != open.ID || refs.Proposals[0].Status != "pending" || len(refs.Tasks) != 0 {
		t.Fatalf("referrers: %+v", refs)
	}
	// Tasks created from approved proposals count only while open.
	approved, _ := e.propose(ids(n.ID))
	now := time.Now().UTC()
	for _, tk := range []struct{ id, state string }{{"task-open", "IN_PROGRESS"}, {"task-done", "COMPLETED"}, {"task-failed", "FAILED"}, {"task-cancelled", "CANCELLED"}} {
		mustExecNote(t, e, `INSERT INTO tasks (id, workspace_id, title, state, created_at, updated_at) VALUES (?, ?, 't', ?, ?, ?)`,
			tk.id, e.c.WorkspaceID, tk.state, now, now)
		p := &Proposal{CoordinatorID: e.c.ID, WorkspaceID: e.c.WorkspaceID, Kind: ProposalKindMessage, RawSpec: `{}`}
		if err := e.store.InsertProposalWith(e.svc.WithProposalNotes(context.Background(), e.c.ID, "sess", json.RawMessage(ids(n.ID))), p, true, nil, nil); err != nil {
			t.Fatal(err)
		}
		mustExecNote(t, e, `UPDATE coordinator_proposals SET status = 'approved', task_id = ? WHERE id = ?`, tk.id, p.ID)
	}
	mustExecNote(t, e, `UPDATE coordinator_proposals SET status = 'approved', task_id = 'task-open' WHERE id = ?`, approved.ID)
	refs = e.referrers(n.ID)
	if len(refs.Tasks) != 1 || refs.Tasks[0].ID != "task-open" {
		t.Fatalf("only the open task counts: %+v", refs.Tasks)
	}
	mustExecNote(t, e, `UPDATE tasks SET archived_at = ? WHERE id = 'task-open'`, now)
	if refs = e.referrers(n.ID); len(refs.Tasks) != 0 {
		t.Fatalf("archived task must drop: %+v", refs.Tasks)
	}
	hist, err := e.svc.NoteReferenceHistory(context.Background(), e.c.WorkspaceID, e.c.ID, n.ID)
	if err != nil || len(hist) != 7 {
		t.Fatalf("history keeps every link: %d %v", len(hist), err)
	}
	if _, err := e.mutate(noteOpDelete, n.ID, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	after, err := e.svc.NoteReferenceHistory(context.Background(), e.c.WorkspaceID, e.c.ID, n.ID)
	if err != nil || len(after) != 7 {
		t.Fatalf("history survives deletion: %d %v", len(after), err)
	}
	if _, err := e.svc.NoteReferenceHistory(context.Background(), e.c.WorkspaceID, e.c.ID, "11111111-1111-1111-1111-111111111112"); err == nil {
		t.Fatal("unknown note must be not found")
	}
}

func (e *proposalNotesEnv) referrers(noteID string) *NoteReferrers {
	e.t.Helper()
	d, err := e.svc.GetNote(context.Background(), e.c.WorkspaceID, e.c.ID, noteID)
	if err != nil {
		e.t.Fatal(err)
	}
	if d.ReferencedBy == nil {
		e.t.Fatal("referenced_by missing")
	}
	return d.ReferencedBy
}

func TestProposalNotesAreKindAgnostic(t *testing.T) {
	e := newProposalNotesEnv(t)
	n := e.create("a")
	e.prepare("turn-1")
	for _, kind := range []string{ProposalKindCreateTask, ProposalKindResume, ProposalKindAnswer, ProposalKindMerge, ProposalKindImprovement, "stub_future_kind"} {
		ctx := e.svc.WithProposalNotes(context.Background(), e.c.ID, "sess", json.RawMessage(ids(n.ID)))
		p := &Proposal{CoordinatorID: e.c.ID, WorkspaceID: e.c.WorkspaceID, Kind: kind, RawSpec: `{}`}
		if err := e.store.InsertProposalWith(ctx, p, true, nil, nil); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if l := e.links(p.ID); l[n.ID] != 1 {
			t.Fatalf("%s: link missing: %v", kind, l)
		}
	}
}
