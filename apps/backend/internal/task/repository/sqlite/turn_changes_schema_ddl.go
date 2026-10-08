package sqlite

import "fmt"

type turnChangeSchemaStatement struct {
	name string
	sql  string
}

const turnChangeSetTableStartDDL = `CREATE TABLE IF NOT EXISTS turn_change_sets (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	session_id TEXT NOT NULL REFERENCES task_sessions(id) ON DELETE CASCADE,
	turn_id TEXT NOT NULL REFERENCES task_session_turns(id) ON DELETE CASCADE,
	task_environment_id TEXT NOT NULL,
	revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
	runtime_execution_id TEXT NOT NULL DEFAULT '',
	startup_attempt_id TEXT NOT NULL DEFAULT '',
	prompt_generation BIGINT NOT NULL DEFAULT 0,
	execution_profile_id TEXT NOT NULL DEFAULT '',
	route_generation BIGINT NOT NULL DEFAULT 0,
`

const turnChangeSetTableBooleanColumnsDDL = `	capture_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	start_accepted BOOLEAN NOT NULL DEFAULT FALSE,
	complete BOOLEAN NOT NULL DEFAULT FALSE,
	summary_complete BOOLEAN NOT NULL DEFAULT FALSE,
	content_complete BOOLEAN NOT NULL DEFAULT FALSE,
`

const turnChangeSetTableTailDDL = `	settings_user_id TEXT NOT NULL DEFAULT '',
	actor_id TEXT NOT NULL DEFAULT '',
	settings_revision BIGINT NOT NULL DEFAULT 0,
	resolution_kind TEXT NOT NULL DEFAULT '',
	availability TEXT NOT NULL DEFAULT 'pending'
		CHECK (availability IN ('pending', 'ready', 'unavailable', 'failed', 'expired')),
	reason TEXT NOT NULL DEFAULT '',
	turn_ordinal BIGINT NOT NULL DEFAULT 0,
	terminal_at TIMESTAMP,
	terminal_outcome TEXT NOT NULL DEFAULT '',
	final_assistant_message_id TEXT NOT NULL DEFAULT '',
	terminal_capture_started_at TIMESTAMP,
	terminal_capture_execution_id TEXT NOT NULL DEFAULT '',
	terminal_capture_startup_attempt_id TEXT NOT NULL DEFAULT '',
	terminal_capture_prompt_generation BIGINT NOT NULL DEFAULT 0,
	terminal_capture_environment_id TEXT NOT NULL DEFAULT '',
	terminal_capture_outcome TEXT NOT NULL DEFAULT '',
	terminal_capture_final_message_id TEXT NOT NULL DEFAULT '',
	fallback_anchor TEXT NOT NULL DEFAULT '',
	file_count BIGINT NOT NULL DEFAULT 0,
	added_lines BIGINT,
	deleted_lines BIGINT,
	binary_file_count BIGINT NOT NULL DEFAULT 0,
	unknown_count_file_count BIGINT NOT NULL DEFAULT 0,
	repository_count BIGINT NOT NULL DEFAULT 0,
	retain_until TIMESTAMP,
	expiry_reason TEXT NOT NULL DEFAULT '',
	content_bytes BIGINT NOT NULL DEFAULT 0,
	overlap_intervals_json TEXT NOT NULL DEFAULT '[]',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	UNIQUE (task_id, session_id, turn_id)
)`

const turnChangeSetTableDDL = turnChangeSetTableStartDDL + turnChangeSetTableBooleanColumnsDDL + turnChangeSetTableTailDDL

const turnRepositoryChangesTableDDL = `CREATE TABLE IF NOT EXISTS turn_repository_changes (
	id TEXT PRIMARY KEY,
	change_set_id TEXT NOT NULL REFERENCES turn_change_sets(id) ON DELETE CASCADE,
	checkout_id TEXT NOT NULL,
	environment_repo_id TEXT NOT NULL DEFAULT '',
	task_repository_id TEXT NOT NULL DEFAULT '',
	repository_id TEXT NOT NULL DEFAULT '',
	worktree_id TEXT NOT NULL DEFAULT '',
	display_name TEXT NOT NULL DEFAULT '',
	repository_subpath TEXT NOT NULL DEFAULT '',
	start_commit_oid TEXT NOT NULL DEFAULT '',
	start_tree_oid TEXT NOT NULL DEFAULT '',
	end_commit_oid TEXT NOT NULL DEFAULT '',
	end_tree_oid TEXT NOT NULL DEFAULT '',
	hash_algorithm TEXT NOT NULL DEFAULT '',
	start_captured_at TIMESTAMP,
	end_captured_at TIMESTAMP,
	start_ref TEXT NOT NULL DEFAULT '',
	end_ref TEXT NOT NULL DEFAULT '',
	availability TEXT NOT NULL DEFAULT 'pending'
		CHECK (availability IN ('pending', 'ready', 'unavailable', 'failed', 'expired')),
	reason TEXT NOT NULL DEFAULT '',
	cleanup_pending BOOLEAN NOT NULL DEFAULT FALSE,
	enumeration_complete BOOLEAN NOT NULL DEFAULT FALSE,
	comparison_complete BOOLEAN NOT NULL DEFAULT FALSE,
	content_complete BOOLEAN NOT NULL DEFAULT FALSE,
	overlap_intervals_json TEXT NOT NULL DEFAULT '[]',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	UNIQUE (change_set_id, checkout_id)
)`

const turnFileChangesTableStartDDL = `CREATE TABLE IF NOT EXISTS turn_file_changes (
	id TEXT PRIMARY KEY,
	repository_change_id TEXT NOT NULL REFERENCES turn_repository_changes(id) ON DELETE CASCADE,
	checkout_id TEXT NOT NULL,
	path TEXT NOT NULL DEFAULT '',
	path_bytes %s NOT NULL,
	old_path TEXT,
	old_path_bytes %s,
	kind TEXT NOT NULL CHECK (kind IN ('added', 'deleted', 'modified', 'renamed', 'copied', 'mode_changed', 'type_changed')),
	old_blob_oid TEXT NOT NULL DEFAULT '',
	new_blob_oid TEXT NOT NULL DEFAULT '',
	old_mode TEXT NOT NULL DEFAULT '',
	new_mode TEXT NOT NULL DEFAULT '',
`

const turnFileChangesTableBooleanColumnsDDL = `	submodule BOOLEAN NOT NULL DEFAULT FALSE,
	binary BOOLEAN NOT NULL DEFAULT FALSE,
`

const turnFileChangesTableTailDDL = `	added_lines BIGINT,
	deleted_lines BIGINT,
	canonical_content_id TEXT NOT NULL DEFAULT '',
	filtered_content_id TEXT NOT NULL DEFAULT '',
	old_content_id TEXT NOT NULL DEFAULT '',
	new_content_id TEXT NOT NULL DEFAULT '',
	content_availability TEXT NOT NULL DEFAULT 'pending'
		CHECK (content_availability IN ('pending', 'ready', 'unavailable', 'failed', 'expired')),
	content_reason TEXT NOT NULL DEFAULT '',
	content_truncated BOOLEAN NOT NULL DEFAULT FALSE,
	canonical_content_bytes BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL,
	UNIQUE (repository_change_id, path_bytes)
)`

const turnFileChangesTableDDL = turnFileChangesTableStartDDL + turnFileChangesTableBooleanColumnsDDL + turnFileChangesTableTailDDL

const turnChangeContentsTableDDL = `CREATE TABLE IF NOT EXISTS turn_change_contents (
	id TEXT PRIMARY KEY,
	digest TEXT NOT NULL UNIQUE,
	codec TEXT NOT NULL,
	uncompressed_bytes BIGINT NOT NULL CHECK (uncompressed_bytes >= 0),
	payload_bytes %s NOT NULL,
	created_at TIMESTAMP NOT NULL
)`

const turnChangeContentLinksTableDDL = `CREATE TABLE IF NOT EXISTS turn_change_content_links (
	file_change_id TEXT NOT NULL REFERENCES turn_file_changes(id) ON DELETE CASCADE,
	variant TEXT NOT NULL CHECK (variant IN ('canonical_patch', 'filtered_patch', 'old_rendering', 'new_rendering')),
	content_id TEXT NOT NULL REFERENCES turn_change_contents(id) ON DELETE RESTRICT,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (file_change_id, variant)
)`

const turnChangeContentLeasesTableDDL = `CREATE TABLE IF NOT EXISTS turn_change_content_leases (
	id TEXT PRIMARY KEY,
	change_set_id TEXT NOT NULL REFERENCES turn_change_sets(id) ON DELETE CASCADE,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL
)`

func turnChangeSchemaStatements(payloadType string) []turnChangeSchemaStatement {
	return []turnChangeSchemaStatement{
		{name: "turn_change_sets.table", sql: turnChangeSetTableDDL},
		{name: "turn_change_sets.history_index", sql: `CREATE INDEX IF NOT EXISTS idx_turn_change_sets_history ON turn_change_sets(task_id, session_id, turn_ordinal, id)`},
		{name: "turn_change_sets.retention_index", sql: `CREATE INDEX IF NOT EXISTS idx_turn_change_sets_retention ON turn_change_sets(terminal_at, retain_until)`},
		{name: "turn_repository_changes.table", sql: turnRepositoryChangesTableDDL},
		{name: "turn_repository_changes.checkout_index", sql: `CREATE INDEX IF NOT EXISTS idx_turn_repository_changes_checkout ON turn_repository_changes(checkout_id, created_at)`},
		{name: "turn_file_changes.table", sql: fmt.Sprintf(turnFileChangesTableDDL, payloadType, payloadType)},
		{name: "turn_file_changes.catalog_index", sql: `CREATE INDEX IF NOT EXISTS idx_turn_file_changes_catalog ON turn_file_changes(repository_change_id, id)`},
		{name: "turn_change_contents.table", sql: fmt.Sprintf(turnChangeContentsTableDDL, payloadType)},
		{name: "turn_change_content_links.table", sql: turnChangeContentLinksTableDDL},
		{name: "turn_change_content_links.content_index", sql: `CREATE INDEX IF NOT EXISTS idx_turn_change_content_links_content ON turn_change_content_links(content_id)`},
		{name: "turn_change_content_leases.table", sql: turnChangeContentLeasesTableDDL},
		{name: "turn_change_content_leases.expiry_index", sql: `CREATE INDEX IF NOT EXISTS idx_turn_change_content_leases_expiry ON turn_change_content_leases(change_set_id, expires_at)`},
	}
}
