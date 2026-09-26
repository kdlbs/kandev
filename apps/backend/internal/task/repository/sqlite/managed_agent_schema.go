package sqlite

import "fmt"

type managedAgentSchemaStep struct {
	name string
	ddl  string
}

func (r *Repository) initManagedAgentSchema() error {
	for _, step := range managedAgentSchemaSteps() {
		if err := r.migrate.Apply(step.name, step.ddl); err != nil {
			return fmt.Errorf("managed agent schema %s: %w", step.name, err)
		}
	}
	if err := r.migrate.Err(); err != nil {
		return fmt.Errorf("required managed agent schema migration: %w", err)
	}
	return nil
}

func managedAgentSchemaSteps() []managedAgentSchemaStep {
	steps := make([]managedAgentSchemaStep, 0, 13)
	steps = append(steps, managedAgentBindingSchemaSteps()...)
	steps = append(steps, managedAgentOperationSchemaSteps()...)
	steps = append(steps, managedAgentOperationResultSchemaSteps()...)
	steps = append(steps, managedAgentCompletionDeliverySchemaSteps()...)
	steps = append(steps, managedAgentStreamSchemaSteps()...)
	steps = append(steps, managedAgentGrantSchemaSteps()...)
	return steps
}

func managedAgentOperationResultSchemaSteps() []managedAgentSchemaStep {
	return []managedAgentSchemaStep{{
		name: "managed_agent_operations.result_snapshot",
		ddl:  `ALTER TABLE managed_agent_operations ADD COLUMN result_snapshot TEXT NOT NULL DEFAULT '{}'`,
	}}
}

func managedAgentCompletionDeliverySchemaSteps() []managedAgentSchemaStep {
	return []managedAgentSchemaStep{{
		name: "managed_agent_operations.completion_pending",
		ddl:  `ALTER TABLE managed_agent_operations ADD COLUMN completion_pending INTEGER NOT NULL DEFAULT 0`,
	}}
}

func managedAgentBindingSchemaSteps() []managedAgentSchemaStep {
	return []managedAgentSchemaStep{
		{
			name: "managed_agent_bindings.table",
			ddl: `CREATE TABLE IF NOT EXISTS managed_agent_bindings (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				task_id TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				user_id TEXT NOT NULL,
				execution_id TEXT NOT NULL,
				provider_kind TEXT NOT NULL,
				executor_id TEXT NOT NULL,
				executor_profile_id TEXT NOT NULL,
				credential_ref TEXT NOT NULL,
				remote_agent_id TEXT NOT NULL,
				lifecycle TEXT NOT NULL,
				repository_id TEXT NOT NULL,
				repository_url TEXT NOT NULL,
				starting_ref TEXT NOT NULL,
				model TEXT NOT NULL,
				callback_url TEXT NOT NULL,
				auto_create_pr INTEGER NOT NULL DEFAULT 0,
				revision BIGINT NOT NULL DEFAULT 1,
				dispatch_generation BIGINT NOT NULL DEFAULT 0,
				dispatch_owner TEXT NOT NULL DEFAULT '',
				dispatch_lease_until TIMESTAMP,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE RESTRICT
			)`},
		{
			name: "managed_agent_bindings.session_index",
			ddl:  `CREATE UNIQUE INDEX IF NOT EXISTS uniq_managed_agent_bindings_session ON managed_agent_bindings(session_id)`,
		},
		{
			name: "managed_agent_bindings.remote_agent_index",
			ddl:  `CREATE UNIQUE INDEX IF NOT EXISTS uniq_managed_agent_bindings_remote_agent ON managed_agent_bindings(remote_agent_id)`,
		},
	}
}

func managedAgentOperationSchemaSteps() []managedAgentSchemaStep {
	return []managedAgentSchemaStep{
		{
			name: "managed_agent_operations.table",
			ddl: `CREATE TABLE IF NOT EXISTS managed_agent_operations (
				id TEXT PRIMARY KEY,
				binding_id TEXT NOT NULL,
				prompt_turn_id TEXT NOT NULL,
				operation_kind TEXT NOT NULL,
				request_digest TEXT NOT NULL,
				request_snapshot TEXT NOT NULL,
				submission_state TEXT NOT NULL,
				remote_run_id TEXT NOT NULL DEFAULT '',
				pre_submit_run_id TEXT NOT NULL DEFAULT '',
				dispatch_generation BIGINT NOT NULL,
				revision BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL,
				dispatch_started_at TIMESTAMP,
				accepted_at TIMESTAMP,
				settled_at TIMESTAMP,
				updated_at TIMESTAMP NOT NULL,
				sanitized_error TEXT NOT NULL DEFAULT '',
				FOREIGN KEY (binding_id) REFERENCES managed_agent_bindings(id) ON DELETE CASCADE
			)`},
		{
			name: "managed_agent_operations.prompt_turn_index",
			ddl:  `CREATE UNIQUE INDEX IF NOT EXISTS uniq_managed_agent_operations_prompt_turn ON managed_agent_operations(prompt_turn_id)`,
		},
		{
			name: "managed_agent_operations.active_binding_index",
			ddl: `CREATE UNIQUE INDEX IF NOT EXISTS uniq_managed_agent_operations_active_binding
				ON managed_agent_operations(binding_id)
				WHERE submission_state IN ('reserved', 'submitting', 'accepted', 'cancelling', 'unknown')`,
		},
	}
}

func managedAgentStreamSchemaSteps() []managedAgentSchemaStep {
	return []managedAgentSchemaStep{
		{
			name: "managed_agent_streams.table",
			ddl: `CREATE TABLE IF NOT EXISTS managed_agent_streams (
				binding_id TEXT NOT NULL,
				remote_run_id TEXT NOT NULL,
				last_event_id TEXT NOT NULL DEFAULT '',
				cursor TEXT NOT NULL DEFAULT '',
				terminal_event_type TEXT NOT NULL DEFAULT '',
				history_gap INTEGER NOT NULL DEFAULT 0,
				dispatch_generation BIGINT NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				PRIMARY KEY (binding_id, remote_run_id),
				FOREIGN KEY (binding_id) REFERENCES managed_agent_bindings(id) ON DELETE CASCADE
			)`},
		{
			name: "managed_agent_stream_events.table",
			ddl: `CREATE TABLE IF NOT EXISTS managed_agent_stream_events (
				binding_id TEXT NOT NULL,
				remote_run_id TEXT NOT NULL,
				event_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				seen_at TIMESTAMP NOT NULL,
				PRIMARY KEY (binding_id, remote_run_id, event_id, event_type),
				FOREIGN KEY (binding_id) REFERENCES managed_agent_bindings(id) ON DELETE CASCADE
			)`},
		{
			name: "managed_agent_streams.assistant_message_started",
			ddl:  `ALTER TABLE managed_agent_streams ADD COLUMN assistant_message_started INTEGER NOT NULL DEFAULT 0`,
		},
	}
}

func managedAgentGrantSchemaSteps() []managedAgentSchemaStep {
	return []managedAgentSchemaStep{
		{
			name: "managed_agent_tool_grants.table",
			ddl: `CREATE TABLE IF NOT EXISTS managed_agent_tool_grants (
				id TEXT PRIMARY KEY,
				binding_id TEXT NOT NULL,
				operation_id TEXT NOT NULL,
				token_hash TEXT NOT NULL,
				scope_snapshot TEXT NOT NULL,
				generation BIGINT NOT NULL,
				expires_at TIMESTAMP NOT NULL,
				revoked_at TIMESTAMP,
				created_at TIMESTAMP NOT NULL,
				FOREIGN KEY (binding_id) REFERENCES managed_agent_bindings(id) ON DELETE CASCADE,
				FOREIGN KEY (operation_id) REFERENCES managed_agent_operations(id) ON DELETE CASCADE
			)`},
		{
			name: "managed_agent_tool_grants.token_index",
			ddl:  `CREATE UNIQUE INDEX IF NOT EXISTS uniq_managed_agent_tool_grants_token_hash ON managed_agent_tool_grants(token_hash)`,
		},
		{
			name: "managed_agent_tool_grants.operation_index",
			ddl:  `CREATE INDEX IF NOT EXISTS idx_managed_agent_tool_grants_operation ON managed_agent_tool_grants(operation_id, revoked_at, expires_at)`,
		},
	}
}
