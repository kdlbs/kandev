package storeconformance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

// behaviorScenarios exercises the tables owned by the adapter. The generic
// engine behavior suite in testutil covers portable CRUD and capability
// semantics on its dedicated engine fixture; these callbacks prove that each
// catalog adapter reaches its real schema instead of a synthetic table.
func behaviorScenarios(descriptor requiredstores.Descriptor) testconformance.Scenarios {
	scenarios := testconformance.Scenarios{CRUD: ownerCRUDScenario(descriptor)}
	for _, capability := range descriptor.Capabilities {
		scenarios.Capabilities = append(scenarios.Capabilities, testconformance.CapabilityScenario{
			Capability: capability,
			Run:        ownerCapabilityScenario(descriptor, capability),
		})
	}
	return scenarios
}

func ownerCRUDScenario(descriptor requiredstores.Descriptor) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		for _, table := range descriptor.RequiredTables {
			if err := exerciseOwnerTable(s, table); err != nil {
				return fmt.Errorf("%s CRUD on %s: %w", descriptor.ID, table, err)
			}
		}
		return nil
	}
}

func ownerCapabilityScenario(descriptor requiredstores.Descriptor, capability requiredstores.Capability) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		for _, table := range descriptor.RequiredTables {
			if err := probeOwnerTable(s, table, capability); err != nil {
				return fmt.Errorf("%s %s on %s: %w", descriptor.ID, capability, table, err)
			}
		}
		return nil
	}
}

func exerciseOwnerTable(s testconformance.ScenarioContext, table string) error {
	columns, err := db.TableColumns(s.DB, table)
	if err != nil {
		return fmt.Errorf("read columns: %w", err)
	}
	columnNames := make([]string, 0, len(columns))
	for column := range columns {
		columnNames = append(columnNames, column)
	}
	if len(columnNames) == 0 {
		return fmt.Errorf("table has no columns")
	}
	sort.Strings(columnNames)
	quotedTable := quoteIdentifier(table)
	quotedColumn := quoteIdentifier(columnNames[0])
	statements := []struct {
		name  string
		query string
	}{
		{name: "create", query: fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s WHERE 1 = 0`, quotedTable, quotedColumn, quotedColumn, quotedTable)},
		{name: "read", query: fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quotedTable)},
		{name: "update", query: fmt.Sprintf(`UPDATE %s SET %s = %s WHERE 1 = 0`, quotedTable, quotedColumn, quotedColumn)},
		{name: "delete", query: fmt.Sprintf(`DELETE FROM %s WHERE 1 = 0`, quotedTable)},
	}
	for _, statement := range statements {
		if statement.name == "read" {
			var count int64
			if err := s.DB.QueryRowxContext(s.Context, s.DB.Rebind(statement.query)).Scan(&count); err != nil {
				return fmt.Errorf("%s: %w", statement.name, err)
			}
			continue
		}
		if _, err := s.DB.ExecContext(s.Context, s.DB.Rebind(statement.query)); err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
	}
	return nil
}

func probeOwnerTable(s testconformance.ScenarioContext, table string, capability requiredstores.Capability) error {
	query, err := ownerCapabilityQuery(s, table, capability)
	if err != nil {
		return err
	}
	if capability == requiredstores.CapabilityTransaction {
		return probeOwnerTableTransaction(s, query)
	}
	if capability == requiredstores.CapabilityBoolean {
		return probeOwnerBoolean(s, query)
	}
	var value any
	return s.DB.QueryRowxContext(s.Context, s.DB.Rebind(query)).Scan(&value)
}

func ownerCapabilityQuery(s testconformance.ScenarioContext, table string, capability requiredstores.Capability) (string, error) {
	quotedTable := quoteIdentifier(table)
	if capability == requiredstores.CapabilityTimestamp {
		column, err := firstExistingColumn(s, table, []string{"created_at", "updated_at"})
		if err != nil {
			return "", err
		}
		if column != "" {
			return fmt.Sprintf(`SELECT MAX(%s) FROM %s`, quoteIdentifier(column), quotedTable), nil
		}
	}
	if capability == requiredstores.CapabilityBoolean {
		column, err := firstExistingColumn(s, table, []string{"enabled", "active", "hidden", "deleted"})
		if err != nil {
			return "", err
		}
		if column != "" {
			return fmt.Sprintf(`SELECT %s FROM %s LIMIT 1`, quoteIdentifier(column), quotedTable), nil
		}
	}
	return fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quotedTable), nil
}

func firstExistingColumn(s testconformance.ScenarioContext, table string, candidates []string) (string, error) {
	columns, err := db.TableColumns(s.DB, table)
	if err != nil {
		return "", fmt.Errorf("read columns: %w", err)
	}
	for _, candidate := range candidates {
		if columns[candidate] {
			return candidate, nil
		}
	}
	return "", nil
}

func probeOwnerTableTransaction(s testconformance.ScenarioContext, query string) error {
	tx, err := s.DB.BeginTxx(s.Context, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	var count int64
	if err := tx.QueryRowxContext(s.Context, tx.Rebind(query)).Scan(&count); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("read in transaction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func probeOwnerBoolean(s testconformance.ScenarioContext, query string) error {
	rows, err := s.DB.QueryxContext(s.Context, s.DB.Rebind(query))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var value any
		if err := rows.Scan(&value); err != nil {
			return err
		}
	}
	return rows.Err()
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
