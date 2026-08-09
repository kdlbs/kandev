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

// worktreeCutover holds the legacy inventory and the normalized result.
type worktreeCutover struct {
	envs                         map[string]*legacyEnv
	taskEnvs                     map[string][]*legacyEnv
	sessions                     map[string]*legacySession
	sessionTasks                 map[string]string
	sessionEnvIDs                map[string]string
	historicalWorktreeSuperseded map[string]bool
	envRepos                     []legacyEnvRepo
	sessionWts                   []legacySessionWorktree
	tasks                        map[string]*taskWorktreeTargets
	taskEnvIDs                   map[string]string
	loserEnvIDs                  map[string]bool
	executorTypes                map[string]string
	conflicts                    []string
}

type legacyEnv struct {
	id, taskID, repositoryID, executorType, executorID, executorProfileID string
	controlPort                                                           int
	status, worktreeID, worktreePath, worktreeBranch, workspacePath       string
	containerID, sandboxID, taskDirName                                   string
	createdAt, updatedAt                                                  time.Time
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
	errorMessage                             string
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
func (c *worktreeCutover) loadLegacy(tx *sqlx.Tx) error {
	if err := c.loadLegacyEnvs(tx); err != nil {
		return err
	}
	if err := c.loadLegacySessions(tx); err != nil {
		return err
	}
	if err := c.loadLegacyEnvRepos(tx); err != nil {
		return err
	}
	return c.loadLegacySessionWorktrees(tx)
}

func (c *worktreeCutover) loadLegacyEnvs(tx *sqlx.Tx) error {
	rows, err := tx.Query(`
		SELECT id, task_id, repository_id, executor_type, executor_id,
			executor_profile_id, control_port, status,
			COALESCE(worktree_id, ''), COALESCE(worktree_path, ''),
			COALESCE(worktree_branch, ''), COALESCE(workspace_path, ''),
			COALESCE(container_id, ''), COALESCE(sandbox_id, ''),
			COALESCE(task_dir_name, ''), created_at, updated_at
		FROM task_environments`)
	if err != nil {
		return fmt.Errorf("cutover: read legacy environments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		env := &legacyEnv{}
		if err := rows.Scan(&env.id, &env.taskID, &env.repositoryID, &env.executorType,
			&env.executorID, &env.executorProfileID, &env.controlPort, &env.status,
			&env.worktreeID, &env.worktreePath, &env.worktreeBranch, &env.workspacePath,
			&env.containerID, &env.sandboxID, &env.taskDirName, &env.createdAt, &env.updatedAt); err != nil {
			return fmt.Errorf("cutover: scan legacy environment: %w", err)
		}
		c.envs[env.id] = env
		c.taskEnvs[env.taskID] = append(c.taskEnvs[env.taskID], env)
	}
	return rows.Err()
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

func (c *worktreeCutover) loadLegacyEnvRepos(tx *sqlx.Tx) error {
	rows, err := tx.Query(`
		SELECT id, task_environment_id, repository_id, COALESCE(branch_slug, ''),
			COALESCE(worktree_id, ''), COALESCE(worktree_path, ''),
			COALESCE(worktree_branch, ''), position, COALESCE(error_message, ''),
			created_at, updated_at
		FROM task_environment_repos`)
	if err != nil {
		return fmt.Errorf("cutover: read legacy environment repos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row legacyEnvRepo
		if err := rows.Scan(&row.id, &row.envID, &row.repositoryID, &row.branchSlug,
			&row.worktreeID, &row.worktreePath, &row.worktreeBranch, &row.position,
			&row.errorMessage, &row.createdAt, &row.updatedAt); err != nil {
			return fmt.Errorf("cutover: scan legacy environment repo: %w", err)
		}
		c.envRepos = append(c.envRepos, row)
	}
	return rows.Err()
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
			c.conflicts = append(c.conflicts, fmt.Sprintf(
				"task_session_worktrees row %s references missing session %s", row.worktreeID, row.sessionID))
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
	if err := c.backfillMissingEnvironments(tx); err != nil {
		return err
	}
	c.mergeSessionWorktrees()
	c.registerWorktreeIdentities()
	c.bindTargetsToEnvironments()
	c.linkSessions()

	if len(c.conflicts) > 0 {
		return fmt.Errorf("cutover: %d conflicting legacy ownership row(s):\n- %s",
			len(c.conflicts), strings.Join(c.conflicts, "\n- "))
	}
	return nil
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
		}
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
		if err := targets.mergeFlatEnv(env); err != nil {
			c.conflicts = append(c.conflicts, fmt.Sprintf("environment %s flat worktree fields: %v", env.id, err))
		}
	}
}

// mergeSessionWorktrees folds every legacy session-worktree row into the
// session's task targets.
func (c *worktreeCutover) mergeSessionWorktrees() {
	for _, wt := range c.sessionWts {
		session, ok := c.sessions[wt.sessionID]
		if !ok {
			continue // already reported in load
		}
		targets := c.targetsForTask(session.taskID)
		if err := targets.mergeSessionWorktree(wt, c.isSupersededHistoricalWorktree(wt)); err != nil {
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
	case "COMPLETED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}

// bindTargetsToEnvironments binds every task's targets to its surviving
// environment.
func (c *worktreeCutover) bindTargetsToEnvironments() {
	for taskID, targets := range c.tasks {
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
// (now folded into this one-time cutover).
func (c *worktreeCutover) backfillMissingEnvironments(tx *sqlx.Tx) error {
	tasksWithSessions := make(map[string]*legacySession)
	for _, s := range c.sessions {
		if _, ok := c.taskEnvIDs[s.taskID]; !ok {
			if _, seen := tasksWithSessions[s.taskID]; !seen {
				tasksWithSessions[s.taskID] = s
			}
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

// isSupersededHistoricalWorktree reports whether a legacy row is no longer an
// ownership source because a canonical repository row already represents the
// same task/repository slot. A historical row with no canonical replacement
// remains eligible for backfill.
func (c *worktreeCutover) isSupersededHistoricalWorktree(wt legacySessionWorktree) bool {
	cacheKey := wt.sessionID + "\x00" + wt.worktreeID
	if superseded, ok := c.historicalWorktreeSuperseded[cacheKey]; ok {
		return superseded
	}

	superseded := false
	if isLegacyDeletedWorktree(wt) {
		superseded = true
	} else if session, ok := c.sessions[wt.sessionID]; ok && isLegacyHistoricalSession(session.state) {
		for _, row := range c.envRepos {
			env, ok := c.envs[row.envID]
			if !ok || env.taskID != session.taskID || row.worktreeID == "" {
				continue
			}
			if row.repositoryID == wt.repositoryID && row.branchSlug == wt.branchSlug {
				superseded = true
				break
			}
		}
	}
	c.historicalWorktreeSuperseded[cacheKey] = superseded
	return superseded
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
