package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ValidateTaskMRScope verifies that the task and optional repository belong to
// the requested workspace. Unknown and cross-workspace resources deliberately
// share one error so callers do not disclose resource existence.
func (s *Store) ValidateTaskMRScope(ctx context.Context, workspaceID, taskID, repositoryID string) error {
	var taskExists int
	err := s.ro.GetContext(ctx, &taskExists,
		`SELECT 1 FROM tasks WHERE id = ? AND workspace_id = ? LIMIT 1`, taskID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("validate task workspace: %w", err)
	}
	if repositoryID == "" {
		return nil
	}
	var repositoryExists int
	err = s.ro.GetContext(ctx, &repositoryExists, `
		SELECT 1
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = ? AND tr.repository_id = ? AND r.workspace_id = ?
		LIMIT 1`, taskID, repositoryID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("validate task repository: %w", err)
	}
	return nil
}

// ValidateTaskMRRepositoryIdentity verifies the repository selected by the
// task identifies the same GitLab host and project as the MR. It accepts a
// match against either the repository's durable provider identity or its
// remote_url, since self-hosted GitLab repositories added as local clones
// never get a durable provider identity (only github.com/gitlab.com are
// tagged at discovery time) yet still carry a resolvable remote_url.
//
// Rows created before remote_url resolution existed (or where it otherwise
// failed to populate) have neither signal. For those, as a last resort, this
// re-derives the origin directly from the repository's local git checkout
// (local_path) and, if it matches the MR, opportunistically backfills
// remote_url so the local filesystem read isn't needed on subsequent calls.
// Rows with no signal resolving to a GitLab identity fail closed.
func (s *Store) ValidateTaskMRRepositoryIdentity(
	ctx context.Context,
	workspaceID, taskID, repositoryID, configuredHost, projectPath string,
) error {
	if repositoryID == "" {
		return ErrTaskMRRepositoryMismatch
	}
	var identity struct {
		Provider  string `db:"provider"`
		Host      string `db:"provider_host"`
		Owner     string `db:"provider_owner"`
		Name      string `db:"provider_name"`
		RemoteURL string `db:"remote_url"`
		LocalPath string `db:"local_path"`
	}
	err := s.ro.GetContext(ctx, &identity, `
		SELECT COALESCE(r.provider, '') AS provider,
			COALESCE(r.provider_host, '') AS provider_host,
			COALESCE(r.provider_owner, '') AS provider_owner,
			COALESCE(r.provider_name, '') AS provider_name,
			COALESCE(r.remote_url, '') AS remote_url,
			COALESCE(r.local_path, '') AS local_path
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		JOIN tasks t ON t.id = tr.task_id
		WHERE tr.task_id = ? AND tr.repository_id = ?
			AND t.workspace_id = ? AND r.workspace_id = ?
		LIMIT 1`, taskID, repositoryID, workspaceID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("load task repository identity: %w", err)
	}
	wantHost, err := normalizeHostOrigin(configuredHost)
	if err != nil {
		return ErrTaskMRRepositoryMismatch
	}
	wantProject := strings.Trim(strings.TrimSpace(projectPath), "/")

	var candidates []taskMRIdentityCandidate
	if strings.EqualFold(identity.Provider, "gitlab") {
		if gotHost, err := normalizeHostOrigin(identity.Host); err == nil {
			gotProject := strings.Trim(strings.TrimSpace(identity.Owner+"/"+identity.Name), "/")
			candidates = append(candidates, taskMRIdentityCandidate{host: gotHost, project: gotProject})
		}
	}
	if remoteURLHost, remoteProject := parseGitLabRemoteURLIdentity(identity.RemoteURL); remoteURLHost != "" {
		if gotHost, err := normalizeHostOrigin(remoteURLHost); err == nil {
			candidates = append(candidates, taskMRIdentityCandidate{host: gotHost, project: remoteProject})
		}
	}
	// Legacy rows: neither the durable identity nor remote_url resolved to
	// anything. Fall back to reading the local checkout's origin directly,
	// since local_path is the one signal every locally-cloned repository
	// carries regardless of when it was added.
	if localCandidate, ok := localCheckoutIdentityCandidate(identity.RemoteURL, identity.LocalPath); ok {
		candidates = append(candidates, localCandidate)
	}
	for _, c := range candidates {
		if sameGitLabHost(c.host, wantHost) && strings.EqualFold(c.project, wantProject) {
			if c.remoteURL != "" {
				s.backfillRepositoryRemoteURL(ctx, repositoryID, c.remoteURL)
			}
			return nil
		}
	}
	return ErrTaskMRRepositoryMismatch
}

// taskMRIdentityCandidate is one (host, project) identity derived from a
// repository row that an incoming MR's host/project can be checked against.
// remoteURL is set only for candidates re-derived from a local checkout, so
// the caller knows to backfill the repository row's remote_url on a match.
type taskMRIdentityCandidate struct{ host, project, remoteURL string }

// localCheckoutIdentityCandidate re-derives a GitLab (host, project) identity
// from a repository's local git checkout, for legacy rows where both the
// durable provider identity and remote_url are blank. ok is false when there
// is nothing to fall back to (no local_path, or remote_url already set) or
// the checkout's origin can't be parsed as a GitLab remote.
func localCheckoutIdentityCandidate(remoteURL, localPath string) (candidate taskMRIdentityCandidate, ok bool) {
	if remoteURL != "" || localPath == "" {
		return taskMRIdentityCandidate{}, false
	}
	localRemoteURL := resolveLocalGitOriginURL(localPath)
	remoteURLHost, remoteProject := parseGitLabRemoteURLIdentity(localRemoteURL)
	if remoteURLHost == "" {
		return taskMRIdentityCandidate{}, false
	}
	gotHost, err := normalizeHostOrigin(remoteURLHost)
	if err != nil {
		return taskMRIdentityCandidate{}, false
	}
	return taskMRIdentityCandidate{host: gotHost, project: remoteProject, remoteURL: localRemoteURL}, true
}

// backfillRepositoryRemoteURL persists a remote_url re-derived from a local
// git checkout onto a legacy repository row so future MR-link checks (and
// any other remote_url consumer) no longer need a filesystem read. Best
// effort: failures are swallowed since the caller already has what it needs
// to proceed with the current request.
func (s *Store) backfillRepositoryRemoteURL(ctx context.Context, repositoryID, remoteURL string) {
	_, _ = s.db.ExecContext(ctx, `
		UPDATE repositories
		SET remote_url = ?
		WHERE id = ? AND (remote_url IS NULL OR remote_url = '')`,
		remoteURL, repositoryID)
}

// resolveLocalGitOriginURL reads the "origin" remote URL directly from a
// local git checkout's config, without depending on the task/service
// package (see parseGitLabRemoteURLIdentity for the rationale: this package
// stays self-contained to avoid a gitlab -> task/service import cycle).
// Returns "" on any error, including non-existent paths and checkouts with
// no origin remote.
func resolveLocalGitOriginURL(repoPath string) string {
	gitDir, err := resolveLocalGitDir(repoPath)
	if err != nil {
		return ""
	}
	commonDir := resolveLocalCommonGitDir(gitDir)
	content, err := os.ReadFile(filepath.Join(commonDir, "config"))
	if err != nil {
		return ""
	}
	return parseLocalGitConfigOriginURL(string(content))
}

// resolveLocalGitDir resolves repoPath/.git to an absolute git directory,
// following the "gitdir: <path>" pointer file used by worktree checkouts.
func resolveLocalGitDir(repoPath string) (string, error) {
	gitPath := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return gitPath, nil
	}
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	gitDir, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("invalid gitdir reference")
	}
	gitDir = strings.TrimSpace(gitDir)
	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}
	return filepath.Clean(filepath.Join(repoPath, gitDir)), nil
}

// resolveLocalCommonGitDir follows a worktree's "commondir" file back to the
// main checkout's git directory, where its remotes are actually configured.
func resolveLocalCommonGitDir(gitDir string) string {
	content, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}
	commonDir := strings.TrimSpace(string(content))
	if commonDir == "" {
		return gitDir
	}
	if filepath.IsAbs(commonDir) {
		return filepath.Clean(commonDir)
	}
	return filepath.Clean(filepath.Join(gitDir, commonDir))
}

// parseLocalGitConfigOriginURL extracts the "origin" remote's url from raw
// git config file content.
func parseLocalGitConfigOriginURL(config string) string {
	inOrigin := false
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inOrigin = false
			continue
		}
		if inOrigin {
			if url, ok := strings.CutPrefix(line, "url = "); ok {
				return url
			}
		}
	}
	return ""
}

func sameGitLabHost(left, right string) bool {
	leftURL, leftErr := validateHost(strings.TrimSpace(left))
	rightURL, rightErr := validateHost(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(hostWithoutDefaultPort(leftURL), hostWithoutDefaultPort(rightURL))
}

// sameConfiguredOrigin reports whether candidate and configured identify the
// same GitLab web origin: same scheme, and hosts equal once each side's
// implicit default port is stripped. Used to validate an incoming MR URL's
// origin against the workspace's configured GitLab host, so an explicit
// "https://gitlab.example.com:443" on either side still matches the
// equivalent unported form.
func sameConfiguredOrigin(candidate, configured string) bool {
	candidateURL, candidateErr := validateHost(strings.TrimSpace(candidate))
	configuredURL, configuredErr := validateHost(strings.TrimSpace(configured))
	if candidateErr != nil || configuredErr != nil {
		return false
	}
	return strings.EqualFold(candidateURL.Scheme, configuredURL.Scheme) &&
		strings.EqualFold(hostWithoutDefaultPort(candidateURL), hostWithoutDefaultPort(configuredURL))
}

// hostWithoutDefaultPort strips a scheme's implicit default port (":443" for
// https, ":80" for http) so an explicitly-ported origin like
// "https://gitlab.example.com:443" compares equal to the equivalent
// "https://gitlab.example.com" without one.
func hostWithoutDefaultPort(u *url.URL) string {
	switch u.Scheme {
	case mentionHTTPSScheme:
		return strings.TrimSuffix(u.Host, ":443")
	case mentionHTTPScheme:
		return strings.TrimSuffix(u.Host, ":80")
	default:
		return u.Host
	}
}

// ResolveTaskMRRepository validates an explicit repository or infers the sole
// task repository. Scratch tasks retain an empty repository association;
// multi-repository tasks must make the choice explicit.
func (s *Store) ResolveTaskMRRepository(
	ctx context.Context,
	workspaceID, taskID, repositoryID string,
) (string, error) {
	if repositoryID != "" {
		if err := s.ValidateTaskMRScope(ctx, workspaceID, taskID, repositoryID); err != nil {
			return "", err
		}
		return repositoryID, nil
	}
	if err := s.ValidateTaskMRScope(ctx, workspaceID, taskID, ""); err != nil {
		return "", err
	}
	var repositoryIDs []string
	if err := s.ro.SelectContext(ctx, &repositoryIDs, `
		SELECT tr.repository_id
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = ? AND r.workspace_id = ?
		ORDER BY tr.id`, taskID, workspaceID); err != nil {
		return "", fmt.Errorf("list task repositories: %w", err)
	}
	switch len(repositoryIDs) {
	case 0:
		return "", nil
	case 1:
		return repositoryIDs[0], nil
	default:
		return "", ErrTaskMRRepositoryRequired
	}
}

// DeleteTaskMRForWorkspace atomically removes one association and only the
// refresh watch that identifies the same task, repository, project and MR.
func (s *Store) DeleteTaskMRForWorkspace(ctx context.Context, workspaceID, associationID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var association TaskMR
	err = tx.GetContext(ctx, &association, `
		SELECT `+taskMRSelectColsQualified+`
		FROM gitlab_task_mrs gtm
		JOIN tasks t ON t.id = gtm.task_id
		WHERE gtm.id = ? AND t.workspace_id = ?
		LIMIT 1`, associationID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("find task MR association: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM gitlab_mr_watches
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		association.TaskID, association.RepositoryID, association.ProjectPath, association.MRIID,
	); err != nil {
		return fmt.Errorf("delete task MR refresh watch: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gitlab_task_mrs WHERE id = ?`, association.ID); err != nil {
		return fmt.Errorf("delete task MR association: %w", err)
	}
	return tx.Commit()
}
