package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const sidebarScratchTable = "temp.kandev_sidebar_filtered"

// A pinned connection owns the scratch relation until transaction and cleanup finish.
type sidebarQuerySnapshot struct {
	conn         *sqlx.Conn
	tx           *sqlx.Tx
	sqlite       bool
	commitFailed bool
	afterStage   func(string, *sqlx.Tx) error
}

func beginSidebarQuerySnapshot(ctx context.Context, reader *sqlx.DB) (*sidebarQuerySnapshot, error) {
	conn, err := reader.Connx(ctx)
	if err != nil {
		return nil, fmt.Errorf("reserve sidebar reader: %w", err)
	}
	tx, err := conn.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("begin sidebar query snapshot: %w", err)
	}
	return &sidebarQuerySnapshot{conn: conn, tx: tx, sqlite: !dialect.IsPostgres(reader.DriverName())}, nil
}

func (s *sidebarQuerySnapshot) prepare(
	ctx context.Context, driverName, workspaceID string, query models.SidebarTaskViewQuery,
) (string, []any, error) {
	if !s.sqlite {
		// Interactive reads cannot amortize compilation of each recursive query shape.
		if _, err := s.tx.ExecContext(ctx, "SET LOCAL jit = off"); err != nil {
			return "", nil, fmt.Errorf("configure sidebar query execution: %w", err)
		}
		return sidebarTaskBaseSQL(driverName, workspaceID, query)
	}
	candidateSQL, candidateArgs, err := sidebarTaskCandidateSQL(driverName, workspaceID, query)
	if err != nil {
		return "", nil, err
	}
	if _, err := s.tx.ExecContext(ctx, "CREATE TEMP TABLE "+sidebarScratchTable+" AS "+candidateSQL+" SELECT * FROM filtered", candidateArgs...); err != nil {
		return "", nil, fmt.Errorf("stage sidebar candidates: %w", err)
	}
	if err := s.checkpoint("created"); err != nil {
		return "", nil, err
	}
	for _, column := range []string{"id", "parent_id"} {
		unique := ""
		if column == "id" {
			unique = "UNIQUE "
		}
		if _, err := s.tx.ExecContext(ctx, "CREATE "+unique+"INDEX temp.kandev_sidebar_"+column+" ON kandev_sidebar_filtered ("+column+")"); err != nil {
			return "", nil, fmt.Errorf("index sidebar candidates: %w", err)
		}
	}
	if _, err := s.tx.ExecContext(ctx, "ANALYZE "+sidebarScratchTable); err != nil {
		return "", nil, fmt.Errorf("analyze sidebar candidates: %w", err)
	}
	if err := s.checkpoint("indexed"); err != nil {
		return "", nil, err
	}
	visibleSQL, visibleArgs := sidebarVisibleCTE(driverName, query)
	return "WITH RECURSIVE filtered AS NOT MATERIALIZED (SELECT * FROM " + sidebarScratchTable + ")" + visibleSQL, visibleArgs, nil
}

func (s *sidebarQuerySnapshot) commit(ctx context.Context) error {
	if s.sqlite {
		if _, err := s.tx.ExecContext(ctx, "DROP TABLE "+sidebarScratchTable+"; DROP TABLE IF EXISTS "+sidebarPreferenceTable); err != nil {
			return fmt.Errorf("release sidebar candidates: %w", err)
		}
	}
	if err := s.checkpoint("commit"); err != nil {
		return err
	}
	if err := s.tx.Commit(); err != nil {
		s.commitFailed = true
		return fmt.Errorf("commit sidebar query snapshot: %w", err)
	}
	return nil
}

func (s *sidebarQuerySnapshot) checkpoint(stage string) error {
	if s.afterStage != nil {
		return s.afterStage(stage, s.tx)
	}
	return nil
}

func (s *sidebarQuerySnapshot) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.release(ctx)
}

func (s *sidebarQuerySnapshot) release(cleanupCtx context.Context) {
	defer func() { _ = s.conn.Close() }()
	err := s.tx.Rollback()
	clean := !s.commitFailed && cleanupCtx.Err() == nil && (err == nil || errors.Is(err, sql.ErrTxDone))
	if s.sqlite && clean {
		_, err = s.conn.ExecContext(cleanupCtx, "DROP TABLE IF EXISTS "+sidebarScratchTable+"; DROP TABLE IF EXISTS "+sidebarPreferenceTable)
		clean = err == nil && cleanupCtx.Err() == nil
	}
	if !clean {
		// ErrConnDone means database/sql already discarded a cancelled transaction's connection.
		_ = s.conn.Raw(func(any) error { return driver.ErrBadConn })
	}
}
