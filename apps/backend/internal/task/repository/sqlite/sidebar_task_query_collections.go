package sqlite

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/db/dialect"
)

func sidebarStringListSQL(driver string, values []string) (string, []any) {
	encoded, _ := json.Marshal(values)
	if dialect.IsPostgres(driver) {
		return "SELECT jsonb_array_elements_text(CAST(? AS JSONB))", []any{string(encoded)}
	}
	return "SELECT value FROM json_each(?)", []any{string(encoded)}
}
