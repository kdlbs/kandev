package automation

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
)

// fileBackedExportServiceFixture builds a Service backed by real writer and
// reader connection pools on a temp-file SQLite database, using the same
// DSNs internal/db.OpenSQLite/OpenSQLiteReader use in production (WAL,
// shared cache). AC-13 and AC-30 are claims about genuine concurrent
// transactions on separate connections; the single shared :memory: handle
// exportServiceTestFixture uses cannot exercise that.
func fileBackedExportServiceFixture(t *testing.T) (*Service, *fakeExportWorkspaceLookup) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "automation.db")

	writerRaw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })

	readerRaw, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	t.Cleanup(func() { _ = reader.Close() })

	store, err := NewStore(writer, reader)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, bus.NewMemoryEventBus(log), log)

	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{}}
	svc.SetExportWorkspaceLookup(wsLookup)
	return svc, wsLookup
}

// AC-9: exporting the same workspace twice with no intervening database
// change produces byte-identical output, for both the document and zip
// forms.

func TestExportAutomationsDocument_AC9_RepeatedExportIsByteIdentical(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Daily Sync", Prompt: "Hello"})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Weekly Digest"})

	first, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	second, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("repeated export was not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestExportAutomationsZip_AC9_RepeatedExportIsByteIdentical(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Daily Sync"})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Weekly Digest"})

	first, err := svc.ExportAutomationsZip(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	second, err := svc.ExportAutomationsZip(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("repeated zip export was not byte-identical")
	}
}

// AC-13: two exports of the same workspace running concurrently, with no
// mutation committing between the moment each one's read transaction
// establishes its snapshot, produce byte-identical output to each other.
// Run on the file-backed fixture so the two exports genuinely execute on
// separate connections rather than serializing on one shared handle.

func TestExportAutomationsDocument_AC13_ConcurrentExportsNoMutation_ByteIdentical(t *testing.T) {
	svc, wsLookup := fileBackedExportServiceFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Daily Sync", Prompt: "Hello"})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Weekly Digest"})

	const runs = 4
	results := make([][]byte, runs)
	errs := make([]error, runs)
	var wg sync.WaitGroup
	wg.Add(runs)
	for i := 0; i < runs; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = svc.ExportAutomationsDocument(context.Background(), "ws-1")
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent export %d: %v", i, err)
		}
	}
	for i := 1; i < runs; i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Errorf("concurrent export %d differs from export 0:\n--- 0 ---\n%s\n--- %d ---\n%s", i, results[0], i, results[i])
		}
	}
}

// AC-30: while an export is in flight, a concurrent mutation of the same
// workspace's automations neither blocks nor is blocked by it, and the
// export's already-open read transaction continues to observe the snapshot
// from its start rather than the concurrent write. Mirrors the spec's own
// probe: open a read transaction, first-read to establish the snapshot,
// write on the writer pool, and check both the write's latency and what the
// still-open transaction sees.

func TestExportAutomationsDocument_AC30_ConcurrentMutationNeitherBlocksNorIsReflected(t *testing.T) {
	svc, wsLookup := fileBackedExportServiceFixture(t)
	wsLookup.exists["ws-1"] = true
	ctx := context.Background()
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Existing"})

	tx, err := svc.store.BeginReadTx(ctx)
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := svc.store.ListAutomationsForExportTx(ctx, tx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx (before): %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("got %d automations before the concurrent write, want 1", len(before))
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Added During Export"})
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("concurrent CreateAutomation failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent CreateAutomation blocked on the in-flight export's read transaction")
	}

	after, err := svc.store.ListAutomationsForExportTx(ctx, tx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx (after): %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("the open read transaction observed the concurrent write; snapshot isolation broken: got %d automations, want 1", len(after))
	}

	body, err := svc.ExportAutomationsDocument(ctx, "ws-1")
	if err != nil {
		t.Fatalf("fresh export: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(doc.Automations) != 2 {
		t.Errorf("a fresh export (new transaction) should observe the committed write: got %d automations, want 2", len(doc.Automations))
	}
}
