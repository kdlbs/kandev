package testutil

import (
	"context"
	"testing"
)

func TestIsolatedPostgresPoolSharesSchema(t *testing.T) {
	db := OpenIsolatedPostgres(t, PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	first, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := first.ExecContext(context.Background(), "CREATE TABLE pool_probe (value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ExecContext(context.Background(), "INSERT INTO pool_probe VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	second, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	var value int
	if err := second.QueryRowContext(context.Background(), "SELECT value FROM pool_probe").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("pooled connection value = %d, want 42", value)
	}
}
