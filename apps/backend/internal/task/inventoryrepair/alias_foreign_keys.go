package inventoryrepair

import (
	"context"
	"errors"
)

var aliasForeignKeyTables = []string{aliasEnvironmentTable, aliasSlotTable, aliasSessionTable, snapshotTable}

func aliasForeignKeyQuery(table string) string {
	return `SELECT f."table",f.rowid,f.parent,f.fkid,r.id AS inventory_id
 FROM pragma_foreign_key_check(?) f LEFT JOIN "` + table + `" r ON r.rowid=f.rowid
 ORDER BY f.rowid,f.parent,f.fkid`
}

// Record existing violations before fencing. Unrelated tables are outside the
// repair graph, and an invalid selected row cannot be safely restored on rollback.
func (in *inspection) inspectAliasForeignKeys(ctx context.Context) error {
	changes, err := in.aliasMutations(ctx)
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, change := range changes {
		selected[change.Table+":"+change.ID] = true
	}
	in.foreignKeys = map[string]int{}
	for _, table := range aliasForeignKeyTables {
		rows, err := in.read(ctx, "foreign-keys:"+table, aliasForeignKeyQuery(table), table)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if selected[table+":"+stringValue(r, "inventory_id")] {
				return errors.New("selected repair row has a pre-existing foreign-key violation")
			}
			in.foreignKeys[digest(r)]++
		}
	}
	return nil
}

func (j *journal) checkNewAliasForeignKeyViolations(ctx context.Context, db queryer) error {
	seen := map[string]int{}
	for _, table := range aliasForeignKeyTables {
		rows, err := queryRows(ctx, db, aliasForeignKeyQuery(table), table)
		if err != nil {
			return err
		}
		for _, r := range rows {
			key := digest(r)
			seen[key]++
			if seen[key] > j.ForeignKeyBaseline[key] {
				return errors.New("repair introduces a foreign-key violation")
			}
		}
	}
	return nil
}
