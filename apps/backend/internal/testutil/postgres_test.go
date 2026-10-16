package testutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
)

func TestIsolatedPostgresConnectionReplacement(t *testing.T) {
	database := OpenIsolatedPostgres(t, PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE retained_schema_probe (value INTEGER)`); err != nil {
		t.Fatal(err)
	}
	connection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Raw(func(any) error { return driver.ErrBadConn }); err != driver.ErrBadConn {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil && err != sql.ErrConnDone {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO retained_schema_probe (value) VALUES (1)`); err != nil {
		t.Fatalf("replacement connection lost isolated schema: %v", err)
	}
}
