//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

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
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
}

func (c *usageBenchConnection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.observer.record(usageBenchStatement{SQL: query, Args: append([]driver.NamedValue(nil), args...)}, nil)
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *usageBenchConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.observer.record(usageBenchStatement{SQL: query, Args: append([]driver.NamedValue(nil), args...)}, nil)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
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
