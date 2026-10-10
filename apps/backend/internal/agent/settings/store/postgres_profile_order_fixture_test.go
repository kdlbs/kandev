package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresIsolatedSchemaSurvivesPoolExpansion(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var schema string
	if err := database.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(schema, "kandev_test_") {
		t.Fatalf("fixture schema = %q", schema)
	}
	seen := make(map[int]bool)
	for range 8 {
		connection, err := database.Connx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = connection.Close() }()
		var actual string
		var pid int
		if err := connection.QueryRowContext(ctx, "SELECT current_schema(), pg_backend_pid()").Scan(&actual, &pid); err != nil {
			t.Fatal(err)
		}
		if actual != schema || seen[pid] {
			t.Fatalf("schema = %q, pid = %d; expected owned schema %q on a distinct connection", actual, pid, schema)
		}
		seen[pid] = true
	}
}
