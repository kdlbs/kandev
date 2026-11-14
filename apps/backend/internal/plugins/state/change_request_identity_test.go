package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// operatingLineChangeRequestDDL is the table exactly as an existing
// installation created it: host-qualified unique key, DATETIME timestamps,
// nullable terminal instants. It is spelled out here, not taken from the
// store, so a drift in the store's DDL cannot hide an incompatibility.
const operatingLineChangeRequestDDL = `
CREATE TABLE IF NOT EXISTS plugin_task_change_requests (
 id TEXT PRIMARY KEY, installation_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 task_id TEXT NOT NULL, provider_id TEXT NOT NULL, provider_host TEXT NOT NULL DEFAULT '',
 repository_id TEXT NOT NULL, number BIGINT NOT NULL, url TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT 'open',
 head_branch TEXT NOT NULL DEFAULT '', base_branch TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL, merged_at DATETIME, closed_at DATETIME, updated_at DATETIME NOT NULL,
 UNIQUE (installation_id, workspace_id, provider_id, provider_host, repository_id, number)
);
CREATE INDEX IF NOT EXISTS idx_plugin_task_change_requests_task ON plugin_task_change_requests(installation_id, workspace_id, task_id);
CREATE INDEX IF NOT EXISTS idx_plugin_task_change_requests_merged ON plugin_task_change_requests(workspace_id, state, merged_at);
`

// rebuiltLegacyChangeRequestDDL is the same table after an older pre-host
// table was rebuilt in place: timestamps keep TEXT affinity and unset terminal
// instants may hold empty text.
const rebuiltLegacyChangeRequestDDL = `
CREATE TABLE plugin_task_change_requests (
 id TEXT PRIMARY KEY, installation_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 task_id TEXT NOT NULL, provider_id TEXT NOT NULL, provider_host TEXT NOT NULL DEFAULT '',
 repository_id TEXT NOT NULL, number BIGINT NOT NULL, url TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT 'open',
 head_branch TEXT NOT NULL DEFAULT '', base_branch TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, merged_at TEXT, closed_at TEXT, updated_at TEXT NOT NULL,
 UNIQUE (installation_id, workspace_id, provider_id, provider_host, repository_id, number)
);
CREATE INDEX IF NOT EXISTS idx_plugin_task_change_requests_task ON plugin_task_change_requests(installation_id, workspace_id, task_id);
`

func openChangeRequestStoreOnExistingTable(t *testing.T, ddl string) (*ChangeRequestStore, *sqlx.DB) {
	t.Helper()
	raw, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })
	if _, err := raw.Exec(ddl); err != nil {
		t.Fatalf("create existing table: %v", err)
	}
	store := &ChangeRequestStore{db: raw, ro: raw}
	if err := store.initSchema(); err != nil {
		t.Fatalf("init change request schema on existing table: %v", err)
	}
	return store, raw
}

func TestChangeRequestStoreOpensOperatingLineTable(t *testing.T) {
	// unsetTerminal is how each shape stores an unset terminal instant: NULL in
	// DATETIME columns, and possibly empty text in the rebuilt TEXT columns.
	for name, shape := range map[string]struct{ ddl, unsetTerminal string }{
		"datetime":       {operatingLineChangeRequestDDL, "NULL"},
		"rebuilt legacy": {rebuiltLegacyChangeRequestDDL, "''"},
	} {
		ddl, unsetTerminal := shape.ddl, shape.unsetTerminal
		t.Run(name, func(t *testing.T) {
			store, raw := openChangeRequestStoreOnExistingTable(t, ddl)
			ctx := context.Background()
			// Rows written before this store existed: one with a driver-formatted
			// time and NULL terminals, one with RFC3339 text and the shape's unset
			// terminal value.
			if _, err := raw.Exec(`INSERT INTO plugin_task_change_requests
				(id, installation_id, workspace_id, task_id, provider_id, provider_host, repository_id, number,
				 state, created_at, merged_at, closed_at, updated_at)
				VALUES ('row-old-1', 'inst-1', 'ws-1', 'task-1', 'forgejo', 'forge.example.test', 'repo-1', 1,
				 'open', ?, ?, ?, ?)`,
				time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), nil, nil, time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)); err != nil {
				t.Fatalf("seed driver-time row: %v", err)
			}
			if _, err := raw.Exec(`INSERT INTO plugin_task_change_requests
				(id, installation_id, workspace_id, task_id, provider_id, provider_host, repository_id, number,
				 state, created_at, merged_at, closed_at, updated_at)
				VALUES ('row-old-2', 'inst-1', 'ws-1', 'task-1', 'forgejo', 'forge.example.test', 'repo-1', 2,
				 'open', '2026-10-02T09:00:00Z', ` + unsetTerminal + `, ` + unsetTerminal + `, '2026-10-02T09:30:00Z')`); err != nil {
				t.Fatalf("seed text row: %v", err)
			}
			rows, err := store.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
			if err != nil || len(rows) != 2 {
				t.Fatalf("list existing rows = %+v err:%v", rows, err)
			}
			for _, row := range rows {
				if row.MergedAt != "" || row.ClosedAt != "" || row.CreatedAt == "" || row.UpdatedAt == "" {
					t.Fatalf("existing row timestamps = %+v", row)
				}
			}

			// A host-less report updates the existing row and keeps its host.
			mergedAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
			merged, existed, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
				InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
				ProviderID: "forgejo", RepositoryID: "repo-1", Number: 2,
				State: ChangeRequestStateMerged, MergedAt: mergedAt,
			})
			if err != nil || !existed || merged.ID != "row-old-2" || merged.ProviderHost != "forge.example.test" ||
				merged.State != ChangeRequestStateMerged || merged.MergedAt == "" || merged.ClosedAt != "" {
				t.Fatalf("report on existing text row = %+v existed:%v err:%v", merged, existed, err)
			}
			if merged.CreatedAt != "2026-10-02T09:00:00.000000000Z" {
				t.Fatalf("created_at = %q, want the stored instant latched", merged.CreatedAt)
			}
			// A stale open poll cannot unmerge it.
			stale, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
				InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
				ProviderID: "forgejo", ProviderHost: "forge.example.test", RepositoryID: "repo-1", Number: 2,
				State: ChangeRequestStateOpen,
			})
			if err != nil || stale.State != ChangeRequestStateMerged || stale.MergedAt != merged.MergedAt {
				t.Fatalf("stale report = %+v err:%v, want merged latched", stale, err)
			}

			// A new row lands with NULL terminal instants.
			fresh, existed, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
				InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
				ProviderID: "forgejo", ProviderHost: "forge.example.test", RepositoryID: "repo-1", Number: 3,
				State: ChangeRequestStateOpen,
			})
			if err != nil || existed || fresh.MergedAt != "" {
				t.Fatalf("report new row = %+v existed:%v err:%v", fresh, existed, err)
			}
			var nullMerged int
			if err := raw.Get(&nullMerged, `SELECT COUNT(*) FROM plugin_task_change_requests WHERE id = ? AND merged_at IS NULL AND closed_at IS NULL`, fresh.ID); err != nil || nullMerged != 1 {
				t.Fatalf("new row terminal instants stored as NULL = %d err:%v", nullMerged, err)
			}

			for _, number := range []int64{1, 2, 3} {
				if err := store.RemoveChangeRequest(ctx, "inst-1", "ws-1", "forgejo", "repo-1", number); err != nil {
					t.Fatalf("remove %d: %v", number, err)
				}
			}
			rows, err = store.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
			if err != nil || len(rows) != 0 {
				t.Fatalf("rows after remove = %+v err:%v", rows, err)
			}
		})
	}
}

func TestChangeRequestStoreKeepsProviderHostsDistinct(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	report := func(host, title string) ChangeRequestRecord {
		t.Helper()
		stored, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
			InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
			ProviderID: "forgejo", ProviderHost: host, RepositoryID: "repo-1", Number: 5,
			Title: title, State: ChangeRequestStateOpen,
		})
		if err != nil {
			t.Fatalf("report %s: %v", host, err)
		}
		return stored
	}
	first := report("forge-a.example.test", "a")
	second := report("forge-b.example.test", "b")
	if first.ID == second.ID {
		t.Fatal("two provider hosts must be two rows")
	}
	again := report("forge-a.example.test", "a2")
	if again.ID != first.ID || again.Title != "a2" {
		t.Fatalf("host-qualified re-report = %+v, want row %s updated", again, first.ID)
	}

	// A host-less identity now names two objects: report and remove refuse to
	// guess, and neither row changes.
	if _, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 5, Title: "x", State: ChangeRequestStateOpen,
	}); !errors.Is(err, ErrChangeRequestAmbiguous) {
		t.Fatalf("host-less report err = %v, want ErrChangeRequestAmbiguous", err)
	}
	if err := store.RemoveChangeRequest(ctx, "inst-1", "ws-1", "forgejo", "repo-1", 5); !errors.Is(err, ErrChangeRequestAmbiguous) {
		t.Fatalf("host-less remove err = %v, want ErrChangeRequestAmbiguous", err)
	}
	rows, err := store.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows after ambiguous calls = %+v err:%v, want both kept", rows, err)
	}
	for _, row := range rows {
		if row.Title == "x" {
			t.Fatalf("ambiguous report changed row %+v", row)
		}
	}
}

func TestRemoveWithReceiptRejectsAmbiguousIdentity(t *testing.T) {
	ledger := newChangeRequestTestStore(t)
	commands := &CommandStore{db: ledger.db, ro: ledger.ro}
	if err := commands.initSchema(); err != nil {
		t.Fatalf("init command schema: %v", err)
	}
	ctx := context.Background()
	for _, host := range []string{"forge-a.example.test", "forge-b.example.test"} {
		if _, _, err := ledger.ReportChangeRequest(ctx, ChangeRequestRecord{
			InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
			ProviderID: "forgejo", ProviderHost: host, RepositoryID: "repo-1", Number: 9, State: ChangeRequestStateOpen,
		}); err != nil {
			t.Fatalf("report %s: %v", host, err)
		}
	}
	admitted, _, err := commands.Admit(ctx, CommandIntent{
		InstallationID: "inst-1", WorkspaceID: "ws-1", RequestID: "remove-request", IdempotencyKey: "remove-key",
		Method: "RemoveTaskChangeRequestExact", CapabilityID: "cap", PayloadDigest: "digest",
		TargetID: "task-1", ExpectedResourceVersion: "v", ApprovalRevision: 1, ManifestDigest: "digest",
	})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if _, err := ledger.RemoveChangeRequestWithReceipt(ctx, "inst-1", "ws-1", "forgejo", "repo-1", 9,
		commands, admitted.Intent.OperationID, "APPLIED"); !errors.Is(err, ErrChangeRequestAmbiguous) {
		t.Fatalf("remove with receipt err = %v, want ErrChangeRequestAmbiguous", err)
	}
	rows, err := ledger.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows after ambiguous remove = %+v err:%v, want both kept", rows, err)
	}
}
