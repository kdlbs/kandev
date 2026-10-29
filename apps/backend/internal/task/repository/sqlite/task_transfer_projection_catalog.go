package sqlite

type transferWorkspaceOwnership uint8

const transferWorkspaceCaptured transferWorkspaceOwnership = 1

type transferWorkspaceProjection struct {
	table              string
	taskColumn         string
	identityColumn     string
	receiptKey         string
	unmappedOwner      bool
	workspaceOwnership transferWorkspaceOwnership
}

var transferWorkspaceProjections = []transferWorkspaceProjection{
	{table: "task_status_summaries", taskColumn: "task_id"},
	{table: "task_message_attachments", taskColumn: "task_id"},
	{table: "github_pr_watches", taskColumn: "task_id"},
	{table: "github_task_prs", taskColumn: "task_id"},
	{table: "task_delivery_ledger", taskColumn: "task_id"},
	{table: "azure_devops_task_work_items", taskColumn: "task_id"},
	{table: "automation_task_cleanup_jobs", taskColumn: "task_id"},
	{table: "storage_quarantine_entries", taskColumn: "task_id"},
	{table: "office_channels", taskColumn: "task_id"},
	{table: "task_workspace_groups", taskColumn: "owner_task_id"},
	{table: "exact_task_command_operations", taskColumn: "task_id"},
	{table: "task_completion_gate_history", taskColumn: "task_id"},
	{table: "task_completion_gate_operations", taskColumn: "task_id"},
	{table: "task_completion_sets", taskColumn: "task_id"},
	{table: "task_management_claims", taskColumn: "task_id"},
	{table: "task_management_claim_history", taskColumn: "task_id"},
	{table: "office_deferred_assignments", taskColumn: "task_id"},
	{table: "plugin_instances", taskColumn: "task_id"},
	{table: "canvas_lifecycle_metadata", taskColumn: "task_id"},
}

var transferPreservationTables = []transferWorkspaceProjection{
	{table: "task_plans", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_plan_revisions", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_walkthroughs", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_documents", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_document_revisions", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_session_messages", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_session_turns", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_sessions", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_repositories", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_workspace_folders", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_environments", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_review_runs", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_review_findings", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_status_summaries", taskColumn: "task_id", identityColumn: "task_id"},
	{table: "task_message_attachments", taskColumn: "task_id", identityColumn: "id"},
	{table: "github_pr_watches", taskColumn: "task_id", identityColumn: "id"},
	{table: "github_task_prs", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_delivery_ledger", taskColumn: "task_id", identityColumn: "id"},
	{table: "azure_devops_task_prs", taskColumn: "task_id", identityColumn: "id"},
	{table: "azure_devops_task_work_items", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_usage_events", taskColumn: "task_id", identityColumn: "id"},
	{table: "office_cost_events", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_comments", taskColumn: "task_id", identityColumn: "id"},
	{table: "exact_task_command_operations", taskColumn: "task_id", identityColumn: "operation_id"},
	{table: "task_completion_gate_history", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_completion_gate_operations", taskColumn: "task_id", identityColumn: "operation_id"},
	{table: "task_completion_sets", taskColumn: "task_id", identityColumn: "task_id"},
	{table: "task_management_claims", taskColumn: "task_id", identityColumn: "task_id"},
	{table: "task_management_claim_history", taskColumn: "task_id", identityColumn: "id"},
	{table: "office_deferred_assignments", taskColumn: "task_id", identityColumn: "task_id"},
	{table: "pending_moves", taskColumn: "task_id", identityColumn: "id"},
	{table: "workflow_step_participants", taskColumn: "task_id", identityColumn: "id"},
	{table: "workflow_step_decisions", taskColumn: "task_id", identityColumn: "id"},
	{table: "office_task_labels", taskColumn: "task_id", identityColumn: "label_id"},
	{table: "task_workspace_group_members", taskColumn: "task_id", identityColumn: "workspace_group_id"},
	{table: "task_blockers", taskColumn: "task_id", identityColumn: "blocker_task_id"},
	{table: "task_blockers", taskColumn: "blocker_task_id", identityColumn: "task_id", receiptKey: "task_blockers_as_blocker"},
	{table: "plugin_instances", taskColumn: "task_id", identityColumn: "id"},
	{table: "canvas_lifecycle_metadata", taskColumn: "task_id", identityColumn: "id"},
	// Captured accounting ownership survives live task placement changes.
	{table: "session_usage_measurements", taskColumn: "task_id", identityColumn: "id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "session_usage_buckets", taskColumn: "task_id", identityColumn: "id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_execution_bindings", taskColumn: "task_id", identityColumn: "binding_id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_collection_bindings", taskColumn: "task_id", identityColumn: "binding_id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_legacy_adoptions", taskColumn: "task_id", identityColumn: "session_id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_history_migration_gaps", taskColumn: "task_id", identityColumn: "record_key", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_preservation_fences", taskColumn: "task_id", identityColumn: "fence_id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_subjects", taskColumn: "task_id", identityColumn: "subject_id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_collection_current_targets", taskColumn: "task_id", identityColumn: "subject_id", workspaceOwnership: transferWorkspaceCaptured},
	{table: "usage_canonical_facts", taskColumn: "task_id", identityColumn: "fact_key", workspaceOwnership: transferWorkspaceCaptured},
}
