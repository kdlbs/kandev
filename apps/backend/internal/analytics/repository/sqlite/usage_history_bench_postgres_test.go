//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func usageBenchPostgresTraceEnabled() bool {
	return os.Getenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES") == "1"
}

type usageBenchConnector struct {
	driver.Connector
	observer *usageBenchObserver
}

func (c usageBenchConnector) Connect(ctx context.Context) (driver.Conn, error) {
	connection, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &usageBenchConnection{Conn: connection, observer: c.observer}, nil
}

type usageBenchConnection struct {
	driver.Conn
	observer *usageBenchObserver
}

func (c *usageBenchConnection) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if !c.observer.limitedTrace() {
		return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
	}
	started := time.Now()
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
	if err != nil {
		c.observer.record(usageBenchStatement{SQL: "<BEGIN>", DurationNS: time.Since(started).Nanoseconds(), Error: err.Error()}, err)
		return nil, err
	}
	if c.observer.limitedTrace() {
		return &usageBenchTransaction{Tx: tx, observer: c.observer}, nil
	}
	return tx, nil
}

func (c *usageBenchConnection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !c.observer.active() {
		return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	}
	if !c.observer.limitedTrace() {
		c.observer.record(usageBenchStatement{SQL: query, Args: append([]driver.NamedValue(nil), args...)}, nil)
		return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	}
	started := time.Now()
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	statement := usageBenchStatement{SQL: query, DurationNS: time.Since(started).Nanoseconds(), Args: append([]driver.NamedValue(nil), args...)}
	if err != nil {
		statement.Error = err.Error()
	}
	c.observer.record(statement, err)
	return result, err
}

func (c *usageBenchConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !c.observer.active() {
		return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	}
	if !c.observer.limitedTrace() {
		c.observer.record(usageBenchStatement{SQL: query, Args: append([]driver.NamedValue(nil), args...)}, nil)
		return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	}
	started := time.Now()
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	statement := usageBenchStatement{SQL: query, Args: append([]driver.NamedValue(nil), args...)}
	if err != nil {
		statement.DurationNS, statement.Error = time.Since(started).Nanoseconds(), err.Error()
		c.observer.record(statement, err)
		return nil, err
	}
	return &usageBenchRows{Rows: rows, observer: c.observer, statement: statement, started: started}, nil
}

type usageBenchTransaction struct {
	driver.Tx
	observer *usageBenchObserver
}

func (tx *usageBenchTransaction) Commit() error {
	started := time.Now()
	err := tx.Tx.Commit()
	statement := usageBenchStatement{SQL: "<COMMIT>", DurationNS: time.Since(started).Nanoseconds()}
	if err != nil {
		statement.Error = err.Error()
	}
	tx.observer.record(statement, err)
	return err
}

func (tx *usageBenchTransaction) Rollback() error {
	started := time.Now()
	err := tx.Tx.Rollback()
	statement := usageBenchStatement{SQL: "<ROLLBACK>", DurationNS: time.Since(started).Nanoseconds()}
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		statement.Error = err.Error()
	}
	if errors.Is(err, sql.ErrTxDone) {
		err = nil
	}
	tx.observer.record(statement, err)
	return err
}

type usageBenchRows struct {
	driver.Rows
	observer  *usageBenchObserver
	statement usageBenchStatement
	started   time.Time
	once      sync.Once
}

func (rows *usageBenchRows) Next(destination []driver.Value) error {
	err := rows.Rows.Next(destination)
	if err != nil {
		rows.finish(err)
	}
	return err
}

func (rows *usageBenchRows) Close() error {
	err := rows.Rows.Close()
	rows.finish(err)
	return err
}

func (rows *usageBenchRows) finish(err error) {
	rows.once.Do(func() {
		rows.statement.DurationNS = time.Since(rows.started).Nanoseconds()
		if err != nil && !errors.Is(err, io.EOF) {
			rows.statement.Error = err.Error()
		} else {
			err = nil
		}
		rows.observer.record(rows.statement, err)
	})
}

func usageBenchPostgresPool(dsn string, observer *usageBenchObserver) (*sqlx.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL benchmark configuration")
	}
	connector := stdlib.GetConnector(*config, stdlib.OptionAfterConnect(func(_ context.Context, connection *pgx.Conn) error {
		for _, name := range []string{"timestamp", "timestamptz"} {
			registered, ok := connection.TypeMap().TypeForName(name)
			if !ok {
				return fmt.Errorf("PostgreSQL benchmark lacks timestamp codec")
			}
			replacement := *registered
			if name == "timestamp" {
				replacement.Codec = &pgtype.TimestampCodec{ScanLocation: time.UTC}
			} else {
				replacement.Codec = &pgtype.TimestamptzCodec{ScanLocation: time.UTC}
			}
			connection.TypeMap().RegisterType(&replacement)
		}
		return nil
	}))
	database := sqlx.NewDb(sql.OpenDB(usageBenchConnector{Connector: connector, observer: observer}), "pgx")
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	return database, nil
}

// Capture the real report first. Replay its exact SQL and bindings against the
// frozen fixture, including temporary-grid DDL/inserts, in one new transaction.
// EXPLAIN overhead cannot consume the production report's query deadline.
func explainUsageBenchPostgres(ctx context.Context, database *sqlx.DB, statements []usageBenchStatement) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for index := range statements {
		statement := &statements[index]
		args := make([]any, len(statement.Args))
		for i, value := range statement.Args {
			args[i] = value.Value
		}
		if !usageBenchReadSQL(statement.SQL) {
			if _, err := tx.ExecContext(ctx, statement.SQL, args...); err != nil {
				return err
			}
			continue
		}
		var plan []byte
		if err := tx.QueryRowContext(ctx, `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) `+statement.SQL, args...).Scan(&plan); err != nil {
			return err
		}
		if !json.Valid(plan) {
			return fmt.Errorf("PostgreSQL benchmark returned an invalid execution plan")
		}
		statement.PostgresPlan = append(json.RawMessage(nil), plan...)
	}
	return tx.Rollback()
}
