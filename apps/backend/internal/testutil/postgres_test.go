package testutil

import (
	"context"
	"testing"
)

func TestOpenIsolatedPostgresScopesEveryPoolConnection(t *testing.T) {
	db := OpenIsolatedPostgres(t, PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	ctx := context.Background()
	first, err := db.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	var schema string
	if err := first.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ExecContext(ctx, "CREATE TABLE pool_scope_probe (value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ExecContext(ctx, "INSERT INTO pool_scope_probe VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	second, err := db.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	var secondSchema string
	if err := second.GetContext(ctx, &secondSchema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	if secondSchema != schema {
		t.Fatalf("second connection schema = %q, want isolated schema %q", secondSchema, schema)
	}
	var count int
	if err := second.GetContext(ctx, &count, "SELECT COUNT(*) FROM pool_scope_probe"); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pooled connection sees %d rows, want 1", count)
	}
}
