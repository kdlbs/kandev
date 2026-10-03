package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("agent project not found")

// Store persists project records in the task database. The task schema owns
// the shared agent_projects table and task membership foreign key.
type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateOrGet(ctx context.Context, project *Project, requestKey string) (*Project, bool, error) {
	encodedRepos, err := json.Marshal(project.RepositoryIDs)
	if err != nil {
		return nil, false, fmt.Errorf("encode project repositories: %w", err)
	}
	query := s.db.Rebind(`
		INSERT INTO agent_projects (
			id, workspace_id, name, repository_ids, primary_repository_id,
			coordinator_profile_id, economy_profile_id, frontier_profile_id,
			executor_profile_id, request_key, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT (workspace_id, request_key) WHERE request_key != '' DO NOTHING
	`)
	if requestKey == "" {
		query = s.db.Rebind(`
			INSERT INTO agent_projects (
				id, workspace_id, name, repository_ids, primary_repository_id,
				coordinator_profile_id, economy_profile_id, frontier_profile_id,
				executor_profile_id, request_key, revision, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`)
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, query,
		project.ID, project.WorkspaceID, project.Name, string(encodedRepos), project.PrimaryRepositoryID,
		project.CoordinatorProfileID, project.EconomyProfileID, project.FrontierProfileID,
		project.ExecutorProfileID, requestKey, now, now,
	); err != nil {
		return nil, false, fmt.Errorf("insert agent project: %w", err)
	}
	if requestKey != "" {
		stored, err := s.GetByRequestKey(ctx, project.WorkspaceID, requestKey)
		if err != nil {
			return nil, false, err
		}
		return stored, stored.ID != project.ID, nil
	}
	stored, err := s.Get(ctx, project.WorkspaceID, project.ID)
	return stored, false, err
}

func (s *Store) Get(ctx context.Context, workspaceID, id string) (*Project, error) {
	row := s.db.QueryRowxContext(ctx, s.db.Rebind(projectSelect+` WHERE workspace_id = ? AND id = ?`), workspaceID, id)
	project, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return project, err
}

func (s *Store) GetByRequestKey(ctx context.Context, workspaceID, requestKey string) (*Project, error) {
	row := s.db.QueryRowxContext(ctx, s.db.Rebind(projectSelect+` WHERE workspace_id = ? AND request_key = ?`), workspaceID, requestKey)
	project, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return project, err
}

func (s *Store) List(ctx context.Context, workspaceID string, archived bool) ([]*Project, error) {
	archiveFilter := `archived_at IS NULL`
	if archived {
		archiveFilter = `archived_at IS NOT NULL`
	}
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(projectSelect+` WHERE workspace_id = ? AND `+archiveFilter+` AND main_task_id <> '' ORDER BY updated_at DESC, id`), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list agent projects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	projects := make([]*Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read agent projects: %w", err)
	}
	return projects, nil
}

func (s *Store) Update(ctx context.Context, project *Project, expectedRevision int64) error {
	encodedRepos, err := json.Marshal(project.RepositoryIDs)
	if err != nil {
		return fmt.Errorf("encode project repositories: %w", err)
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE agent_projects
		SET name = ?, repository_ids = ?, primary_repository_id = ?,
			coordinator_profile_id = ?, economy_profile_id = ?, frontier_profile_id = ?,
			executor_profile_id = ?,
			revision = revision + 1, updated_at = ?
		WHERE id = ? AND workspace_id = ? AND revision = ? AND archived_at IS NULL
	`), project.Name, string(encodedRepos), project.PrimaryRepositoryID,
		project.CoordinatorProfileID, project.EconomyProfileID, project.FrontierProfileID, project.ExecutorProfileID,
		time.Now().UTC(), project.ID, project.WorkspaceID, expectedRevision)
	if err != nil {
		return fmt.Errorf("update agent project: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRevisionConflict
	}
	return nil
}

func (s *Store) SetMainTaskID(ctx context.Context, workspaceID, projectID, taskID string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE agent_projects SET main_task_id = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND (main_task_id = '' OR main_task_id = ?)
	`), taskID, time.Now().UTC(), workspaceID, projectID, taskID)
	if err != nil {
		return fmt.Errorf("set project coordinator: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrCoordinatorConflict
	}
	return nil
}

func (s *Store) FindCoordinatorTaskID(ctx context.Context, projectID string) (string, error) {
	var taskID string
	err := s.db.GetContext(ctx, &taskID, s.db.Rebind(`
		SELECT id FROM tasks
		WHERE agent_project_id = ? AND COALESCE(parent_id, '') = ''
		ORDER BY created_at, id LIMIT 1
	`), projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return taskID, err
}

func (s *Store) ListTasks(ctx context.Context, projectID string) ([]TaskSummary, error) {
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(`
		SELECT id, title, COALESCE(state, ''), COALESCE(parent_id, ''), archived_at, updated_at
		FROM tasks WHERE agent_project_id = ? ORDER BY CASE WHEN COALESCE(parent_id, '') = '' THEN 0 ELSE 1 END, created_at, id
	`), projectID)
	if err != nil {
		return nil, fmt.Errorf("list project tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tasks := make([]TaskSummary, 0)
	for rows.Next() {
		var task TaskSummary
		if err := rows.Scan(&task.ID, &task.Title, &task.State, &task.ParentID, &task.ArchivedAt, &task.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project task: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) CountActiveWorkers(ctx context.Context, projectID, coordinatorTaskID string) (int, error) {
	var count int
	err := s.db.GetContext(ctx, &count, s.db.Rebind(`
		SELECT COUNT(*) FROM tasks t
		WHERE t.agent_project_id = ? AND t.parent_id = ? AND t.archived_at IS NULL
		AND EXISTS (
			SELECT 1 FROM task_sessions ts
			WHERE ts.task_id = t.id AND UPPER(ts.state) IN ('STARTING', 'RUNNING')
		)
	`), projectID, coordinatorTaskID)
	return count, err
}

func (s *Store) HasRunningTaskSessions(ctx context.Context, projectID string) (bool, error) {
	var running bool
	err := s.db.GetContext(ctx, &running, s.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM tasks t
			JOIN task_sessions ts ON ts.task_id = t.id
			WHERE t.agent_project_id = ? AND UPPER(ts.state) IN ('STARTING', 'RUNNING')
		)
	`), projectID)
	return running, err
}

func (s *Store) SetArchived(ctx context.Context, workspaceID, projectID string, archived bool) error {
	var archivedAt any
	filter := `archived_at IS NOT NULL`
	if archived {
		archivedAt = time.Now().UTC()
		filter = `archived_at IS NULL`
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE agent_projects SET archived_at = ?, revision = revision + 1, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND `+filter), archivedAt, time.Now().UTC(), workspaceID, projectID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		project, err := s.Get(ctx, workspaceID, projectID)
		if err != nil {
			return err
		}
		if (project.ArchivedAt != nil) == archived {
			return nil
		}
		return ErrNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, workspaceID, projectID string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM agent_projects WHERE workspace_id = ? AND id = ?`), workspaceID, projectID)
	if err != nil {
		return fmt.Errorf("delete agent project: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

const projectSelect = `
	SELECT id, workspace_id, name, repository_ids, primary_repository_id,
		coordinator_profile_id, economy_profile_id, frontier_profile_id,
		executor_profile_id, main_task_id, revision, archived_at, created_at, updated_at
	FROM agent_projects
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(row rowScanner) (*Project, error) {
	var project Project
	var repositories string
	if err := row.Scan(
		&project.ID, &project.WorkspaceID, &project.Name, &repositories, &project.PrimaryRepositoryID,
		&project.CoordinatorProfileID, &project.EconomyProfileID, &project.FrontierProfileID,
		&project.ExecutorProfileID, &project.MainTaskID, &project.Revision, &project.ArchivedAt,
		&project.CreatedAt, &project.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(repositories), &project.RepositoryIDs); err != nil {
		return nil, fmt.Errorf("decode project repositories: %w", err)
	}
	if project.RepositoryIDs == nil {
		project.RepositoryIDs = []string{}
	}
	return &project, nil
}
