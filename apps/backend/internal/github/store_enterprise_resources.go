package github

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/githubhost"
	"github.com/kandev/kandev/internal/gitcredentials"
)

var enterpriseResourceTables = []string{
	"github_pr_watches", "github_task_prs", "github_review_watches", "github_review_pr_tasks", "github_issue_watches", "github_issue_watch_tasks",
}

func (s *Store) addEnterpriseResourceColumns() error {
	missing := make([]string, 0, len(enterpriseResourceTables))
	for _, table := range enterpriseResourceTables {
		if !s.tableExists(table) {
			continue
		}
		columns, err := s.tableColumns(table)
		if err != nil {
			return err
		}
		if _, exists := columns["github_host"]; !exists {
			missing = append(missing, table)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	repositoryHosts, err := s.historicalRepositoryHosts()
	if err != nil {
		return err
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range missing {
		if _, err := tx.Exec("ALTER TABLE " + table + " ADD COLUMN github_host TEXT NOT NULL DEFAULT 'github.com'"); err != nil {
			return fmt.Errorf("add GitHub resource host: %w", err)
		}
		if err := backfillResourceHost(tx, table, repositoryHosts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) historicalRepositoryHosts() (map[string]string, error) {
	if !s.tableExists("repositories") {
		return nil, nil
	}
	columns, err := s.tableColumns("repositories")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"provider", "provider_host", "provider_owner", "provider_name", "remote_url"} {
		if _, exists := columns[name]; !exists {
			return nil, nil
		}
	}
	var rows []struct {
		ID           string `db:"id"`
		Provider     string `db:"provider"`
		ProviderHost string `db:"provider_host"`
		Owner        string `db:"provider_owner"`
		Name         string `db:"provider_name"`
		RemoteURL    string `db:"remote_url"`
	}
	if err := s.ro.Select(&rows, "SELECT id,provider,provider_host,provider_owner,provider_name,remote_url FROM repositories"); err != nil {
		return nil, err
	}
	hosts := make(map[string]string, len(rows))
	for _, row := range rows {
		hosts[row.ID] = ""
		if !strings.EqualFold(row.Provider, "github") {
			continue
		}
		identity, err := gitcredentials.ResolveRepositoryIdentity(gitcredentials.RepositoryIdentityInput{RepositoryID: row.ID, Provider: row.Provider, ProviderHost: row.ProviderHost, ProviderOwner: row.Owner, ProviderName: row.Name, RemoteURL: row.RemoteURL})
		if err != nil {
			continue
		}
		host, err := githubhost.Parse(identity.Host)
		if err == nil {
			hosts[row.ID] = host.String()
		}
	}
	return hosts, nil
}

type historicalResourceHostRow struct {
	ID           string `db:"id"`
	RepositoryID string `db:"repository_id"`
	URL          string `db:"resource_url"`
	ParentHost   string `db:"parent_host"`
}

func backfillResourceHost(tx *sqlx.Tx, table string, repositoryHosts map[string]string) error {
	query := historicalResourceHostQuery(table)
	if query == "" {
		return nil
	}
	var rows []historicalResourceHostRow
	if err := tx.Select(&rows, query); err != nil {
		return err
	}
	for _, row := range rows {
		host := resolvedHistoricalResourceHost(table, row, repositoryHosts)
		if _, err := tx.Exec(tx.Rebind("UPDATE "+table+" SET github_host = ? WHERE id = ?"), host, row.ID); err != nil {
			return err
		}
	}
	return nil
}

func historicalGitHubURLHost(rawURL string) string {
	if rawURL == "" {
		return defaultGitHubHost
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return ""
	}
	host, err := githubhost.Parse(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return ""
	}
	return host.String()
}

func normalizedResourceHost(value string) string {
	if value == "" {
		return defaultGitHubHost
	}
	host, err := githubhost.Parse(value)
	if err != nil {
		return ""
	}
	return host.String()
}

func historicalResourceHostQuery(table string) string {
	switch table {
	case "github_pr_watches":
		return "SELECT id,repository_id FROM github_pr_watches"
	case "github_task_prs":
		return "SELECT id,repository_id,pr_url AS resource_url FROM github_task_prs"
	case "github_review_pr_tasks":
		return "SELECT c.id,c.pr_url AS resource_url,COALESCE(p.github_host,'') AS parent_host FROM github_review_pr_tasks c LEFT JOIN github_review_watches p ON p.id=c.review_watch_id"
	case "github_issue_watch_tasks":
		return "SELECT c.id,c.issue_url AS resource_url,COALESCE(p.github_host,'') AS parent_host FROM github_issue_watch_tasks c LEFT JOIN github_issue_watches p ON p.id=c.issue_watch_id"
	default:
		return ""
	}
}
func resolvedHistoricalResourceHost(table string, row historicalResourceHostRow, repositoryHosts map[string]string) string {
	host := historicalGitHubURLHost(row.URL)
	if repositoryHosts != nil && row.RepositoryID != "" {
		repositoryHost := repositoryHosts[row.RepositoryID]
		if row.URL == "" {
			host = repositoryHost
		} else if repositoryHost != host {
			host = ""
		}
	}
	if (strings.HasSuffix(table, "_pr_tasks") || table == "github_issue_watch_tasks") && row.ParentHost != host {
		return ""
	}
	return host
}
