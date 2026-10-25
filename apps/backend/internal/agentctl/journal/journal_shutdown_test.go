package journal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestJournalShutdownLifecycle(t *testing.T) {
	path := t.TempDir() + "/delivery.bbolt"
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := j.Append(ctx, Event{StreamID: "stream", SessionID: "session", Type: "message", Payload: []byte(`{"text":"committed"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatalf("repeated Close() error = %v", err)
	}

	event := Event{
		StreamID: "stream", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, Type: "complete", Payload: []byte(`{}`), Terminal: true,
	}
	submission := Submission{ID: "submission", Hash: "hash", SessionID: "session", State: SubmissionPrepared}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "append", run: func() error { _, err := j.Append(ctx, event); return err }},
		{name: "append batch", run: func() error { _, err := j.AppendBatch(ctx, []Event{event}); return err }},
		{name: "replay", run: func() error { _, _, err := j.Replay(ctx, "stream", 0, 10); return err }},
		{name: "acknowledge", run: func() error { return j.Acknowledge(ctx, "stream", 1) }},
		{name: "get stream", run: func() error { _, err := j.GetStream(ctx, "stream"); return err }},
		{name: "compact", run: func() error { return j.Compact(ctx) }},
		{name: "compact if needed", run: func() error { return j.CompactIfNeeded(ctx) }},
		{name: "unresolved submissions", run: func() error { _, err := j.HasUnresolvedSubmissions(ctx); return err }},
		{name: "list submissions", run: func() error { _, err := j.ListSubmissions(ctx, "session"); return err }},
		{name: "retire submission", run: func() error { _, err := j.RetireSubmission(ctx, "submission", 2); return err }},
		{name: "put submission", run: func() error { _, err := j.PutSubmission(ctx, submission); return err }},
		{name: "get submission", run: func() error { _, err := j.GetSubmission(ctx, "submission"); return err }},
		{name: "unresolved work", run: func() error { _, err := j.HasUnresolvedWork(ctx); return err }},
		{name: "transition submission", run: func() error {
			_, err := j.TransitionSubmission(ctx, "submission", SubmissionAccepted, time.Now())
			return err
		}},
		{name: "recovery descriptor", run: func() error { _, err := j.RecoveryDescriptor(ctx, "session", "incarnation", 1, "stream"); return err }},
		{name: "rollover stream", run: func() error { return j.RolloverStream(ctx, "stream", Stream{StreamID: "replacement"}) }},
		{name: "cancel submission", run: func() error { return j.CancelSubmission(ctx, "submission", event) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrJournalClosed) {
				t.Fatalf("operation error = %v, want %v", err, ErrJournalClosed)
			}
		})
	}

	reopened, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	events, _, err := reopened.Replay(ctx, "stream", 0, 10)
	if err != nil || len(events) != 1 || string(events[0].Payload) != `{"text":"committed"}` {
		t.Fatalf("reopened Replay() = (%+v, %v), want the committed event", events, err)
	}
}

func TestJournalCloseDuringReplay(t *testing.T) {
	j, err := Open(Config{Path: t.TempDir() + "/delivery.bbolt"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if _, err := j.Append(context.Background(), Event{
		StreamID: "stream", SessionID: "session", Type: "message", Payload: []byte(`{"text":"committed"}`),
	}); err != nil {
		t.Fatal(err)
	}

	replayCtx := &replayBarrierContext{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	type replayResult struct {
		events []Event
		err    error
	}
	replayed := make(chan replayResult, 1)
	go func() {
		events, _, err := j.Replay(replayCtx, "stream", 0, 10)
		replayed <- replayResult{events: events, err: err}
	}()
	<-replayCtx.entered // Replay is inside its bbolt read transaction and owns the lifetime read lock.

	closeStarted := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		close(closeStarted)
		closed <- j.Close()
	}()
	<-closeStarted
	close(replayCtx.release)

	result := <-replayed
	if result.err != nil || len(result.events) != 1 || string(result.events[0].Payload) != `{"text":"committed"}` {
		t.Fatalf("Replay() = (%+v, %v), want the committed event", result.events, result.err)
	}
	if err := <-closed; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, _, err := j.Replay(context.Background(), "stream", 0, 10); !errors.Is(err, ErrJournalClosed) {
		t.Fatalf("Replay() after close error = %v, want %v", err, ErrJournalClosed)
	}
}

type replayBarrierContext struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *replayBarrierContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *replayBarrierContext) Done() <-chan struct{}       { return nil }
func (c *replayBarrierContext) Value(any) any               { return nil }
func (c *replayBarrierContext) Err() error {
	c.once.Do(func() {
		close(c.entered)
		<-c.release
	})
	return nil
}
