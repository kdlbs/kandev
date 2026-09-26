package storeconformance

import (
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

const engineBehaviorTable = "__kandev_conformance_engine_behavior"

// engineBehaviorScenario proves the portable runner contract independently of
// domain schemas. Domain adapters still execute against their owning tables;
// this dedicated fixture is the only generated table in the suite.
func engineBehaviorScenario(s ScenarioContext) error {
	schema := `CREATE TABLE IF NOT EXISTS "` + engineBehaviorTable + `" (
		id TEXT PRIMARY KEY,
		enabled {{boolean}} NOT NULL DEFAULT FALSE,
		value TEXT NOT NULL DEFAULT '',
		created_at {{timestamp}} NOT NULL DEFAULT {{current_time}}
	)`
	if _, err := s.DB.ExecContext(s.Context, dialect.MustRenderSchema(string(s.Engine), schema)); err != nil {
		return fmt.Errorf("create engine behavior table: %w", err)
	}
	if err := engineCRUD(s); err != nil {
		return err
	}
	if err := engineBoolean(s); err != nil {
		return err
	}
	if err := engineTimestamp(s); err != nil {
		return err
	}
	if err := engineConflict(s); err != nil {
		return err
	}
	return engineTransaction(s)
}

func engineCRUD(s ScenarioContext) error {
	table := `"` + engineBehaviorTable + `"`
	if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
		`INSERT INTO `+table+` (id, enabled, value, created_at) VALUES (?, ?, ?, ?)`,
	), "crud", false, "created", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return fmt.Errorf("engine create: %w", err)
	}
	var value string
	if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
		`SELECT value FROM `+table+` WHERE id = ?`,
	), "crud").Scan(&value); err != nil {
		return fmt.Errorf("engine read: %w", err)
	}
	if value != "created" {
		return fmt.Errorf("engine read value = %q, want created", value)
	}
	if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
		`UPDATE `+table+` SET value = ? WHERE id = ?`,
	), "updated", "crud"); err != nil {
		return fmt.Errorf("engine update: %w", err)
	}
	if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
		`DELETE FROM `+table+` WHERE id = ?`,
	), "crud"); err != nil {
		return fmt.Errorf("engine delete: %w", err)
	}
	return nil
}

func engineBoolean(s ScenarioContext) error {
	table := `"` + engineBehaviorTable + `"`
	for _, row := range []struct {
		id   string
		want bool
	}{
		{id: "boolean-false", want: false},
		{id: "boolean-true", want: true},
	} {
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
			`INSERT INTO `+table+` (id, enabled, value) VALUES (?, ?, ?)`,
		), row.id, row.want, row.id); err != nil {
			return fmt.Errorf("engine boolean insert %s: %w", row.id, err)
		}
		var got bool
		if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
			`SELECT enabled FROM `+table+` WHERE id = ?`,
		), row.id).Scan(&got); err != nil {
			return fmt.Errorf("engine boolean read %s: %w", row.id, err)
		}
		if got != row.want {
			return fmt.Errorf("engine boolean %s = %t, want %t", row.id, got, row.want)
		}
	}
	return nil
}

func engineTimestamp(s ScenarioContext) error {
	table := `"` + engineBehaviorTable + `"`
	first := time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	for _, row := range []struct {
		id string
		at time.Time
	}{{"timestamp-first", first}, {"timestamp-second", second}} {
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
			`INSERT INTO `+table+` (id, value, created_at) VALUES (?, ?, ?)`,
		), row.id, row.id, row.at); err != nil {
			return fmt.Errorf("engine timestamp insert %s: %w", row.id, err)
		}
	}
	rows, err := s.DB.QueryxContext(s.Context, s.DB.Rebind(
		`SELECT created_at FROM `+table+` WHERE id LIKE ? ORDER BY created_at`,
	), "timestamp-%")
	if err != nil {
		return fmt.Errorf("engine timestamp query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var got []time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return fmt.Errorf("engine timestamp scan: %w", err)
		}
		got = append(got, at.UTC())
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("engine timestamp rows: %w", err)
	}
	if len(got) != 2 || !got[0].Equal(first) || !got[1].Equal(second) {
		return fmt.Errorf("engine timestamps = %v, want %v and %v", got, first, second)
	}
	return nil
}

func engineConflict(s ScenarioContext) error {
	table := `"` + engineBehaviorTable + `"`
	query := `INSERT INTO ` + table + ` (id, value) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET value = excluded.value`
	for _, value := range []string{"first", "second"} {
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(query), "conflict", value); err != nil {
			return fmt.Errorf("engine conflict upsert %s: %w", value, err)
		}
	}
	var got string
	if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
		`SELECT value FROM `+table+` WHERE id = ?`,
	), "conflict").Scan(&got); err != nil {
		return fmt.Errorf("engine conflict read: %w", err)
	}
	if got != "second" {
		return fmt.Errorf("engine conflict value = %q, want second", got)
	}
	return nil
}

func engineTransaction(s ScenarioContext) error {
	table := `"` + engineBehaviorTable + `"`
	tx, err := s.DB.BeginTxx(s.Context, nil)
	if err != nil {
		return fmt.Errorf("engine begin rollback transaction: %w", err)
	}
	if _, err := tx.ExecContext(s.Context, tx.Rebind(
		`INSERT INTO `+table+` (id, value) VALUES (?, ?)`,
	), "transaction-rollback", "rollback"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("engine rollback insert: %w", err)
	}
	if err := tx.Rollback(); err != nil {
		return fmt.Errorf("engine rollback: %w", err)
	}
	var count int
	if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
		`SELECT COUNT(*) FROM `+table+` WHERE id = ?`,
	), "transaction-rollback").Scan(&count); err != nil {
		return fmt.Errorf("engine rollback read: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("engine rollback left %d row(s)", count)
	}
	committed, err := s.DB.BeginTxx(s.Context, nil)
	if err != nil {
		return fmt.Errorf("engine begin commit transaction: %w", err)
	}
	if _, err := committed.ExecContext(s.Context, committed.Rebind(
		`INSERT INTO `+table+` (id, value) VALUES (?, ?)`,
	), "transaction-commit", "commit"); err != nil {
		_ = committed.Rollback()
		return fmt.Errorf("engine commit insert: %w", err)
	}
	if err := committed.Commit(); err != nil {
		return fmt.Errorf("engine commit: %w", err)
	}
	return nil
}
