package ledger

import (
	"testing"
	"time"
)

func TestStoppedLedgerDoesNotReadClosedRepositoryOnStart(t *testing.T) {
	f := newFixture(t)
	f.l.Start(t.Context())
	f.l.Stop()
	if err := f.l.deps.RO.Close(); err != nil {
		t.Fatal(err)
	}
	before := FailureCount(StageStart)
	f.l.Start(t.Context())
	f.l.Stop()
	if got := FailureCount(StageStart); got != before {
		t.Fatalf("stopped ledger accessed closed repository: start failures %d -> %d", before, got)
	}
}

func TestLedgerStopCancelsCallsWaitingForWriterConnection(t *testing.T) {
	f := newFixture(t)
	f.db.SetMaxOpenConns(1)
	connection, err := f.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	before := f.db.Stats().WaitCount
	f.l.Start(t.Context())
	f.l.Call(testSession, "coordinator.list_tasks", "", true)
	waitFor(t, func() bool { return f.db.Stats().WaitCount >= before+2 })
	stopped := make(chan struct{})
	go func() { f.l.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		_ = connection.Close()
		<-stopped
		t.Fatal("ledger shutdown waited for writer connection instead of cancelling owned I/O")
	}
	f.l.Stop()
}

func TestPostgresLedgerStopCancelsRebuildWaitingForReaderConnection(t *testing.T) {
	f := newPostgresFixture(t)
	f.db.SetMaxOpenConns(1)
	connection, err := f.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	before := f.db.Stats().WaitCount
	started := make(chan struct{})
	go func() { f.l.Start(t.Context()); close(started) }()
	waitFor(t, func() bool { return f.db.Stats().WaitCount > before })
	stopped := make(chan struct{})
	go func() { f.l.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		_ = connection.Close()
		<-started
		<-stopped
		t.Fatal("ledger shutdown waited for reader connection instead of cancelling rebuild")
	}
	<-started
	f.l.Stop()
}
