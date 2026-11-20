package github

import "fmt"

func (s *Store) migrateWorkspaceRepoScopes() error {
	columns, err := s.tableColumns("github_workspace_settings")
	if err != nil {
		return fmt.Errorf("read repository scope settings columns: %w", err)
	}
	for _, column := range []string{"repo_scope_mode", "repo_scope_orgs", "repo_scope_repos"} {
		if _, ok := columns[column]; !ok {
			return nil
		}
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin repository scope migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(tx.Rebind(`UPDATE github_workspace_settings SET
		repo_scope_mode = CASE lower(trim(repo_scope_mode))
			WHEN '' THEN 'all'
			WHEN 'all' THEN 'all'
			WHEN 'orgs' THEN 'selected'
			WHEN 'repos' THEN 'selected'
			WHEN 'selected' THEN 'selected'
			ELSE repo_scope_mode
		END,
		repo_scope_orgs = CASE lower(trim(repo_scope_mode))
			WHEN '' THEN '[]'
			WHEN 'all' THEN '[]'
			WHEN 'repos' THEN '[]'
			ELSE repo_scope_orgs
		END,
		repo_scope_repos = CASE lower(trim(repo_scope_mode))
			WHEN '' THEN '[]'
			WHEN 'all' THEN '[]'
			WHEN 'orgs' THEN '[]'
			ELSE repo_scope_repos
		END`))
	if err != nil {
		return fmt.Errorf("canonicalize repository scopes: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit repository scope migration: %w", err)
	}
	return nil
}
