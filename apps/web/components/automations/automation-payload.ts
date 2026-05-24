import { createRepositoryAction } from "@/app/actions/workspaces";
import type { ExecutionMode, TriggerType } from "@/lib/types/automation";
import type { RepositorySelection } from "./config-section";

// Shared form state + pending trigger types used by the editor and its
// save handler. Lifted out of automation-editor.tsx so the editor stays
// under the file-length lint cap.

export type FormState = {
  name: string;
  description: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  // repositorySelection captures either a registered workspace repo (id),
  // a discovered local repo (path — registered at save time to obtain an
  // id), or "none" for repo-less automations.
  repositorySelection: RepositorySelection;
  prompt: string;
  taskTitleTemplate: string;
  executionMode: ExecutionMode;
  enabled: boolean;
  maxConcurrentRuns: number;
};

export type PendingTrigger = {
  tempId: string;
  type: TriggerType;
  config: Record<string, unknown>;
  enabled: boolean;
};

// resolveRepositoryId turns a RepositorySelection into a concrete
// repository_id, registering a discovered local repo with the workspace
// first when needed. Empty string for "none" — the orchestrator runs the
// task without a repository, which is the right choice for notification-
// style or side-effect-only automations.
export async function resolveRepositoryId(
  workspaceId: string,
  selection: RepositorySelection,
): Promise<string> {
  if (selection.kind === "none") return "";
  if (selection.kind === "registered") return selection.id;
  const created = await createRepositoryAction({
    workspace_id: workspaceId,
    name: selection.name,
    source_type: "local",
    local_path: selection.path,
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: selection.defaultBranch,
    worktree_branch_prefix: "feature/",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
  });
  return created.id;
}

export function buildCreatePayload(
  workspaceId: string,
  form: FormState,
  repositoryId: string,
  pending: PendingTrigger[],
) {
  return {
    workspace_id: workspaceId,
    name: form.name || "New Automation",
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    repository_id: repositoryId,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    execution_mode: form.executionMode,
    max_concurrent_runs: form.maxConcurrentRuns,
    triggers: pending.map((t) => ({ type: t.type, config: t.config, enabled: t.enabled })),
  };
}

export function buildUpdatePayload(form: FormState, repositoryId: string) {
  return {
    name: form.name,
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    repository_id: repositoryId,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    execution_mode: form.executionMode,
    enabled: form.enabled,
    max_concurrent_runs: form.maxConcurrentRuns,
  };
}

export function buildWebhookUrl(automationId: string): string {
  if (typeof window === "undefined") return `/api/v1/automations/webhook/${automationId}`;
  return `${window.location.origin}/api/v1/automations/webhook/${automationId}`;
}

export type CreatedWebhookDetails = { url: string; secret: string };
