package sqlite

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

type legacyRoleAssignment struct {
	AgentID      string `db:"agent_id"`
	RoleID       string `db:"role_id"`
	Name         string `db:"name"`
	Instructions string `db:"instructions"`
	Settings     string `db:"settings"`
	Icon         string
}

// Adding the icon column also marks the transactional transition from copied
// workspace identities/instructions to globally owned role configuration.
func (r *Repository) migrateRoleConfiguration() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := db.ColumnExists(tx, "orchestration_roles", "icon")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE orchestration_roles ADD COLUMN icon TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	profiles, err := db.TableExists(tx, "agent_profiles")
	if err != nil {
		return err
	}
	if profiles {
		if err := migrateRoleAssignments(tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func migrateRoleAssignments(tx *sqlx.Tx) error {
	settingsColumn, err := db.ColumnExists(tx, "agent_profiles", "settings")
	if err != nil {
		return err
	}
	settings := "''"
	if settingsColumn {
		settings = "COALESCE(a.settings,'')"
	}
	var rows []legacyRoleAssignment
	query := fmt.Sprintf(`SELECT o.agent_id,o.role_id,a.name,COALESCE(i.content,r.instructions) AS instructions,%s AS settings
 FROM workspace_orchestrators o JOIN agent_profiles a ON a.id=o.agent_id
 JOIN orchestration_roles r ON r.id=o.role_id
 LEFT JOIN orchestration_instructions i ON i.agent_profile_id=o.agent_id AND i.filename='ROLE.md'`, settings)
	if err := tx.Select(&rows, query); err != nil {
		return err
	}
	groups := map[string]map[string][]legacyRoleAssignment{}
	for _, row := range rows {
		var config struct {
			Icon string `json:"orchestrator_icon"`
		}
		if err := json.Unmarshal([]byte(row.Settings), &config); err == nil {
			row.Icon = config.Icon
		}
		data, _ := json.Marshal([]string{row.Name, row.Icon, row.Instructions})
		if groups[row.RoleID] == nil {
			groups[row.RoleID] = map[string][]legacyRoleAssignment{}
		}
		groups[row.RoleID][string(data)] = append(groups[row.RoleID][string(data)], row)
	}
	for roleID, variants := range groups {
		if err := preserveRoleVariants(tx, roleID, variants); err != nil {
			return err
		}
	}
	return nil
}

func preserveRoleVariants(tx *sqlx.Tx, roleID string, variants map[string][]legacyRoleAssignment) error {
	for key, assignments := range variants {
		row := assignments[0]
		target := roleID
		if len(variants) > 1 {
			target = uuid.NewSHA1(uuid.NameSpaceOID, []byte(roleID+key)).String()
		}
		if _, err := tx.Exec(tx.Rebind(`INSERT INTO orchestration_roles(id,name,icon,instructions) VALUES(?,?,?,?)
   ON CONFLICT(id) DO UPDATE SET name=excluded.name,icon=excluded.icon,instructions=excluded.instructions`), target, row.Name, row.Icon, row.Instructions); err != nil {
			return err
		}
		for _, assignment := range assignments {
			if _, err := tx.Exec(tx.Rebind(`UPDATE workspace_orchestrators SET role_id=? WHERE agent_id=?`), target, assignment.AgentID); err != nil {
				return err
			}
		}
	}
	return nil
}
