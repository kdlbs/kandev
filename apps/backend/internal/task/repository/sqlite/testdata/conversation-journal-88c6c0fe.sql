
CREATE TABLE IF NOT EXISTS conversation_session_streams (
	session_id TEXT PRIMARY KEY,
	watermark BIGINT NOT NULL DEFAULT 0,
	terminal BOOLEAN NOT NULL DEFAULT FALSE,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS conversation_session_events (
	session_id TEXT NOT NULL,
	sequence BIGINT NOT NULL,
	event_id TEXT NOT NULL UNIQUE,
	protocol_version INTEGER NOT NULL DEFAULT 1,
	event_type TEXT NOT NULL,
	task_id TEXT,
	payload TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_events_created
	ON conversation_session_events(created_at);
CREATE TABLE IF NOT EXISTS conversation_message_versions (
	session_id TEXT NOT NULL,
	message_id TEXT NOT NULL,
	row_sequence BIGINT NOT NULL,
	task_id TEXT,
	author_type TEXT,
	created_at TIMESTAMP NOT NULL,
	tombstone BOOLEAN NOT NULL DEFAULT FALSE,
	payload TEXT NOT NULL,
	PRIMARY KEY (session_id, message_id, row_sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_message_snapshot
	ON conversation_message_versions(session_id, row_sequence, created_at, message_id);
CREATE TABLE IF NOT EXISTS conversation_turn_versions (
	session_id TEXT NOT NULL,
	turn_id TEXT NOT NULL,
	row_sequence BIGINT NOT NULL,
	task_id TEXT,
	started_at TIMESTAMP NOT NULL,
	tombstone BOOLEAN NOT NULL DEFAULT FALSE,
	payload TEXT NOT NULL,
	PRIMARY KEY (session_id, turn_id, row_sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_turn_snapshot
	ON conversation_turn_versions(session_id, row_sequence, started_at, turn_id);
CREATE TABLE IF NOT EXISTS conversation_journal_meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

DROP TRIGGER IF EXISTS conversation_message_insert;
CREATE TRIGGER IF NOT EXISTS conversation_message_insert
AFTER INSERT ON task_session_messages
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_message_versions(session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.author_type, NEW.created_at, FALSE,
		json_object('type','message.added','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'message_id',NEW.id,'turn_id',NULLIF(NEW.turn_id,''),'author_type',NEW.author_type,
			'content',(SELECT trim(x) FROM (
		WITH RECURSIVE strip(x, depth) AS (
			SELECT NEW.content, 0
			UNION ALL
			SELECT substr(x, 1, instr(x, '<kandev-system>') - 1)
				|| ltrim(substr(x,
					instr(x, '<kandev-system>') + length('<kandev-system>')
						+ instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') - 1
						+ length('</kandev-system>')),
					char(32) || char(9) || char(10) || char(13)), depth + 1
			FROM strip
			WHERE depth < 64
				AND instr(x, '<kandev-system>') > 0
				AND instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') > 0
		)
		SELECT CASE WHEN depth = 64 AND instr(x, '<kandev-system>') > 0 AND instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') > 0 THEN substr(x, 1, instr(x, '<kandev-system>') - 1) ELSE x END AS x FROM strip ORDER BY depth DESC LIMIT 1
	)),'message_type',NEW.type,'requests_input',NEW.requests_input,
			'created_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.created_at),
		'updated_at',strftime('%Y-%m-%dT%H:%M:%fZ', COALESCE(NEW.updated_at,NEW.created_at)),'prompt_index',NEW.prompt_seq,
			'metadata',CASE WHEN json_valid(NEW.metadata) THEN json_object('action_details',CASE json_type(NEW.metadata, '$.action_details') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.action_details') END,'action_type',CASE json_type(NEW.metadata, '$.action_type') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.action_type') END,'action_visibility',CASE json_type(NEW.metadata, '$.action_visibility') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.action_visibility') END,'actions',CASE json_type(NEW.metadata, '$.actions') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.actions') END,'agent_disconnected',CASE json_type(NEW.metadata, '$.agent_disconnected') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.agent_disconnected') END,'attempt',CASE json_type(NEW.metadata, '$.attempt') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.attempt') END,'attachments',CASE json_type(NEW.metadata, '$.attachments') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.attachments') END,'auth_methods',CASE json_type(NEW.metadata, '$.auth_methods') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.auth_methods') END,'auto_start',CASE json_type(NEW.metadata, '$.auto_start') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.auto_start') END,'base_branch',CASE json_type(NEW.metadata, '$.base_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.base_branch') END,'context',CASE json_type(NEW.metadata, '$.context') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.context') END,'context_files',CASE json_type(NEW.metadata, '$.context_files') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.context_files') END,'decision_id',CASE json_type(NEW.metadata, '$.decision_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.decision_id') END,'effective_model',CASE json_type(NEW.metadata, '$.effective_model') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.effective_model') END,'entity_references',CASE json_type(NEW.metadata, '$.entity_references') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.entity_references') END,'error_output',CASE json_type(NEW.metadata, '$.error_output') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.error_output') END,'failure_code',CASE json_type(NEW.metadata, '$.failure_code') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.failure_code') END,'failure_details',CASE json_type(NEW.metadata, '$.failure_details') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.failure_details') END,'failure_kind',CASE json_type(NEW.metadata, '$.failure_kind') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.failure_kind') END,'fallback_model',CASE json_type(NEW.metadata, '$.fallback_model') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.fallback_model') END,'has_hidden_prompts',CASE json_type(NEW.metadata, '$.has_hidden_prompts') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.has_hidden_prompts') END,'has_resume_token',CASE json_type(NEW.metadata, '$.has_resume_token') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.has_resume_token') END,'has_review_comments',CASE json_type(NEW.metadata, '$.has_review_comments') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.has_review_comments') END,'is_auth_error',CASE json_type(NEW.metadata, '$.is_auth_error') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.is_auth_error') END,'kind',CASE json_type(NEW.metadata, '$.kind') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.kind') END,'max_attempts',CASE json_type(NEW.metadata, '$.max_attempts') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.max_attempts') END,'message',CASE json_type(NEW.metadata, '$.message') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.message') END,'missing_branch',CASE json_type(NEW.metadata, '$.missing_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.missing_branch') END,'model_id',CASE json_type(NEW.metadata, '$.model_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.model_id') END,'new_branch',CASE json_type(NEW.metadata, '$.new_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.new_branch') END,'original_branch',CASE json_type(NEW.metadata, '$.original_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.original_branch') END,'options',CASE json_type(NEW.metadata, '$.options') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.options') END,'pending_id',CASE json_type(NEW.metadata, '$.pending_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.pending_id') END,'plan_mode',CASE json_type(NEW.metadata, '$.plan_mode') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.plan_mode') END,'progress',CASE json_type(NEW.metadata, '$.progress') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.progress') END,'provider_name',CASE json_type(NEW.metadata, '$.provider_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.provider_name') END,'question',CASE json_type(NEW.metadata, '$.question') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question') END,'question_id',CASE json_type(NEW.metadata, '$.question_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question_id') END,'question_index',CASE json_type(NEW.metadata, '$.question_index') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question_index') END,'question_total',CASE json_type(NEW.metadata, '$.question_total') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question_total') END,'recovery_actions',CASE json_type(NEW.metadata, '$.recovery_actions') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.recovery_actions') END,'remediation',CASE json_type(NEW.metadata, '$.remediation') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.remediation') END,'remediation_url',CASE json_type(NEW.metadata, '$.remediation_url') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.remediation_url') END,'requested_model',CASE json_type(NEW.metadata, '$.requested_model') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.requested_model') END,'request_id',CASE json_type(NEW.metadata, '$.request_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.request_id') END,'response',CASE json_type(NEW.metadata, '$.response') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.response') END,'reset_at',CASE json_type(NEW.metadata, '$.reset_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.reset_at') END,'retry_at',CASE json_type(NEW.metadata, '$.retry_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.retry_at') END,'retry_in_seconds',CASE json_type(NEW.metadata, '$.retry_in_seconds') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.retry_in_seconds') END,'retrying',CASE json_type(NEW.metadata, '$.retrying') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.retrying') END,'sender_session_id',CASE json_type(NEW.metadata, '$.sender_session_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_session_id') END,'sender_session_name',CASE json_type(NEW.metadata, '$.sender_session_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_session_name') END,'sender_task_id',CASE json_type(NEW.metadata, '$.sender_task_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_task_id') END,'sender_task_title',CASE json_type(NEW.metadata, '$.sender_task_title') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_task_title') END,'stage',CASE json_type(NEW.metadata, '$.stage') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.stage') END,'status',CASE json_type(NEW.metadata, '$.status') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.status') END,'script_type',CASE json_type(NEW.metadata, '$.script_type') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.script_type') END,'agent_name',CASE json_type(NEW.metadata, '$.agent_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.agent_name') END,'command',CASE json_type(NEW.metadata, '$.command') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.command') END,'exit_code',CASE json_type(NEW.metadata, '$.exit_code') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.exit_code') END,'is_resuming',CASE json_type(NEW.metadata, '$.is_resuming') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.is_resuming') END,'started_at',CASE json_type(NEW.metadata, '$.started_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.started_at') END,'completed_at',CASE json_type(NEW.metadata, '$.completed_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.completed_at') END,'error',CASE json_type(NEW.metadata, '$.error') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.error') END,'task_id',CASE json_type(NEW.metadata, '$.task_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.task_id') END,'text',CASE json_type(NEW.metadata, '$.text') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.text') END,'tool_call_id',CASE json_type(NEW.metadata, '$.tool_call_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.tool_call_id') END,'variant',CASE json_type(NEW.metadata, '$.variant') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.variant') END,'workflow_message',CASE json_type(NEW.metadata, '$.workflow_message') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_message') END,'workflow_step_color',CASE json_type(NEW.metadata, '$.workflow_step_color') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_step_color') END,'workflow_step_id',CASE json_type(NEW.metadata, '$.workflow_step_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_step_id') END,'workflow_step_name',CASE json_type(NEW.metadata, '$.workflow_step_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_step_name') END) ELSE json_object() END,
			'sender_task_id',CASE WHEN json_valid(NEW.metadata) AND typeof(json_extract(NEW.metadata,'$.sender_task_id')) = 'text' THEN json_extract(NEW.metadata,'$.sender_task_id') END)
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'message.added', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_message_versions
		ON conversation_message_versions.session_id = conversation_session_streams.session_id
		AND conversation_message_versions.message_id = NEW.id
		AND conversation_message_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;

DROP TRIGGER IF EXISTS conversation_message_update;
CREATE TRIGGER IF NOT EXISTS conversation_message_update
AFTER UPDATE ON task_session_messages
WHEN OLD.id IS NOT NEW.id
	OR OLD.task_session_id IS NOT NEW.task_session_id
	OR OLD.task_id IS NOT NEW.task_id
	OR OLD.turn_id IS NOT NEW.turn_id
	OR OLD.author_type IS NOT NEW.author_type
	OR OLD.author_id IS NOT NEW.author_id
	OR OLD.content IS NOT NEW.content
	OR OLD.requests_input IS NOT NEW.requests_input
	OR OLD.type IS NOT NEW.type
	OR OLD.metadata IS NOT NEW.metadata
	OR OLD.created_at IS NOT NEW.created_at
	OR OLD.updated_at IS NOT NEW.updated_at
	OR OLD.prompt_seq IS NOT NEW.prompt_seq
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_message_versions(session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.author_type, NEW.created_at, FALSE,
		json_object('type','message.updated','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'message_id',NEW.id,'turn_id',NULLIF(NEW.turn_id,''),'author_type',NEW.author_type,
			'content',(SELECT trim(x) FROM (
		WITH RECURSIVE strip(x, depth) AS (
			SELECT NEW.content, 0
			UNION ALL
			SELECT substr(x, 1, instr(x, '<kandev-system>') - 1)
				|| ltrim(substr(x,
					instr(x, '<kandev-system>') + length('<kandev-system>')
						+ instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') - 1
						+ length('</kandev-system>')),
					char(32) || char(9) || char(10) || char(13)), depth + 1
			FROM strip
			WHERE depth < 64
				AND instr(x, '<kandev-system>') > 0
				AND instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') > 0
		)
		SELECT CASE WHEN depth = 64 AND instr(x, '<kandev-system>') > 0 AND instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') > 0 THEN substr(x, 1, instr(x, '<kandev-system>') - 1) ELSE x END AS x FROM strip ORDER BY depth DESC LIMIT 1
	)),'message_type',NEW.type,'requests_input',NEW.requests_input,
			'created_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.created_at),
			'updated_at',strftime('%Y-%m-%dT%H:%M:%fZ', COALESCE(NEW.updated_at,NEW.created_at)),'prompt_index',NEW.prompt_seq,
			'metadata',CASE WHEN json_valid(NEW.metadata) THEN json_object('action_details',CASE json_type(NEW.metadata, '$.action_details') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.action_details') END,'action_type',CASE json_type(NEW.metadata, '$.action_type') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.action_type') END,'action_visibility',CASE json_type(NEW.metadata, '$.action_visibility') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.action_visibility') END,'actions',CASE json_type(NEW.metadata, '$.actions') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.actions') END,'agent_disconnected',CASE json_type(NEW.metadata, '$.agent_disconnected') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.agent_disconnected') END,'attempt',CASE json_type(NEW.metadata, '$.attempt') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.attempt') END,'attachments',CASE json_type(NEW.metadata, '$.attachments') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.attachments') END,'auth_methods',CASE json_type(NEW.metadata, '$.auth_methods') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.auth_methods') END,'auto_start',CASE json_type(NEW.metadata, '$.auto_start') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.auto_start') END,'base_branch',CASE json_type(NEW.metadata, '$.base_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.base_branch') END,'context',CASE json_type(NEW.metadata, '$.context') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.context') END,'context_files',CASE json_type(NEW.metadata, '$.context_files') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.context_files') END,'decision_id',CASE json_type(NEW.metadata, '$.decision_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.decision_id') END,'effective_model',CASE json_type(NEW.metadata, '$.effective_model') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.effective_model') END,'entity_references',CASE json_type(NEW.metadata, '$.entity_references') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.entity_references') END,'error_output',CASE json_type(NEW.metadata, '$.error_output') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.error_output') END,'failure_code',CASE json_type(NEW.metadata, '$.failure_code') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.failure_code') END,'failure_details',CASE json_type(NEW.metadata, '$.failure_details') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.failure_details') END,'failure_kind',CASE json_type(NEW.metadata, '$.failure_kind') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.failure_kind') END,'fallback_model',CASE json_type(NEW.metadata, '$.fallback_model') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.fallback_model') END,'has_hidden_prompts',CASE json_type(NEW.metadata, '$.has_hidden_prompts') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.has_hidden_prompts') END,'has_resume_token',CASE json_type(NEW.metadata, '$.has_resume_token') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.has_resume_token') END,'has_review_comments',CASE json_type(NEW.metadata, '$.has_review_comments') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.has_review_comments') END,'is_auth_error',CASE json_type(NEW.metadata, '$.is_auth_error') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.is_auth_error') END,'kind',CASE json_type(NEW.metadata, '$.kind') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.kind') END,'max_attempts',CASE json_type(NEW.metadata, '$.max_attempts') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.max_attempts') END,'message',CASE json_type(NEW.metadata, '$.message') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.message') END,'missing_branch',CASE json_type(NEW.metadata, '$.missing_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.missing_branch') END,'model_id',CASE json_type(NEW.metadata, '$.model_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.model_id') END,'new_branch',CASE json_type(NEW.metadata, '$.new_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.new_branch') END,'original_branch',CASE json_type(NEW.metadata, '$.original_branch') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.original_branch') END,'options',CASE json_type(NEW.metadata, '$.options') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.options') END,'pending_id',CASE json_type(NEW.metadata, '$.pending_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.pending_id') END,'plan_mode',CASE json_type(NEW.metadata, '$.plan_mode') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.plan_mode') END,'progress',CASE json_type(NEW.metadata, '$.progress') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.progress') END,'provider_name',CASE json_type(NEW.metadata, '$.provider_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.provider_name') END,'question',CASE json_type(NEW.metadata, '$.question') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question') END,'question_id',CASE json_type(NEW.metadata, '$.question_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question_id') END,'question_index',CASE json_type(NEW.metadata, '$.question_index') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question_index') END,'question_total',CASE json_type(NEW.metadata, '$.question_total') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.question_total') END,'recovery_actions',CASE json_type(NEW.metadata, '$.recovery_actions') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.recovery_actions') END,'remediation',CASE json_type(NEW.metadata, '$.remediation') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.remediation') END,'remediation_url',CASE json_type(NEW.metadata, '$.remediation_url') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.remediation_url') END,'requested_model',CASE json_type(NEW.metadata, '$.requested_model') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.requested_model') END,'request_id',CASE json_type(NEW.metadata, '$.request_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.request_id') END,'response',CASE json_type(NEW.metadata, '$.response') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.response') END,'reset_at',CASE json_type(NEW.metadata, '$.reset_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.reset_at') END,'retry_at',CASE json_type(NEW.metadata, '$.retry_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.retry_at') END,'retry_in_seconds',CASE json_type(NEW.metadata, '$.retry_in_seconds') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.retry_in_seconds') END,'retrying',CASE json_type(NEW.metadata, '$.retrying') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.retrying') END,'sender_session_id',CASE json_type(NEW.metadata, '$.sender_session_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_session_id') END,'sender_session_name',CASE json_type(NEW.metadata, '$.sender_session_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_session_name') END,'sender_task_id',CASE json_type(NEW.metadata, '$.sender_task_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_task_id') END,'sender_task_title',CASE json_type(NEW.metadata, '$.sender_task_title') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.sender_task_title') END,'stage',CASE json_type(NEW.metadata, '$.stage') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.stage') END,'status',CASE json_type(NEW.metadata, '$.status') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.status') END,'script_type',CASE json_type(NEW.metadata, '$.script_type') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.script_type') END,'agent_name',CASE json_type(NEW.metadata, '$.agent_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.agent_name') END,'command',CASE json_type(NEW.metadata, '$.command') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.command') END,'exit_code',CASE json_type(NEW.metadata, '$.exit_code') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.exit_code') END,'is_resuming',CASE json_type(NEW.metadata, '$.is_resuming') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.is_resuming') END,'started_at',CASE json_type(NEW.metadata, '$.started_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.started_at') END,'completed_at',CASE json_type(NEW.metadata, '$.completed_at') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.completed_at') END,'error',CASE json_type(NEW.metadata, '$.error') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.error') END,'task_id',CASE json_type(NEW.metadata, '$.task_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.task_id') END,'text',CASE json_type(NEW.metadata, '$.text') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.text') END,'tool_call_id',CASE json_type(NEW.metadata, '$.tool_call_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.tool_call_id') END,'variant',CASE json_type(NEW.metadata, '$.variant') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.variant') END,'workflow_message',CASE json_type(NEW.metadata, '$.workflow_message') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_message') END,'workflow_step_color',CASE json_type(NEW.metadata, '$.workflow_step_color') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_step_color') END,'workflow_step_id',CASE json_type(NEW.metadata, '$.workflow_step_id') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_step_id') END,'workflow_step_name',CASE json_type(NEW.metadata, '$.workflow_step_name') WHEN 'true' THEN json('true') WHEN 'false' THEN json('false') ELSE json_extract(NEW.metadata, '$.workflow_step_name') END) ELSE json_object() END,
			'sender_task_id',CASE WHEN json_valid(NEW.metadata) AND typeof(json_extract(NEW.metadata,'$.sender_task_id')) = 'text' THEN json_extract(NEW.metadata,'$.sender_task_id') END)
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'message.updated', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_message_versions
		ON conversation_message_versions.session_id = conversation_session_streams.session_id
		AND conversation_message_versions.message_id = NEW.id
		AND conversation_message_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;
DROP TRIGGER IF EXISTS conversation_message_delete;

CREATE TRIGGER IF NOT EXISTS conversation_message_delete
BEFORE DELETE ON task_session_messages
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (OLD.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_message_versions(session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload)
	SELECT OLD.task_session_id, OLD.id, watermark, NULLIF(OLD.task_id, ''), OLD.author_type, OLD.created_at, TRUE,
		json_object('type','message.deleted','session_id',OLD.task_session_id,'task_id',NULLIF(OLD.task_id,''),'message_id',OLD.id)
	FROM conversation_session_streams WHERE session_id = OLD.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT OLD.task_session_id, watermark, OLD.task_session_id || ':' || watermark, 'message.deleted', NULLIF(OLD.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_message_versions
		ON conversation_message_versions.session_id = conversation_session_streams.session_id
		AND conversation_message_versions.message_id = OLD.id
		AND conversation_message_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = OLD.task_session_id;
END;
DROP TRIGGER IF EXISTS conversation_turn_insert;

CREATE TRIGGER IF NOT EXISTS conversation_turn_insert
AFTER INSERT ON task_session_turns
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_turn_versions(session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.started_at, FALSE,
		json_object('type','session.turn.started','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'id',NEW.id,'started_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.started_at),
			'completed_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.completed_at),
			'created_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.created_at),
			'updated_at',strftime('%Y-%m-%dT%H:%M:%fZ', COALESCE(NEW.updated_at,NEW.started_at)))
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'session.turn.started', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_turn_versions
		ON conversation_turn_versions.session_id = conversation_session_streams.session_id
		AND conversation_turn_versions.turn_id = NEW.id
		AND conversation_turn_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;
DROP TRIGGER IF EXISTS conversation_turn_complete;

CREATE TRIGGER IF NOT EXISTS conversation_turn_complete
AFTER UPDATE OF completed_at ON task_session_turns
WHEN OLD.completed_at IS NULL AND NEW.completed_at IS NOT NULL
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_turn_versions(session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.started_at, FALSE,
		json_object('type','session.turn.completed','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'id',NEW.id,'started_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.started_at),
			'completed_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.completed_at),
			'created_at',strftime('%Y-%m-%dT%H:%M:%fZ', NEW.created_at),
			'updated_at',strftime('%Y-%m-%dT%H:%M:%fZ', COALESCE(NEW.updated_at,NEW.completed_at,NEW.started_at)),
			'metadata',CASE WHEN json_valid(NEW.metadata) THEN json(NEW.metadata) ELSE json('{}') END,
			'had_output',json(CASE WHEN EXISTS (
				SELECT 1 FROM task_session_messages output
				WHERE output.turn_id = NEW.id AND output.author_type = 'agent'
				AND (
					output.type IN ('tool_call','tool_edit','tool_read','tool_search','tool_execute',
						'agent_plan','todo','permission_request','clarification_request')
					OR (output.type IN ('message','content','') AND trim(output.content) <> '')
				)) THEN 'true' ELSE 'false' END))
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'session.turn.completed', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_turn_versions
		ON conversation_turn_versions.session_id = conversation_session_streams.session_id
		AND conversation_turn_versions.turn_id = NEW.id
		AND conversation_turn_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;

DROP TRIGGER IF EXISTS conversation_turn_delete;
CREATE TRIGGER IF NOT EXISTS conversation_turn_delete
AFTER DELETE ON task_session_turns
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (OLD.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_turn_versions(session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload)
	SELECT OLD.task_session_id, OLD.id, watermark, NULLIF(OLD.task_id, ''), OLD.started_at, TRUE,
		json_object('type','session.turn.removed','session_id',OLD.task_session_id,'task_id',NULLIF(OLD.task_id,''),'id',OLD.id)
	FROM conversation_session_streams WHERE session_id = OLD.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT OLD.task_session_id, watermark, OLD.task_session_id || ':' || watermark, 'session.turn.removed', NULLIF(OLD.task_id,''),
		payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_turn_versions
		ON conversation_turn_versions.session_id = conversation_session_streams.session_id
		AND conversation_turn_versions.turn_id = OLD.id
		AND conversation_turn_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = OLD.task_session_id;
END;

DROP TRIGGER IF EXISTS conversation_session_delete;
CREATE TRIGGER IF NOT EXISTS conversation_session_delete
AFTER DELETE ON task_sessions
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (OLD.id, 1, TRUE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, terminal = TRUE, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT OLD.id, watermark, OLD.id || ':' || watermark, 'session.removed', NULLIF(OLD.task_id,''),
		json_object('type','session.removed','session_id',OLD.id,'task_id',NULLIF(OLD.task_id,'')), CURRENT_TIMESTAMP
	FROM conversation_session_streams WHERE session_id = OLD.id;
END;
