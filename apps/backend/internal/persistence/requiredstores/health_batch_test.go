package requiredstores

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/startup"
)

// Observe executed queries at the driver boundary without changing Health.
type catalogConnector struct {
	driver  sqlite3.SQLiteDriver
	queries int
	fault   string
	cancel  context.CancelFunc
}

func (c *catalogConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(":memory:")
	if err != nil {
		return nil, err
	}
	return &catalogConnection{SQLiteConn: conn.(*sqlite3.SQLiteConn), observer: c}, nil
}

func (c *catalogConnector) Driver() driver.Driver { return &c.driver }

type catalogConnection struct {
	*sqlite3.SQLiteConn
	observer *catalogConnector
}

func (c *catalogConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.observer.queries++
	switch c.observer.fault {
	case "query":
		return nil, errors.New("injected catalog query failure")
	case "cancel":
		c.observer.cancel()
		return nil, ctx.Err()
	}
	rows, err := c.SQLiteConn.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return &catalogRows{Rows: rows, fault: c.observer.fault}, nil
}

// @covers AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1
func TestHealthSQLiteCatalogUsesOneSchemaQuery(t *testing.T) {
	observer := &catalogConnector{}
	writer := sqlx.NewDb(sql.OpenDB(observer), "sqlite3")
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	writer.SetMaxOpenConns(1)
	catalog := Catalog()
	tracker, err := NewTracker(catalog)
	require.NoError(t, err)
	tables := make(map[string]bool)
	for _, descriptor := range catalog {
		for _, table := range descriptor.RequiredTables {
			if !tables[table] {
				_, err := writer.Exec(fmt.Sprintf("CREATE TABLE %q (id TEXT)", table))
				require.NoError(t, err)
				tables[table] = true
			}
		}
		require.NoError(t, tracker.RecordSuccess(descriptor.ID))
	}
	health := NewHealth(tracker, db.NewPool(writer, writer), nil)
	require.NoError(t, health.Check(context.Background()))
	require.True(t, health.Healthy())
	require.Equal(t, 1, observer.queries, "one sweep must acquire the writer for one schema query")
}

// Faults occur after the real query has opened rows, so partial inventories
// exercise the same scan and iteration boundaries as SQLite.
type catalogRows struct {
	driver.Rows
	fault string
	read  bool
}

func (r *catalogRows) Next(dest []driver.Value) error {
	if r.fault == "iteration" && r.read {
		return errors.New("injected catalog iteration failure")
	}
	err := r.Rows.Next(dest)
	if err == nil && r.fault == "scan" {
		dest[0] = nil
	}
	r.read = true
	return err
}

func batchDescriptors() []Descriptor {
	return []Descriptor{
		{ID: "a", OwnerPackage: "owner/a", RequiredTables: []string{"shared"}, Sweep: startup.StepStoresRepositories},
		{ID: "b", OwnerPackage: "owner/b", RequiredTables: []string{"shared", "extra"}, Sweep: startup.StepStoresRepositories},
		{ID: "c", OwnerPackage: "owner/c", RequiredTables: []string{"independent"}, Sweep: startup.StepStoresRepositories},
	}
}

func seedBatchHealth(t *testing.T, pool *db.Pool) *Health {
	t.Helper()
	for _, table := range []string{"shared", "extra", "independent"} {
		_, err := pool.Writer().Exec(fmt.Sprintf("CREATE TABLE %q (id TEXT)", table))
		require.NoError(t, err)
	}
	tracker, err := NewTracker(batchDescriptors())
	require.NoError(t, err)
	for _, descriptor := range batchDescriptors() {
		require.NoError(t, tracker.RecordSuccess(descriptor.ID))
	}
	return NewHealth(tracker, pool, nil)
}

func TestHealthSQLiteCatalogIndependentMissingTables(t *testing.T) {
	for _, test := range []struct {
		table     string
		unhealthy []string
	}{
		{"shared", []string{"a", "b"}}, {"extra", []string{"b"}}, {"independent", []string{"c"}},
	} {
		t.Run(test.table, func(t *testing.T) {
			writer := openSingleConnectionSQLite(t)
			health := seedBatchHealth(t, db.NewPool(writer, writer))
			require.NoError(t, health.Check(context.Background()))
			_, err := writer.Exec(fmt.Sprintf("DROP TABLE %q", test.table))
			require.NoError(t, err)
			diagnostic, err := health.check(context.Background())
			require.Error(t, err)
			require.Equal(t, test.unhealthy, health.UnhealthyStoreIDs())
			require.Equal(t, "required_table_missing", diagnostic.errorClass)
			require.Equal(t, test.unhealthy[0], diagnostic.storeID)
			_, err = writer.Exec(fmt.Sprintf("CREATE TABLE %q (id TEXT)", test.table))
			require.NoError(t, err)
			require.NoError(t, health.Check(context.Background()))
			require.True(t, health.Healthy())
		})
	}
}

func TestHealthSQLiteCatalogFailureDiscardsInventory(t *testing.T) {
	for _, fault := range []string{"query", "scan", "iteration", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			observer := &catalogConnector{}
			writer := sqlx.NewDb(sql.OpenDB(observer), "sqlite3")
			t.Cleanup(func() { require.NoError(t, writer.Close()) })
			writer.SetMaxOpenConns(1)
			health := seedBatchHealth(t, db.NewPool(writer, writer))
			require.NoError(t, health.Check(context.Background()))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observer.fault, observer.cancel = fault, cancel
			diagnostic, err := health.check(ctx)
			require.Error(t, err)
			require.Equal(t, []string{"a", "b", "c"}, health.UnhealthyStoreIDs())
			require.Equal(t, "table_probe", diagnostic.stage)
			require.Empty(t, diagnostic.storeID, "an incomplete batch cannot attribute failure to one store")
			if fault == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Zero(t, writer.Stats().InUse, "failed rows must release the writer")
			observer.fault = ""
			require.NoError(t, health.Check(context.Background()))
			require.True(t, health.Healthy())
		})
	}
}

func TestHealthSQLiteCatalogFactoryPoolContention(t *testing.T) {
	for _, role := range []string{"writer", "reader"} {
		t.Run(role, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "health.db")
			rawWriter, err := db.OpenSQLite(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, rawWriter.Close()) })
			rawReader, err := db.OpenSQLiteReader(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, rawReader.Close()) })
			writer, reader := sqlx.NewDb(rawWriter, "sqlite3"), sqlx.NewDb(rawReader, "sqlite3")
			health := seedBatchHealth(t, db.NewPool(writer, reader))
			require.NoError(t, health.Check(context.Background()))
			blocked := writer
			if role == "reader" {
				blocked = reader
			}
			// Reserve every connection in the selected factory-created pool.
			var held []*sql.Conn
			t.Cleanup(func() {
				for _, conn := range held {
					_ = conn.Close()
				}
			})
			for range blocked.Stats().MaxOpenConnections {
				conn, err := blocked.Conn(context.Background())
				require.NoError(t, err)
				held = append(held, conn)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			diagnostic, err := health.check(ctx)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, role+"_ping", diagnostic.stage)
			require.Equal(t, []string{"a", "b", "c"}, health.UnhealthyStoreIDs())
			for _, conn := range held {
				require.NoError(t, conn.Close())
			}
			held = nil
			require.NoError(t, health.Check(context.Background()))
			require.True(t, health.Healthy())
		})
	}
}
