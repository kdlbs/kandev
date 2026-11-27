package testutil

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestIsolatedPostgresDSNScopesEveryConnection(t *testing.T) {
	const schema = "kandev_test_owned"
	for _, dsn := range []string{
		"postgres://fixture:fake%20password@localhost:5432/fixture?sslmode=disable&application_name=fixture",
		"postgresql://fixture:fake%20password@localhost:5432/fixture?sslmode=disable&application_name=fixture&search_path=public",
		"host=localhost port=5432 dbname=fixture user=fixture password='fake password' sslmode=disable application_name=fixture",
		"host=localhost port=5432 dbname=fixture user=fixture password='fake password' sslmode=disable application_name=fixture search_path=public",
	} {
		t.Run(dsn, func(t *testing.T) {
			scoped, err := isolatedPostgresDSN(dsn, schema)
			if err != nil {
				t.Fatal(err)
			}
			config, err := pgx.ParseConfig(scoped)
			if err != nil {
				t.Fatal(err)
			}
			if config.RuntimeParams["search_path"] != schema || config.RuntimeParams["application_name"] != "fixture" {
				t.Fatalf("startup parameters = %v", config.RuntimeParams)
			}
			if config.Database != "fixture" || config.User != "fixture" || config.Password != "fake password" || config.Host != "localhost" || config.Port != 5432 {
				t.Fatal("scoping changed connection identity")
			}
		})
	}
}

func TestIsolatedPostgresDSNRejectsInvalidURLWithoutCredentials(t *testing.T) {
	const password = "fake-test-password"
	for _, dsn := range []string{
		"postgres://fixture:" + password + "@localhost/%zz",
		"postgres://fixture:" + password + "@localhost/fixture?sslmode=%zz",
	} {
		_, err := isolatedPostgresDSN(dsn, "kandev_test_owned")
		if err == nil {
			t.Fatal("invalid connection URL was accepted")
		}
		if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), dsn) {
			t.Fatal("parse error exposes connection credentials")
		}
	}
}
