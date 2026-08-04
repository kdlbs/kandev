import { createRepositoryAction } from "@/app/actions/workspaces";
import type {
  CreateAutomationRequest,
  ExecutionMode,
  TriggerType,
  UpdateAutomationRequest,
} from "@/lib/types/automation";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";
import {
  normalizeRepositorySelections,
  type RepositorySelection,
} from "./automation-repository-selection";

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
  // repositorySelections captures an ordered list of registered workspace
  // repos (id), discovered local repos (path — registered at save time to
  // obtain an id), or an empty list for repo-less automations.
  repositorySelections: RepositorySelection[];
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

// ResolvedRepositories bundles the save-time output of resolving an ordered
// RepositorySelection[]: `ids` is the compact repository_ids payload (in
// order, "none"/empty entries dropped), `selections` is the 1:1-with-input
// list promoted from "discovered" to "registered" wherever a repository was
// newly created — the form re-adopts this so a second save doesn't
// re-register the same local path.
export type ResolvedRepositories = {
  ids: string[];
  selections: RepositorySelection[];
};

// resolveRepositoryIds turns each RepositorySelection into a concrete
// repository_id, in order, registering any discovered local repo with the
// workspace first when needed. The positional zip against `selections`
// happens BEFORE filtering out empty ("none") entries, so promotion always
// lines up with the original array — only the returned `ids` list is
// compacted for the wire payload.
export async function resolveRepositoryIds(
  workspaceId: string,
  selections: RepositorySelection[],
): Promise<ResolvedRepositories> {
  const resolvedIds = await Promise.all(
    selections.map((selection) => resolveOneRepositoryId(workspaceId, selection)),
  );
  const promoted = selections.map((selection, i) =>
    selection.kind === "discovered" && resolvedIds[i]
      ? ({ kind: "registered", id: resolvedIds[i] } as const)
      : selection,
  );
  return { ids: resolvedIds.filter((id) => id !== ""), selections: promoted };
}

// resolveNormalizedRepositoryIds is the save-boundary entry point used by
// useSaveHandler (automation-editor.tsx). It enforces the single-repository
// invariant (see normalizeRepositorySelections) before resolving, so a form
// that had 2+ repos selected under a compatible executor, then switched to
// an incompatible executor or a github_pr trigger without the picker being
// touched again, can't smuggle every stale entry into the saved
// repository_ids — only the first survives. Kept as a named function (not
// inlined at the call site) deliberately: it's the test seam this module's
// unit tests exercise directly, without needing a full AutomationEditor
// render harness.
export async function resolveNormalizedRepositoryIds(
  workspaceId: string,
  selections: RepositorySelection[],
  mode: { supportsMultiRepo: boolean; isPRTrigger: boolean },
): Promise<ResolvedRepositories> {
  return resolveRepositoryIds(workspaceId, normalizeRepositorySelections(selections, mode));
}

async function resolveOneRepositoryId(
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
    worktree_branch_template: defaultWorktreeBranchTemplate,
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
  });
  return created.id;
}

export function buildCreatePayload(
  workspaceId: string,
  form: FormState,
  repositoryIds: string[],
  pending: PendingTrigger[],
): CreateAutomationRequest {
  return {
    workspace_id: workspaceId,
    // Persisted as the automation's name — user data, so it stays English
    // rather than writing a locale-dependent value into the record. (Unreachable
    // in practice: canSave requires a non-empty name.)
    name: form.name || "New Automation",
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    repository_ids: repositoryIds,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    execution_mode: form.executionMode,
    max_concurrent_runs: form.maxConcurrentRuns,
    triggers: pending.map((t) => ({ type: t.type, config: t.config, enabled: t.enabled })),
  };
}

export function buildUpdatePayload(
  form: FormState,
  repositoryIds: string[],
): UpdateAutomationRequest {
  return {
    name: form.name,
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    repository_ids: repositoryIds,
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
