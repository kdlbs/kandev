package dream

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

func TestSchedulerStopCancelsFinalWritesWaitingForWriterConnection(t *testing.T) {
	for _, answer := range []string{`{"items":[],"considered":[]}`, "invalid output"} {
		t.Run(answer, func(t *testing.T) {
			f := newFixture(t)
			f.seedEvidence(6)
			f.ep.answer = answer
			f.ep.block = make(chan struct{})
			f.sch.Tick(context.Background(), f.coord.ID)
			row := f.waitRunning()
			f.db.SetMaxOpenConns(1)
			connection, err := f.db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = connection.Close() }()
			before := f.db.Stats().WaitCount
			close(f.ep.block)
			deadline := time.Now().Add(5 * time.Second)
			for f.db.Stats().WaitCount == before && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if f.db.Stats().WaitCount == before {
				t.Fatal("dream final write did not wait for the held writer connection")
			}
			stopped := make(chan struct{})
			go func() { f.sch.Stop(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				_ = connection.Close()
				<-stopped
				t.Fatal("scheduler shutdown waited for a writer connection instead of cancelling owned I/O")
			}
			f.sch.Stop()
			stored, err := f.store.GetDream(t.Context(), f.coord.ID, row.ID)
			if err != nil || stored.Status != coordinator.DreamRunning {
				t.Fatalf("cancelled final write changed the lease row: dream=%+v err=%v", stored, err)
			}
		})
	}
}
