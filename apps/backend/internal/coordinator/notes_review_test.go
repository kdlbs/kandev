package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNoteBaseStatusPrecedence(t *testing.T) {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	gone := at.Add(-time.Minute)
	cases := []struct {
		name string
		n    Note
		want NoteStatus
	}{
		{"deleted beats everything", Note{DeletedAt: &gone, ExpiresAt: at.Add(-time.Hour), LastResult: validationStale}, NoteStatusDeleted},
		{"expired beats stale", Note{ExpiresAt: at.Add(-time.Hour), LastResult: validationStale}, NoteStatusExpired},
		{"stale beats unavailable", Note{ExpiresAt: at.Add(time.Hour), LastResult: validationStale, LastAttempt: validationUnavailable}, NoteStatusStale},
		{"unavailable beats live", Note{ExpiresAt: at.Add(time.Hour), LastResult: validationValid, LastAttempt: validationUnavailable}, NoteStatusUnavailable},
		{"never validated", Note{ExpiresAt: at.Add(time.Hour)}, NoteStatusNotValidated},
		{"live", Note{ExpiresAt: at.Add(time.Hour), LastResult: validationValid, LastAttempt: validationValid}, NoteStatusLive},
		{"expiry equal to now is expired", Note{ExpiresAt: at, LastResult: validationValid}, NoteStatusExpired},
	}
	for _, tc := range cases {
		if got := tc.n.baseStatus(at); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestOverCapIDsRanksPastTheIndexInOrder(t *testing.T) {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	var notes []*Note
	for i := 0; i < noteIndexLines+2; i++ {
		notes = append(notes, &Note{ID: fmt.Sprintf("n%02d", i), ExpiresAt: at.Add(time.Hour), LastResult: validationValid})
	}
	stale := &Note{ID: "stale", ExpiresAt: at.Add(time.Hour), LastResult: validationStale}
	notes = append([]*Note{stale}, notes...)
	over := overCapIDs(notes, at)
	if len(over) != 2 || !over[fmt.Sprintf("n%02d", noteIndexLines)] || !over[fmt.Sprintf("n%02d", noteIndexLines+1)] {
		t.Fatalf("over cap = %v, want only the last two live notes", over)
	}
	if over["stale"] {
		t.Fatal("a stale note takes no index slot")
	}
}

func TestFoldCitationStatesIgnoresOrder(t *testing.T) {
	P, A, U := CitationPresent, CitationAbsent, CitationUnavailable
	cases := []struct {
		name   string
		states []CitationState
		result string
		absent int
	}{
		{"none", nil, validationValid, -1},
		{"all present", []CitationState{P, P}, validationValid, -1},
		{"unavailable then absent", []CitationState{U, A}, validationStale, 1},
		{"absent then unavailable", []CitationState{A, U}, validationStale, 0},
		{"two absent report the lowest", []CitationState{P, A, A}, validationStale, 1},
		{"unavailable alone", []CitationState{P, U}, validationUnavailable, -1},
	}
	for _, tc := range cases {
		result, idx := foldCitationStates(tc.states)
		if result != tc.result || idx != tc.absent {
			t.Errorf("%s: got (%s,%d), want (%s,%d)", tc.name, result, idx, tc.result, tc.absent)
		}
	}
}

type blockingChecker struct{}

func (blockingChecker) Check(ctx context.Context, _ NoteCitation) CitationState {
	<-ctx.Done()
	return CitationPresent
}

func TestRunCitationJobsDeadlineMakesUnfinishedUnavailable(t *testing.T) {
	cites := []NoteCitation{{Kind: CitationTask, ID: taskCiteID}, {Kind: CitationPR, RepositoryID: "r", Number: 1}}
	got := runCitationJobs(context.Background(), blockingChecker{}, cites, 20*time.Millisecond)
	for i, st := range got {
		if st != CitationUnavailable {
			t.Fatalf("citation %d = %v, want unavailable after the deadline", i, st)
		}
	}
}

func TestRecordNoteValidationOnlyStrictlyLaterAttemptWins(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("cas")
	ctx := context.Background()
	t0 := f.now.Add(time.Hour)
	ok, err := f.store.RecordNoteValidation(ctx, f.c.ID, v.ID, 1, validationValid, t0)
	if err != nil || !ok {
		t.Fatalf("first attempt: %v %v", ok, err)
	}
	for name, at := range map[string]time.Time{"equal": t0, "earlier": t0.Add(-time.Minute)} {
		ok, err := f.store.RecordNoteValidation(ctx, f.c.ID, v.ID, 1, validationStale, at)
		if err != nil || ok {
			t.Fatalf("%s attempt must lose: %v %v", name, ok, err)
		}
	}
	if n := f.note(v.ID); n.LastResult != validationValid {
		t.Fatalf("result overwritten by a losing attempt: %+v", n)
	}
	ok, _ = f.store.RecordNoteValidation(ctx, f.c.ID, v.ID, 1, validationUnavailable, t0.Add(time.Minute))
	n := f.note(v.ID)
	if !ok || n.LastResult != validationValid || n.LastAttempt != validationUnavailable {
		t.Fatalf("unavailable must not overwrite the last successful result: %+v", n)
	}
	if ok, _ := f.store.RecordNoteValidation(ctx, f.c.ID, v.ID, 7, validationStale, t0.Add(time.Hour)); ok {
		t.Fatal("an attempt for another version must not persist")
	}
}

func TestNoteFlagOffThenOnKeepsData(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("kept")
	before := f.note(v.ID)
	f.svc.notes = false
	if _, err := f.svc.ListNotes(context.Background(), f.c.WorkspaceID, f.c.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("flag off must hide the routes: %v", err)
	}
	if _, err := f.mutate(noteOpCreate, "", `{"type":"user","text":"x"}`); !errors.Is(err, ErrNotFound) {
		t.Fatalf("flag off must refuse writes: %v", err)
	}
	f.svc.notes = true
	after := f.note(v.ID)
	if after.Current.Text != "kept" || !after.ExpiresAt.Equal(before.ExpiresAt) || after.UseCount != before.UseCount {
		t.Fatalf("data changed across the flag toggle: %+v vs %+v", before, after)
	}
}

func TestNoteConcurrentCreatesAtLimitAdmitExactlyOne(t *testing.T) {
	f := newNotesFixture(t)
	for i := 0; i < maxLiveNotes-1; i++ {
		f.create(fmt.Sprintf("n%d", i))
	}
	body := `{"type":"user","text":"racer","citations":[{"kind":"task","id":"` + taskCiteID + `"}]}`
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.mutate(noteOpCreate, "", body)
		}()
	}
	wg.Wait()
	var ok, limited int
	for _, err := range errs {
		var ne *NoteError
		switch {
		case err == nil:
			ok++
		case errors.As(err, &ne) && ne.Code == NoteLimitReached:
			limited++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || limited != 1 {
		t.Fatalf("ok=%d limited=%d, want exactly one of each", ok, limited)
	}
	if notes, _ := f.store.ListNotes(context.Background(), f.c.ID, false); len(notes) != maxLiveNotes {
		t.Fatalf("live notes = %d, want %d", len(notes), maxLiveNotes)
	}
}
