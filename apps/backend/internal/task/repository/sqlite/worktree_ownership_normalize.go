package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const (
	sessionStateCancelled = "CANCELLED"
	sessionStateCompleted = "COMPLETED"
	sessionStateFailed    = "FAILED"
)

// worktreeCutover holds the legacy inventory and the normalized result.
type worktreeCutover struct {
	envs                      map[string]*legacyEnv
	taskEnvs                  map[string][]*legacyEnv
	sessions                  map[string]*legacySession
	sessionTasks              map[string]string
	sessionEnvIDs             map[string]string
	sessionWorktreeSuperseded map[string]bool
	demotedWorktrees          map[string]bool
	demotedFlatEnvironments   map[string]bool
	demotions                 []string
	authoritativeWorktreeIDs  map[string]bool
	envRepos                  []legacyEnvRepo
	sessionWts                []legacySessionWorktree
	orphanedSessionWts        []legacySessionWorktree
	tasks                     map[string]*taskWorktreeTargets
	taskEnvIDs                map[string]string
	loserEnvIDs               map[string]bool
	executorTypes             map[string]string
	conflicts                 []string
}

type legacyEnv struct {
	id, taskID, repositoryID, executorType, executorID, executorProfileID           string
	controlPort                                                                     int
	status, worktreeID, worktreePath, worktreeBranch, workspacePath                 string
	containerID, containerBootstrapNonceSecretID, containerControlAuthTokenSecretID string
	sandboxID, taskDirName                                                          string
	createdAt, updatedAt                                                            time.Time
}

type legacySession struct {
	id, taskID, taskEnvironmentID string
	state                         string
	startedAt                     time.Time
	executorID, executorProfileID string
	repositoryID                  string
	containerID                   string
}

type legacyEnvRepo struct {
	id, envID, repositoryID, branchSlug      string
	worktreeID, worktreePath, worktreeBranch string
	position                                 int
	errorMessage, status                     string
	mergedAt, deletedAt                      *time.Time
	createdAt, updatedAt                     time.Time
}

type legacySessionWorktree struct {
	sessionID, worktreeID, repositoryID, branchSlug string
	position                                        int
	worktreePath, worktreeBranch, status            string
	createdAt, updatedAt                            time.Time
	mergedAt, deletedAt                             *time.Time
}

// loadLegacy reads every legacy ownership row into memory. The transaction
// holds the writer/advisory locks, so the snapshot is consistent.
func (c *worktreeCutover) loadLegacy(tx *sqlx.Tx, legacyEnvColumns, legacyRepoColumns map[string]bool) error {
	if err := c.loadLegacyEnvs(tx, legacyEnvColumns); err != nil {
		return err
	}
	if err := c.loadLegacySessions(tx); err != nil {
		return err
	}
	if err := c.loadLegacyEnvRepos(tx, legacyRepoColumns); err != nil {
		return err
	}
	return c.loadLegacySessionWorktrees(tx)
}

func (c *worktreeCutover) loadLegacyEnvs(tx *sqlx.Tx, columns map[string]bool) error {
	// Every interpolated expression comes from the fixed deprecated-column allowlist.
	//nolint:gosec
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT id, task_id, %s, executor_type, executor_id,
			executor_profile_id, control_port, status,
			%s, %s, %s, COALESCE(workspace_path, ''),
			COALESCE(container_id, ''), %s, %s, COALESCE(sandbox_id, ''),
			COALESCE(task_dir_name, ''), created_at, updated_at
		FROM task_environments`,
		legacyEnvColumnExpr(columns, "repository_id"),
		legacyEnvColumnExpr(columns, "worktree_id"),
		legacyEnvColumnExpr(columns, "worktree_path"),
		legacyEnvColumnExpr(columns, "worktree_branch"),
		legacyEnvColumnExpr(columns, "container_bootstrap_nonce_secret_id"),
		legacyEnvColumnExpr(columns, "container_control_auth_token_secret_id")))
	if err != nil {
		return fmt.Errorf("cutover: read legacy environments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		env := &legacyEnv{}
		if err := rows.Scan(&env.id, &env.taskID, &env.repositoryID, &env.executorType,
			&env.executorID, &env.executorProfileID, &env.controlPort, &env.status,
			&env.worktreeID, &env.worktreePath, &env.worktreeBranch, &env.workspacePath,
			&env.containerID, &env.containerBootstrapNonceSecretID, &env.containerControlAuthTokenSecretID,
			&env.sandboxID, &env.taskDirName, &env.createdAt, &env.updatedAt); err != nil {
			return fmt.Errorf("cutover: scan legacy environment: %w", err)
		}
		c.envs[env.id] = env
		c.taskEnvs[env.taskID] = append(c.taskEnvs[env.taskID], env)
	}
	return rows.Err()
}

func legacyEnvColumnExpr(columns map[string]bool, column string) string {
	if columns[column] {
		return "COALESCE(" + column + ", '')"
	}
	return "CAST('' AS TEXT)"
}

func (c *worktreeCutover) loadLegacySessions(tx *sqlx.Tx) error {
	// GROUP BY lists every non-aggregate column so PostgreSQL accepts the
	// query (SQLite is lenient about functional dependency). started_at is
	// scanned as a string because MIN() makes the driver return the raw
	// TEXT value instead of a time.
	rows, err := tx.Query(`
		SELECT ts.id, ts.task_id, COALESCE(ts.task_environment_id, ''), ts.state,
			COALESCE(ts.executor_id, ''), COALESCE(ts.executor_profile_id, ''),
			COALESCE(ts.repository_id, ''), COALESCE(er.container_id, ''),
			MIN(ts.started_at)
		FROM task_sessions ts
		LEFT JOIN executors_running er ON er.session_id = ts.id
		GROUP BY ts.id, ts.task_id, ts.task_environment_id, ts.state, ts.executor_id,
			ts.executor_profile_id, ts.repository_id, er.container_id`)
	if err != nil {
		return fmt.Errorf("cutover: read legacy sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		s := &legacySession{}
		var startedAt string
		if err := rows.Scan(&s.id, &s.taskID, &s.taskEnvironmentID, &s.state, &s.executorID,
			&s.executorProfileID, &s.repositoryID, &s.containerID, &startedAt); err != nil {
			return fmt.Errorf("cutover: scan legacy session: %w", err)
		}
		s.startedAt = parseLegacyTimestamp(startedAt)
		c.sessions[s.id] = s
		c.sessionTasks[s.id] = s.taskID
		c.sessionEnvIDs[s.id] = s.taskEnvironmentID
	}
	return rows.Err()
}

// parseLegacyTimestamp converts a stored timestamp text into a time.Time.
// SQLite returns raw TEXT for aggregate expressions; the application writes
// RFC3339Nano values. Unknown formats fall back to the zero time so the
// backfilled environment still carries a usable (if conservative) lifecycle.
func parseLegacyTimestamp(raw string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func (c *worktreeCutover) loadLegacyEnvRepos(tx *sqlx.Tx, columns map[string]bool) error {
	// Every interpolated expression comes from the fixed lifecycle-column allowlist.
	//nolint:gosec
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT id, task_environment_id, repository_id, COALESCE(branch_slug, ''),
			COALESCE(worktree_id, ''), COALESCE(worktree_path, ''),
			COALESCE(worktree_branch, ''), position, COALESCE(error_message, ''),
			%s, created_at, updated_at, %s, %s
		FROM task_environment_repos`,
		legacyRepoColumnExpr(columns, "status", "CAST('active' AS TEXT)"),
		legacyRepoColumnExpr(columns, "merged_at", "NULL"),
		legacyRepoColumnExpr(columns, "deleted_at", "NULL")))
	if err != nil {
		return fmt.Errorf("cutover: read legacy environment repos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row legacyEnvRepo
		var mergedAt, deletedAt sql.NullTime
		if err := rows.Scan(&row.id, &row.envID, &row.repositoryID, &row.branchSlug,
			&row.worktreeID, &row.worktreePath, &row.worktreeBranch, &row.position,
			&row.errorMessage, &row.status, &row.createdAt, &row.updatedAt, &mergedAt, &deletedAt); err != nil {
			return fmt.Errorf("cutover: scan legacy environment repo: %w", err)
		}
		if mergedAt.Valid {
			t := mergedAt.Time
			row.mergedAt = &t
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			row.deletedAt = &t
		}
		c.envRepos = append(c.envRepos, row)
	}
	return rows.Err()
}

func legacyRepoColumnExpr(columns map[string]bool, column, fallback string) string {
	if columns[column] {
		return column
	}
	return fallback
}

func (c *worktreeCutover) loadLegacySessionWorktrees(tx *sqlx.Tx) error {
	rows, err := tx.Query(`
		SELECT session_id, worktree_id, repository_id, COALESCE(branch_slug, ''),
			position, COALESCE(worktree_path, ''), COALESCE(worktree_branch, ''),
			status, created_at, updated_at, merged_at, deleted_at
		FROM task_session_worktrees`)
	if err != nil {
		return fmt.Errorf("cutover: read legacy session worktrees: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row legacySessionWorktree
		var mergedAt, deletedAt sql.NullTime
		if err := rows.Scan(&row.sessionID, &row.worktreeID, &row.repositoryID, &row.branchSlug,
			&row.position, &row.worktreePath, &row.worktreeBranch, &row.status,
			&row.createdAt, &row.updatedAt, &mergedAt, &deletedAt); err != nil {
			return fmt.Errorf("cutover: scan legacy session worktree: %w", err)
		}
		if mergedAt.Valid {
			t := mergedAt.Time
			row.mergedAt = &t
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			row.deletedAt = &t
		}
		if _, ok := c.sessions[row.sessionID]; !ok {
			c.orphanedSessionWts = append(c.orphanedSessionWts, row)
			continue
		}
		c.sessionWts = append(c.sessionWts, row)
	}
	return rows.Err()
}

// taskWorktreeTargets accumulates the normalized environment-repository rows
// for one task. Keyed by repositoryID+"\x00"+branchSlug.
type taskWorktreeTargets struct {
	envID    string
	byKey    map[string]*envRepoTarget
	byWtID   map[string]*envRepoTarget
	ordering []string
}

type envRepoTarget struct {
	key                      string
	repositoryID, branchSlug string
	canonical                bool
	worktreeID               string
	worktreePath             string
	worktreeBranch           string
	position                 int
	errorMessage             string
	status                   string
	mergedAt, deletedAt      *time.Time
	createdAt, updatedAt     time.Time
}

// normalize builds the final ownership graph in memory: one environment per
// task (deduplicated, backfilled when missing), one repository row per
// (repository, branch slug), and a session-to-environment link for every
// session. Any conflicting identity, path, or branch fails the migration.
func (c *worktreeCutover) normalize(tx *sqlx.Tx) error {
	c.pickSurvivingEnvironments()
	c.mergeLegacyEnvRepoRows()
	c.mergeFlatEnvironmentFields()
	c.classifyOrphanedSessionWorktrees()
	if err := c.backfillMissingEnvironments(tx); err != nil {
		return err
	}
	// Sessions are linked before session worktrees are merged: a session's
	// worktree belongs to the environment the session resolves to, which is
	// not always an environment owned by the session's own task.
	c.linkSessions()
	c.electSlotWorktreeOwners()
	c.mergeSessionWorktrees()
	c.registerWorktreeIdentities()
	c.bindTargetsToEnvironments()

	if len(c.conflicts) > 0 {
		return fmt.Errorf("cutover: %d conflicting legacy ownership row(s):\n- %s",
			len(c.conflicts), strings.Join(c.conflicts, "\n- "))
	}
	return nil
}

// classifyOrphanedSessionWorktrees rejects a legacy row whose missing session
// leaves no task or environment owner to recover. Deleted rows and rows whose
// physical identity already has a higher-precedence task-owned source are
// historical evidence and contribute no ownership or metadata.
func (c *worktreeCutover) classifyOrphanedSessionWorktrees() {
	for _, wt := range c.orphanedSessionWts {
		if isLegacyDeletedWorktree(wt) || c.hasTaskOwnedSourceForWorktree(wt.worktreeID) {
			continue
		}
		c.conflicts = append(c.conflicts, fmt.Sprintf(
			"task_session_worktrees row %s references missing session %s", wt.worktreeID, wt.sessionID))
	}
}

// hasTaskOwnedSourceForWorktree reports whether an exact physical identity is
// represented by a non-deleted normalized target built from a canonical row
// or surviving flat environment. Path, branch, and repository metadata cannot
// recover the task relationship lost with the session.
func (c *worktreeCutover) hasTaskOwnedSourceForWorktree(worktreeID string) bool {
	if worktreeID == "" {
		return false
	}
	for _, targets := range c.tasks {
		for _, key := range targets.ordering {
			target := targets.byKey[key]
			if target.worktreeID == worktreeID && target.status != worktreeRepoStatusDeleted &&
				target.deletedAt == nil {
				return true
			}
		}
	}
	return false
}

// pickSurvivingEnvironments keeps the most recently updated environment per
// task and re-points sessions that referenced a collapsed row.
func (c *worktreeCutover) pickSurvivingEnvironments() {
	for taskID, envs := range c.taskEnvs {
		if len(envs) == 0 {
			continue
		}
		sort.SliceStable(envs, func(i, j int) bool {
			if !envs[i].updatedAt.Equal(envs[j].updatedAt) {
				return envs[i].updatedAt.After(envs[j].updatedAt)
			}
			return envs[i].createdAt.After(envs[j].createdAt)
		})
		winner := envs[0]
		c.taskEnvIDs[taskID] = winner.id
		for _, loser := range envs[1:] {
			c.loserEnvIDs[loser.id] = true
			// Sessions pointing at a loser re-point to the winner.
			for _, s := range c.sessions {
				if s.taskEnvironmentID == loser.id {
					c.sessionEnvIDs[s.id] = winner.id
				}
			}
		}
	}
}

// mergeLegacyEnvRepoRows folds every pre-existing environment-repository row
// into its task's targets. Loser env rows re-home their repositories to the
// winner.
func (c *worktreeCutover) mergeLegacyEnvRepoRows() {
	for _, row := range c.envRepos {
		env, ok := c.envs[row.envID]
		if !ok {
			c.conflicts = append(c.conflicts, fmt.Sprintf(
				"task_environment_repos row %s references missing environment %s", row.id, row.envID))
			continue
		}
		targets := c.targetsForTask(env.taskID)
		if err := targets.mergeLegacyEnvRepo(row); err != nil {
			c.conflicts = append(c.conflicts, fmt.Sprintf("environment %s repository row: %v", row.envID, err))
			continue
		}
		c.recordAuthoritativeWorktree(env.taskID, row.worktreeID)
	}
}

// mergeFlatEnvironmentFields merges the deprecated flat worktree columns of
// the surviving environments.
func (c *worktreeCutover) mergeFlatEnvironmentFields() {
	for _, env := range c.envs {
		if c.taskEnvIDs[env.taskID] != env.id || (env.repositoryID == "" && env.worktreeID == "") {
			continue
		}
		targets := c.targetsForTask(env.taskID)
		if canonical := targets.canonicalOwnerForSlot(env.repositoryID, ""); canonical != nil && env.worktreeID != "" && env.worktreeID != canonical.worktreeID {
			c.demoteFlatEnvironment(env, canonical)
			continue
		}
		if err := targets.mergeFlatEnv(env); err != nil {
			c.conflicts = append(c.conflicts, fmt.Sprintf("environment %s flat worktree fields: %v", env.id, err))
			continue
		}
		c.recordAuthoritativeWorktree(env.taskID, env.worktreeID)
	}
}

// demoteFlatEnvironment records deprecated flat metadata that lost the exact
// empty-branch slot to a non-deleted canonical repository row. The migration
// keeps the physical flat worktree untouched and logs it after commit.
func (c *worktreeCutover) demoteFlatEnvironment(env *legacyEnv, winner *envRepoTarget) {
	if c.demotedFlatEnvironments[env.id] {
		return
	}
	c.demotedFlatEnvironments[env.id] = true
	c.demotions = append(c.demotions, fmt.Sprintf(
		"environment %s flat worktree %s (repository %s, path %q, branch %q) superseded by canonical worktree %s",
		env.id, env.worktreeID, env.repositoryID, env.worktreePath, env.worktreeBranch, winner.worktreeID))
}

// worktreeSlot is one normalized repository slot — a (repository, branch
// slug) pair of a single task — plus the legacy session rows competing to own
// it.
type worktreeSlot struct {
	targets *taskWorktreeTargets
	key     string
	rows    []legacySessionWorktree
}

// electSlotWorktreeOwners resolves the one legacy shape the task-owned model
// cannot represent: several distinct physical worktrees claiming the same
// (repository, branch slug) slot of one task. The per-session model produced
// these routinely — a handoff, a re-materialized workspace, or a second
// session each minted their own worktree for the same repository — so failing
// the upgrade would strand those databases on the previous binary.
//
// A canonical repository row or the surviving flat environment owns the slot
// outright; otherwise the newest live session worktree wins. Every other row
// is demoted to historical evidence: the migration touches no filesystem, so
// the demoted directory and branch stay on disk, but the normalized schema
// records exactly one owner per slot.
func (c *worktreeCutover) electSlotWorktreeOwners() {
	slots, order := c.collectSlotContenders()
	for _, id := range order {
		slot := slots[id]
		winner := slot.currentOwner()
		if winner == "" {
			winner = c.bestSlotOwner(slot.rows)
		}
		for _, row := range slot.rows {
			if row.worktreeID == winner {
				continue
			}
			c.demoteSessionWorktree(row, winner)
		}
	}
}

// collectSlotContenders groups every legacy session worktree that still needs
// a home by the slot it would claim, preserving row order so the election is
// deterministic.
func (c *worktreeCutover) collectSlotContenders() (map[string]*worktreeSlot, []string) {
	slots := make(map[string]*worktreeSlot)
	var order []string
	for _, wt := range c.sessionWts {
		if wt.worktreeID == "" || isLegacyDeletedWorktree(wt) {
			continue
		}
		taskID := c.sessionOwnerTaskID(wt.sessionID)
		targets := c.targetsForTask(taskID)
		if targets.targetForWorktree(wt.worktreeID) != nil {
			continue // a canonical source already owns this physical worktree
		}
		key := envRepoKey(wt.repositoryID, wt.branchSlug)
		id := taskID + "\x00" + key
		slot, ok := slots[id]
		if !ok {
			slot = &worktreeSlot{targets: targets, key: key}
			slots[id] = slot
			order = append(order, id)
		}
		slot.rows = append(slot.rows, wt)
	}
	return slots, order
}

// currentOwner returns the physical worktree a canonical source already
// homed on this slot, or "" when the slot is still unclaimed.
func (s *worktreeSlot) currentOwner() string {
	target, ok := s.targets.byKey[s.key]
	if !ok {
		return ""
	}
	return target.worktreeID
}

// bestSlotOwner picks the worktree that keeps an otherwise unclaimed slot.
func (c *worktreeCutover) bestSlotOwner(rows []legacySessionWorktree) string {
	best := rows[0]
	for _, row := range rows[1:] {
		if c.outranksSlotOwner(row, best) {
			best = row
		}
	}
	return best.worktreeID
}

// outranksSlotOwner orders two competing rows: a live session's worktree
// beats a terminal session's, then the most recently updated row wins, with
// creation time and identity as stable tie-breakers.
func (c *worktreeCutover) outranksSlotOwner(candidate, best legacySessionWorktree) bool {
	candidateHistorical := c.isHistoricalSessionRow(candidate)
	bestHistorical := c.isHistoricalSessionRow(best)
	if candidateHistorical != bestHistorical {
		return bestHistorical
	}
	if !candidate.updatedAt.Equal(best.updatedAt) {
		return candidate.updatedAt.After(best.updatedAt)
	}
	if !candidate.createdAt.Equal(best.createdAt) {
		return candidate.createdAt.After(best.createdAt)
	}
	return candidate.worktreeID > best.worktreeID
}

// isHistoricalSessionRow reports whether the row's session is terminal (or
// gone), which makes its worktree the weaker claim on a contested slot.
func (c *worktreeCutover) isHistoricalSessionRow(wt legacySessionWorktree) bool {
	session, ok := c.sessions[wt.sessionID]
	return !ok || isLegacyHistoricalSession(session.state)
}

// demoteSessionWorktree records a legacy row as historical evidence for a
// slot another worktree owns, and keeps a human-readable line so the losing
// directory can still be found on disk.
func (c *worktreeCutover) demoteSessionWorktree(wt legacySessionWorktree, winner string) {
	key := sessionWorktreeKey(wt.sessionID, wt.worktreeID)
	if c.demotedWorktrees[key] {
		return
	}
	c.demotedWorktrees[key] = true
	c.demotions = append(c.demotions, fmt.Sprintf(
		"session %s worktree %s (repository %s, path %q, branch %q) superseded by worktree %s",
		wt.sessionID, wt.worktreeID, wt.repositoryID, wt.worktreePath, wt.worktreeBranch, winner))
}

// mergeSessionWorktrees folds every legacy session-worktree row into the
// targets of the task that owns the session's environment.
func (c *worktreeCutover) mergeSessionWorktrees() {
	for _, wt := range c.sessionWts {
		if _, ok := c.sessions[wt.sessionID]; !ok {
			continue // already reported in load
		}
		if c.demotedWorktrees[sessionWorktreeKey(wt.sessionID, wt.worktreeID)] {
			continue // another physical worktree owns this repository slot
		}
		targets := c.targetsForTask(c.sessionOwnerTaskID(wt.sessionID))
		if err := targets.mergeSessionWorktree(wt, c.isSupersededSessionWorktree(wt)); err != nil {
			c.conflicts = append(c.conflicts, fmt.Sprintf(
				"session %s worktree %s: %v", wt.sessionID, wt.worktreeID, err))
		}
	}
}

func isLegacyDeletedWorktree(wt legacySessionWorktree) bool {
	return wt.status == worktreeRepoStatusDeleted || wt.deletedAt != nil
}

// isLegacyHistoricalSession reports whether a session no longer represents an
// open execution that can own a conflicting worktree during cutover.
func isLegacyHistoricalSession(state string) bool {
	switch state {
	case sessionStateCompleted, sessionStateFailed, sessionStateCancelled:
		return true
	default:
		return false
	}
}

// bindTargetsToEnvironments binds every task's targets to its surviving
// environment.
func (c *worktreeCutover) bindTargetsToEnvironments() {
	for taskID, targets := range c.tasks {
		if len(targets.ordering) == 0 {
			continue // nothing to home
		}
		envID := c.taskEnvIDs[taskID]
		if envID == "" {
			c.conflicts = append(c.conflicts, fmt.Sprintf(
				"task %s has session worktrees but no resolvable environment", taskID))
			continue
		}
		targets.envID = envID
	}
}

func (c *worktreeCutover) targetsForTask(taskID string) *taskWorktreeTargets {
	targets, ok := c.tasks[taskID]
	if !ok {
		targets = &taskWorktreeTargets{
			byKey:  make(map[string]*envRepoTarget),
			byWtID: make(map[string]*envRepoTarget),
		}
		c.tasks[taskID] = targets
	}
	return targets
}

// backfillMissingEnvironments creates a normalized environment for every task
// that has sessions but no environment, mirroring the legacy startup backfill
// (now folded into this one-time cutover). Sessions that borrow a surviving
// environment from another task are skipped: fabricating an environment for
// the borrowing task would duplicate the shared workspace.
func (c *worktreeCutover) backfillMissingEnvironments(tx *sqlx.Tx) error {
	tasksWithSessions := make(map[string]*legacySession)
	for _, s := range c.sessions {
		if _, ok := c.taskEnvIDs[s.taskID]; ok {
			continue
		}
		if c.hasSurvivingEnvironmentRef(s.id) {
			continue
		}
		if _, seen := tasksWithSessions[s.taskID]; !seen {
			tasksWithSessions[s.taskID] = s
		}
	}
	for taskID, sample := range tasksWithSessions {
		executorType, err := c.resolveExecutorType(tx, sample.executorID)
		if err != nil {
			return err
		}
		envID := uuid.New().String()
		c.taskEnvIDs[taskID] = envID
		c.envs[envID] = &legacyEnv{
			id: envID, taskID: taskID,
			executorType:      executorType,
			executorID:        sample.executorID,
			executorProfileID: sample.executorProfileID,
			status:            taskEnvironmentStatusStopped,
			containerID:       sample.containerID,
			createdAt:         sample.startedAt,
			updatedAt:         sample.startedAt,
		}
		for _, s := range c.sessions {
			if s.taskID == taskID && c.sessionEnvIDs[s.id] == "" {
				c.sessionEnvIDs[s.id] = envID
			}
		}
	}
	return nil
}

// resolveExecutorType looks up a session's executor type. A missing executor
// row means a legacy session whose executor was deleted: default to
// "local_pc" exactly like the legacy backfill. Any other error aborts the
// cutover instead of silently guessing.
func (c *worktreeCutover) resolveExecutorType(tx *sqlx.Tx, executorID string) (string, error) {
	if executorID == "" {
		return executorTypeLocalPC, nil
	}
	if cached, ok := c.executorTypes[executorID]; ok {
		return cached, nil
	}
	var executorType string
	err := tx.QueryRow(tx.Rebind(`SELECT type FROM executors WHERE id = ?`), executorID).Scan(&executorType)
	if errors.Is(err, sql.ErrNoRows) {
		c.executorTypes[executorID] = executorTypeLocalPC
		return executorTypeLocalPC, nil
	}
	if err != nil {
		return "", fmt.Errorf("cutover: lookup executor type for %s: %w", executorID, err)
	}
	c.executorTypes[executorID] = executorType
	return executorType, nil
}

// linkSessions records the final environment for every session: its own
// task's surviving environment unless it retains a valid reference (including
// borrowed environments owned by another task).
func (c *worktreeCutover) linkSessions() {
	for _, s := range c.sessions {
		current := c.sessionEnvIDs[s.id]
		if current != "" {
			if c.loserEnvIDs[current] {
				current = ""
			}
			if _, ok := c.envs[current]; ok && current != "" {
				continue // valid surviving reference
			}
		}
		c.sessionEnvIDs[s.id] = c.taskEnvIDs[s.taskID]
	}
}

// isSupersededSessionWorktree reports whether a legacy session row is no
// longer an ownership source. Rows demoted by the slot election, repository
// rows, and the surviving flat environment take precedence for the same
// physical identity regardless of session state. Historical rows with no
// authoritative replacement remain eligible for backfill.
func (c *worktreeCutover) isSupersededSessionWorktree(wt legacySessionWorktree) bool {
	cacheKey := sessionWorktreeKey(wt.sessionID, wt.worktreeID)
	if c.demotedWorktrees[cacheKey] {
		return true
	}
	if superseded, ok := c.sessionWorktreeSuperseded[cacheKey]; ok {
		return superseded
	}

	session, hasSession := c.sessions[wt.sessionID]
	ownerTaskID := c.sessionOwnerTaskID(wt.sessionID)
	superseded := false
	switch {
	case isLegacyDeletedWorktree(wt):
		superseded = true
	case hasSession && c.authoritativeWorktreeIDs[authoritativeWorktreeKey(ownerTaskID, wt.worktreeID)]:
		superseded = true
	case hasSession && isLegacyHistoricalSession(session.state):
		superseded = c.flatEnvironmentSupersedesSessionWorktree(wt, session, ownerTaskID)
		if !superseded {
			for _, row := range c.envRepos {
				env, ok := c.envs[row.envID]
				if !ok || env.taskID != ownerTaskID || row.worktreeID == "" {
					continue
				}
				if row.repositoryID == wt.repositoryID && row.branchSlug == wt.branchSlug {
					superseded = true
					break
				}
			}
		}
	}
	c.sessionWorktreeSuperseded[cacheKey] = superseded
	return superseded
}

// flatEnvironmentSupersedesSessionWorktree reports whether a surviving flat
// environment owns the session's repository slot. Terminal history may carry
// an older physical identity, but only for the task that owns the session's
// environment and the legacy empty branch slot.
func (c *worktreeCutover) flatEnvironmentSupersedesSessionWorktree(
	wt legacySessionWorktree, session *legacySession, ownerTaskID string,
) bool {
	if !isLegacyHistoricalSession(session.state) || wt.branchSlug != "" {
		return false
	}
	envID, ok := c.taskEnvIDs[ownerTaskID]
	if !ok {
		return false
	}
	env, ok := c.envs[envID]
	return ok && env.taskID == ownerTaskID && env.repositoryID == wt.repositoryID && env.worktreeID != ""
}

// sessionOwnerTaskID returns the task that owns the environment a session
// resolves to. A session does not always run in an environment owned by its
// own task: workspace-group members, subtasks inheriting their parent's
// workspace, and tasks that received a shared environment through ownership
// handoff all borrow another task's environment. The physical worktree such a
// session used belongs to that environment, so its normalized repository row
// must be homed on the owning task — homing it on the borrowing task would
// give one worktree two owners.
//
// Only valid after linkSessions has settled every session's environment.
func (c *worktreeCutover) sessionOwnerTaskID(sessionID string) string {
	if env, ok := c.envs[c.sessionEnvIDs[sessionID]]; ok && env.taskID != "" {
		return env.taskID
	}
	return c.sessionTasks[sessionID]
}

// hasSurvivingEnvironmentRef reports whether a session already points at an
// environment that survives the cutover (its own task's or a borrowed one).
func (c *worktreeCutover) hasSurvivingEnvironmentRef(sessionID string) bool {
	envID := c.sessionEnvIDs[sessionID]
	if envID == "" || c.loserEnvIDs[envID] {
		return false
	}
	_, ok := c.envs[envID]
	return ok
}

func (c *worktreeCutover) recordAuthoritativeWorktree(taskID, worktreeID string) {
	if worktreeID == "" {
		return
	}
	c.authoritativeWorktreeIDs[authoritativeWorktreeKey(taskID, worktreeID)] = true
}

func authoritativeWorktreeKey(taskID, worktreeID string) string {
	return taskID + "\x00" + worktreeID
}

// sessionWorktreeKey identifies one legacy session-worktree row.
func sessionWorktreeKey(sessionID, worktreeID string) string {
	return sessionID + "\x00" + worktreeID
}

// registerWorktreeIdentities ensures every physical worktree is owned by
// exactly one environment-repository key.
func (c *worktreeCutover) registerWorktreeIdentities() {
	for _, targets := range c.tasks {
		for _, key := range targets.ordering {
			target := targets.byKey[key]
			if target.worktreeID == "" {
				continue
			}
			if existing, ok := targets.byWtID[target.worktreeID]; ok && existing != target {
				c.conflicts = append(c.conflicts, fmt.Sprintf(
					"worktree %s claimed by repository slots %q and %q",
					target.worktreeID, existing.key, target.key))
				continue
			}
			targets.byWtID[target.worktreeID] = target
		}
	}
}
