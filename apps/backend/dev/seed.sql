-- Seed data for development database
-- This file is used to populate the dev database with sample data

-- Default workspace (created by initSchema, just update if needed)
INSERT OR REPLACE INTO workspaces (id, name, description, owner_id, created_at, updated_at)
VALUES ('default', 'Default Workspace', 'Your default workspace', '', datetime('now'), datetime('now'));

-- Default user and settings
INSERT OR REPLACE INTO users (id, email, settings, created_at, updated_at)
VALUES ('default-user', 'default@kandev.local', '{"workspace_id":"default","board_id":"dev-board-1","repository_ids":[]}', datetime('now'), datetime('now'));

-- Default repository (for task creation)
INSERT OR REPLACE INTO repositories (id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner, provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at)
VALUES ('default-repo', 'default', 'kandev', 'local', '$KANDEV_REPO_PATH', '', '', '', '', 'main', '', '', datetime('now'), datetime('now'));

-- Sample board
INSERT OR REPLACE INTO boards (id, workspace_id, name, description, created_at, updated_at)
VALUES 
    ('dev-board-1', 'default', 'Kandev Development', 'Main development board for kandev', datetime('now'), datetime('now')),
    ('dev-board-2', 'default', 'Feature Backlog', 'Upcoming features and ideas', datetime('now'), datetime('now'));

-- Columns for dev-board-1
INSERT OR REPLACE INTO columns (id, board_id, name, position, state, created_at, updated_at)
VALUES
    ('col-backlog', 'dev-board-1', 'Backlog', 0, 'TODO', datetime('now'), datetime('now')),
    ('col-todo', 'dev-board-1', 'To Do', 1, 'TODO', datetime('now'), datetime('now')),
    ('col-inprogress', 'dev-board-1', 'In Progress', 2, 'IN_PROGRESS', datetime('now'), datetime('now')),
    ('col-review', 'dev-board-1', 'Review', 3, 'REVIEW', datetime('now'), datetime('now')),
    ('col-done', 'dev-board-1', 'Done', 4, 'COMPLETED', datetime('now'), datetime('now'));

-- Columns for dev-board-2
INSERT OR REPLACE INTO columns (id, board_id, name, position, state, created_at, updated_at)
VALUES
    ('col-ideas', 'dev-board-2', 'Ideas', 0, 'TODO', datetime('now'), datetime('now')),
    ('col-planned', 'dev-board-2', 'Planned', 1, 'TODO', datetime('now'), datetime('now')),
    ('col-scheduled', 'dev-board-2', 'Scheduled', 2, 'IN_PROGRESS', datetime('now'), datetime('now'));

-- Sample tasks for dev-board-1
-- NOTE: repository_url should be an absolute path to a local git repository
-- The placeholder $KANDEV_REPO_PATH will be replaced by init-db.sh with the actual path
INSERT OR REPLACE INTO tasks (id, board_id, column_id, title, description, state, priority, position, agent_type, repository_url, branch, metadata, created_at, updated_at)
VALUES
    -- Backlog tasks (no agent assigned)
    ('task-001', 'dev-board-1', 'col-backlog', 'Add dark mode support', 'Implement a dark theme toggle for the UI', 'TODO', 3, 0, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now'), datetime('now')),
    ('task-002', 'dev-board-1', 'col-backlog', 'Implement search functionality', 'Add full-text search for tasks across all boards', 'TODO', 2, 1, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now'), datetime('now')),

    -- To Do tasks (agent assigned with repository)
    ('task-003', 'dev-board-1', 'col-todo', 'Fix WebSocket reconnection', 'Handle WebSocket disconnects gracefully and auto-reconnect', 'TODO', 5, 0, 'augment-agent', '$KANDEV_REPO_PATH', 'main', '{}', datetime('now'), datetime('now')),
    ('task-004', 'dev-board-1', 'col-todo', 'Add task labels/tags', 'Allow users to add colored labels to tasks for categorization', 'TODO', 2, 1, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now'), datetime('now')),

    -- In Progress tasks (agent with repository)
    ('task-005', 'dev-board-1', 'col-inprogress', 'Agent session persistence', 'Persist agent sessions in database instead of memory', 'IN_PROGRESS', 8, 0, 'augment-agent', '$KANDEV_REPO_PATH', 'main', '{}', datetime('now'), datetime('now')),

    -- Review tasks
    ('task-006', 'dev-board-1', 'col-review', 'Comment system improvements', 'Enhanced comment display with markdown and tool calls', 'REVIEW', 6, 0, 'augment-agent', '$KANDEV_REPO_PATH', 'main', '{}', datetime('now'), datetime('now')),

    -- Done tasks
    ('task-007', 'dev-board-1', 'col-done', 'Basic kanban board UI', 'Create the initial kanban board with drag-and-drop', 'COMPLETED', 10, 0, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now', '-3 days'), datetime('now', '-1 day')),
    ('task-008', 'dev-board-1', 'col-done', 'WebSocket API implementation', 'Implement real-time WebSocket communication', 'COMPLETED', 9, 1, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now', '-5 days'), datetime('now', '-2 days'));

-- Sample tasks for dev-board-2 (Feature Backlog)
INSERT OR REPLACE INTO tasks (id, board_id, column_id, title, description, state, priority, position, agent_type, repository_url, branch, metadata, created_at, updated_at)
VALUES
    ('task-101', 'dev-board-2', 'col-ideas', 'Multi-user collaboration', 'Allow multiple users to work on the same board', 'TODO', 5, 0, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now'), datetime('now')),
    ('task-102', 'dev-board-2', 'col-ideas', 'Task dependencies', 'Add ability to link tasks as dependencies', 'TODO', 4, 1, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now'), datetime('now')),
    ('task-103', 'dev-board-2', 'col-planned', 'Agent type marketplace', 'Browse and install different agent types', 'TODO', 6, 0, '', '$KANDEV_REPO_PATH', '', '{}', datetime('now'), datetime('now'));

-- Sample comments on tasks
INSERT OR REPLACE INTO task_comments (id, task_id, author_type, author_id, content, type, metadata, requests_input, task_session_id, created_at)
VALUES
    -- Comments on task-005 (Agent session persistence)
    ('comment-001', 'task-005', 'user', 'dev-user', 'Starting work on database schema for agent sessions', 'message', '{}', 0, '', datetime('now', '-2 hours')),
    ('comment-002', 'task-005', 'agent', 'augment-agent', 'I''ll help you implement the agent session persistence. Let me start by analyzing the current codebase structure.', 'message', '{}', 0, '', datetime('now', '-1 hour')),
    ('comment-003', 'task-005', 'agent', 'augment-agent', 'codebase-retrieval', 'tool_call', '{"tool_call_id": "tc-001", "title": "codebase-retrieval: Looking for agent execution tracking", "status": "completed"}', 0, '', datetime('now', '-55 minutes')),
    
    -- Comments on task-006 (Comment system)
    ('comment-004', 'task-006', 'user', 'dev-user', 'Need to add support for tool call rendering in the chat panel', 'message', '{}', 0, '', datetime('now', '-1 day')),
    ('comment-005', 'task-006', 'agent', 'augment-agent', 'I''ve implemented the tool call comment type with proper rendering. The changes include:\n\n1. Added `type` field to comments\n2. Added `metadata` for tool call details\n3. Updated the frontend to render tool calls differently', 'message', '{}', 0, '', datetime('now', '-20 hours'));

-- Sample task session
INSERT OR REPLACE INTO task_sessions (id, task_id, agent_instance_id, agent_type, state, progress, error_message, metadata, started_at, completed_at, updated_at)
VALUES
    ('session-001', 'task-005', 'agent-instance-123', 'augment-agent', 'RUNNING', 0.0, '', '{}', datetime('now', '-1 hour'), NULL, datetime('now'));
