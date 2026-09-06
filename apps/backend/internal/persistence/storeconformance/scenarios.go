package storeconformance

import (
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func behaviorScenarios(id string, capabilities []requiredstores.Capability) testconformance.Scenarios {
	scenarios := testconformance.Scenarios{CRUD: crudScenario(id)}
	for _, capability := range capabilities {
		scenarios.Capabilities = append(scenarios.Capabilities, testconformance.CapabilityScenario{
			Capability: capability,
			Run:        capabilityScenario(id, capability),
		})
	}
	return scenarios
}

func crudScenario(id string) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		table := behaviorTable(id)
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
			`INSERT INTO "`+table+`" (id, enabled, value, created_at) VALUES (?, ?, ?, ?)`,
		), "crud", false, "created", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			return fmt.Errorf("create: %w", err)
		}
		var value string
		if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
			`SELECT value FROM "`+table+`" WHERE id = ?`,
		), "crud").Scan(&value); err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if value != "created" {
			return fmt.Errorf("read value = %q, want created", value)
		}
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
			`UPDATE "`+table+`" SET value = ? WHERE id = ?`,
		), "updated", "crud"); err != nil {
			return fmt.Errorf("update: %w", err)
		}
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
			`DELETE FROM "`+table+`" WHERE id = ?`,
		), "crud"); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	}
}

func capabilityScenario(id string, capability requiredstores.Capability) testconformance.Scenario {
	switch capability {
	case requiredstores.CapabilityBoolean:
		return booleanScenario(id)
	case requiredstores.CapabilityTimestamp:
		return timestampScenario(id)
	case requiredstores.CapabilityConflict:
		return conflictScenario(id)
	case requiredstores.CapabilityTransaction:
		return transactionScenario(id)
	default:
		return func(testconformance.ScenarioContext) error {
			return fmt.Errorf("unsupported capability %q", capability)
		}
	}
}

func booleanScenario(id string) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		table := behaviorTable(id)
		for _, row := range []struct {
			id   string
			want bool
		}{
			{id: "boolean-false", want: false},
			{id: "boolean-true", want: true},
		} {
			if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
				`INSERT INTO "`+table+`" (id, enabled, value) VALUES (?, ?, ?)`,
			), row.id, row.want, row.id); err != nil {
				return fmt.Errorf("insert %s: %w", row.id, err)
			}
			var got bool
			if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
				`SELECT enabled FROM "`+table+`" WHERE id = ?`,
			), row.id).Scan(&got); err != nil {
				return fmt.Errorf("read %s: %w", row.id, err)
			}
			if got != row.want {
				return fmt.Errorf("%s = %t, want %t", row.id, got, row.want)
			}
		}
		return nil
	}
}

func timestampScenario(id string) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		table := behaviorTable(id)
		first := time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC)
		second := first.Add(time.Hour)
		for _, row := range []struct {
			id string
			at time.Time
		}{{"timestamp-first", first}, {"timestamp-second", second}} {
			if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(
				`INSERT INTO "`+table+`" (id, value, created_at) VALUES (?, ?, ?)`,
			), row.id, row.id, row.at); err != nil {
				return fmt.Errorf("insert %s: %w", row.id, err)
			}
		}
		rows, err := s.DB.QueryxContext(s.Context, `SELECT created_at FROM "`+table+`" WHERE id LIKE 'timestamp-%' ORDER BY created_at`)
		if err != nil {
			return fmt.Errorf("query timestamps: %w", err)
		}
		defer func() { _ = rows.Close() }()
		var got []time.Time
		for rows.Next() {
			var at time.Time
			if err := rows.Scan(&at); err != nil {
				return fmt.Errorf("scan timestamp: %w", err)
			}
			got = append(got, at.UTC())
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(got) != 2 || !got[0].Equal(first) || !got[1].Equal(second) {
			return fmt.Errorf("timestamps = %v, want %v and %v", got, first, second)
		}
		return nil
	}
}

func conflictScenario(id string) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		table := behaviorTable(id)
		query := `INSERT INTO "` + table + `" (id, value) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET value = excluded.value`
		for _, value := range []string{"first", "second"} {
			if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(query), "conflict", value); err != nil {
				return fmt.Errorf("upsert %s: %w", value, err)
			}
		}
		var got string
		if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
			`SELECT value FROM "`+table+`" WHERE id = ?`,
		), "conflict").Scan(&got); err != nil {
			return err
		}
		if got != "second" {
			return fmt.Errorf("conflict value = %q, want second", got)
		}
		return nil
	}
}

func transactionScenario(id string) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		table := behaviorTable(id)
		rolledBack, err := s.DB.BeginTxx(s.Context, nil)
		if err != nil {
			return fmt.Errorf("begin rollback transaction: %w", err)
		}
		if _, err := rolledBack.ExecContext(s.Context, rolledBack.Rebind(
			`INSERT INTO "`+table+`" (id, value) VALUES (?, ?)`,
		), "transaction-rollback", "rollback"); err != nil {
			_ = rolledBack.Rollback()
			return err
		}
		if err := rolledBack.Rollback(); err != nil {
			return fmt.Errorf("rollback: %w", err)
		}
		var count int
		if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(
			`SELECT COUNT(*) FROM "`+table+`" WHERE id = ?`,
		), "transaction-rollback").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("rollback left %d row(s)", count)
		}
		committed, err := s.DB.BeginTxx(s.Context, nil)
		if err != nil {
			return fmt.Errorf("begin commit transaction: %w", err)
		}
		if _, err := committed.ExecContext(s.Context, committed.Rebind(
			`INSERT INTO "`+table+`" (id, value) VALUES (?, ?)`,
		), "transaction-commit", "commit"); err != nil {
			_ = committed.Rollback()
			return err
		}
		if err := committed.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil
	}
}

func renderSchema(engine, schema string) string {
	return dialect.MustRenderSchema(engine, schema)
}
