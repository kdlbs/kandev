package sqlite

import (
	"context"
	"testing"
	"time"
)

type retiredAzureRepositoryRow struct {
	id, provider, host, remoteURL, wantHost string
}

var retiredAzureRepositoryRows = []retiredAzureRepositoryRow{
	{"azure-https", "azure_devops", "", "https://dev.azure.com/acme/Platform/_git/api", "https://dev.azure.com"},
	{"azure-no-remote", "azure_devops", "", "", ""},
	{"azure-other-host", "azure_devops", "", "https://acme.visualstudio.com/Platform/_git/api", ""},
	{"azure-ssh", "azure_devops", "", "git@ssh.dev.azure.com:v3/acme/Platform/api", ""},
	{"azure-with-host", "azure_devops", "https://dev.azure.com", "https://dev.azure.com/acme/Platform/_git/web", "https://dev.azure.com"},
	{"other-provider-empty-host", "bitbucket", "", "https://dev.azure.com/acme/Platform/_git/api", ""},
}

func TestRetiredAzureDevOpsRepositoriesRecordTheirProviderHost(t *testing.T) {
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC()
	if _, err := repo.db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'ws-1', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	for _, row := range retiredAzureRepositoryRows {
		if _, err := repo.db.Exec(`INSERT INTO repositories
			(id, workspace_id, name, source_type, provider, provider_host, remote_url, created_at, updated_at)
			VALUES (?, 'ws-1', ?, 'provider', ?, ?, ?, ?, ?)`,
			row.id, row.id, row.provider, row.host, row.remoteURL, now, now); err != nil {
			t.Fatalf("insert repository %s: %v", row.id, err)
		}
	}

	for range 2 {
		if err := repo.runMigrations(context.Background()); err != nil {
			t.Fatalf("run migrations: %v", err)
		}
	}

	for _, row := range retiredAzureRepositoryRows {
		var host string
		if err := repo.db.QueryRow(`SELECT provider_host FROM repositories WHERE id = ?`, row.id).Scan(&host); err != nil {
			t.Fatalf("read repository %s: %v", row.id, err)
		}
		if host != row.wantHost {
			t.Fatalf("repository %s provider_host = %q, want %q", row.id, host, row.wantHost)
		}
	}
}
