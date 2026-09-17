package sqlite

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/db/dialect"
)

func TestChildSetKeyAndGenerationQueryUsesOneChildSnapshot(t *testing.T) {
	for _, driver := range []string{dialect.SQLite3, dialect.PGX} {
		t.Run(driver, func(t *testing.T) {
			query := childSetKeyAndGenerationQuery(driver)

			if got := strings.Count(query, "FROM tasks"); got != 1 {
				t.Fatalf("query references tasks %d times, want one snapshot source: %s", got, query)
			}
			if !strings.Contains(query, "AS child_set_key") {
				t.Fatalf("query does not return child_set_key: %s", query)
			}
			if !strings.Contains(query, "AS child_generation") {
				t.Fatalf("query does not return child_generation: %s", query)
			}
		})
	}
}
