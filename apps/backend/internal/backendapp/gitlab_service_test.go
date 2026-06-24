package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/secrets"
)

type emptySecretStore struct{}

func (emptySecretStore) Create(context.Context, *secrets.SecretWithValue) error { return nil }
func (emptySecretStore) Get(context.Context, string) (*secrets.Secret, error)   { return nil, nil }
func (emptySecretStore) Reveal(context.Context, string) (string, error)         { return "", nil }
func (emptySecretStore) Update(context.Context, string, *secrets.UpdateSecretRequest) error {
	return nil
}
func (emptySecretStore) Delete(context.Context, string) error { return nil }
func (emptySecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}
func (emptySecretStore) Close() error { return nil }

func TestInitGitLabServiceLoadsConfiguredHostAfterRestart(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "true")
	t.Setenv("GITLAB_TOKEN", "")

	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"version":"16.0.0"}`))
	}))
	t.Cleanup(gitlabServer.Close)

	pool := newGitLabServiceTestPool(t)
	log := newTestLogger()
	secretsStore := emptySecretStore{}

	first := initGitLabService(pool, nil, secretsStore, log)
	if first == nil {
		t.Fatal("first GitLab service is nil")
	}
	if err := first.ConfigureHost(context.Background(), gitlabServer.URL+"/"); err != nil {
		t.Fatalf("ConfigureHost: %v", err)
	}

	second := initGitLabService(pool, nil, secretsStore, log)
	if second == nil {
		t.Fatal("second GitLab service is nil")
	}
	if got := second.Host(); got != gitlabServer.URL {
		t.Fatalf("Host after restart = %q, want %q", got, gitlabServer.URL)
	}
}

func newGitLabServiceTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "kandev.db")
	conn, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	pool := db.NewPool(conn, conn)
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}
