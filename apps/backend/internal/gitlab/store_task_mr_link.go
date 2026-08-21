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
	"time"
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
// failed to populate) have no signal at all: provider/provider_host/
// provider_owner/provider_name AND remote_url are all blank. Only for those
// rows, as a last resort, this re-derives the origin directly from the
// repository's local git checkout (local_path) and, if it matches the MR,
// opportunistically backfills remote_url so the local filesystem read isn't
// needed on subsequent calls. The local-checkout fallback deliberately never
// runs when the row already carries ANY durable identity signal (even one
// that doesn't match the MR), so it can never override an already-known
// repository's identity with an unrelated local checkout. Rows with no
// signal resolving to a GitLab identity fail closed.
func (s *Store) ValidateTaskMRRepositoryIdentity(
	ctx context.Context,
	workspaceID, taskID, repositoryID, configuredHost, projectPath string,
) error {
	if repositoryID == "" {
		return ErrTaskMRRepositoryMismatch
	}
	var identity taskMRRepositoryIdentityRow
	err := s.ro.GetContext(ctx, &identity, `
		SELECT COALESCE(r.provider, '') AS provider,
			COALESCE(r.provider_repo_id, '') AS provider_repo_id,
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
	// Legacy rows only: no durable identity signal at all. A row that
	// already carries any provider/host/owner/name/remote_url value has an
	// established identity, even if it doesn't match this MR, and must not
	// be overridden by a coincidentally-matching local checkout.
	if identity.hasNoDurableIdentitySignal() {
		if localCandidate, ok := localCheckoutIdentityCandidate(identity.LocalPath); ok {
			candidates = append(candidates, localCandidate)
		}
	}
	for _, c := range candidates {
		if sameGitLabHost(c.host, wantHost) && strings.EqualFold(c.project, wantProject) {
			if c.backfillRemoteURL != "" {
				s.backfillRepositoryRemoteURL(ctx, repositoryID, c.backfillRemoteURL)
			}
			return nil
		}
	}
	return ErrTaskMRRepositoryMismatch
}

// taskMRRepositoryIdentityRow is the repository identity data loaded for one
// task-repository pair.
type taskMRRepositoryIdentityRow struct {
	Provider       string `db:"provider"`
	ProviderRepoID string `db:"provider_repo_id"`
	Host           string `db:"provider_host"`
	Owner          string `db:"provider_owner"`
	Name           string `db:"provider_name"`
	RemoteURL      string `db:"remote_url"`
	LocalPath      string `db:"local_path"`
}

// hasNoDurableIdentitySignal reports whether the row carries absolutely no
// identity signal: no durable provider/provider_repo_id/host/owner/name AND
// no remote_url. Only such rows are eligible for the local-checkout
// fallback, so a row that already identifies a (possibly different)
// repository can never be overridden by a coincidentally-matching local
// checkout.
func (r taskMRRepositoryIdentityRow) hasNoDurableIdentitySignal() bool {
	return r.Provider == "" && r.ProviderRepoID == "" && r.Host == "" && r.Owner == "" &&
		r.Name == "" && r.RemoteURL == ""
}

// taskMRIdentityCandidate is one (host, project) identity derived from a
// repository row that an incoming MR's host/project can be checked against.
// backfillRemoteURL is set only for candidates re-derived from a local
// checkout, so the caller knows to persist it onto the repository row on a
// match. It deliberately holds a credential-free canonical "host/project"
// value rather than the raw local remote URL, since the raw value may embed
// userinfo credentials (e.g. "https://user:token@host/...") that must never
// be persisted into a column returned by repository API payloads.
type taskMRIdentityCandidate struct{ host, project, backfillRemoteURL string }

// localCheckoutIdentityCandidate re-derives a GitLab (host, project) identity
// from a repository's local git checkout, for legacy rows where neither the
// durable provider identity nor remote_url carries any signal. ok is false
// when there is nothing to fall back to (no local_path) or the checkout's
// origin can't be parsed as a GitLab remote.
func localCheckoutIdentityCandidate(localPath string) (candidate taskMRIdentityCandidate, ok bool) {
	if localPath == "" {
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
	backfillRemoteURL := strings.TrimRight(gotHost, "/") + "/" + strings.Trim(remoteProject, "/")
	return taskMRIdentityCandidate{host: gotHost, project: remoteProject, backfillRemoteURL: backfillRemoteURL}, true
}

// backfillRepositoryRemoteURL persists a remote_url re-derived from a local
// git checkout onto a legacy repository row so future MR-link checks (and
// any other remote_url consumer) no longer need a filesystem read. Best
// effort: failures are swallowed since the caller already has what it needs
// to proceed with the current request.
func (s *Store) backfillRepositoryRemoteURL(ctx context.Context, repositoryID, remoteURL string) {
	_, _ = s.db.ExecContext(ctx, `
		UPDATE repositories
		SET remote_url = ?, updated_at = ?
		WHERE id = ? AND (remote_url IS NULL OR remote_url = '')`,
		remoteURL, time.Now().UTC(), repositoryID)
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
// git config file content. Tolerant of the whitespace-around-"=" and
// key-casing variations git itself accepts ("url = x", "url=x", "URL\t=\tx"),
// since hand-edited or tool-written configs don't always match the exact
// "url = " spacing git-init produces.
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
		if !inOrigin {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "url") {
			return strings.TrimSpace(value)
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
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_state
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		association.TaskID, association.RepositoryID, association.ProjectPath, association.MRIID,
	); err != nil {
		return fmt.Errorf("delete task MR lifecycle state: %w", err)
	}
	// Drop this MR's automation switches with the association. Leaving the row
	// behind means re-linking the same MR later — by hand, or through
	// push-detection auto-link — silently re-arms whatever was configured
	// before it was unlinked, including auto-merge. Unlinking is the natural
	// "stop automating this" action, so it must not leave a latent enabled
	// switch that no surface displays (taskMRAutomationOptionsList hides rows
	// whose MR is not linked) but the evaluator still reads.
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_automation_options
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		association.TaskID, association.RepositoryID, association.ProjectPath, association.MRIID,
	); err != nil {
		return fmt.Errorf("delete task MR automation options: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gitlab_task_mrs WHERE id = ?`, association.ID); err != nil {
		return fmt.Errorf("delete task MR association: %w", err)
	}
	return tx.Commit()
}
