import { createRepositoryAction } from "@/app/actions/workspaces";
import type {
  CreateAutomationRequest,
  AutomationRepository,
  ContinuationPolicy,
  RepositoryMode,
  TaskMode,
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
  taskMode: TaskMode;
  repositoryMode: RepositoryMode;
  // repositorySelections captures an ordered list of registered workspace
  // repos (id), discovered local repos (path — registered at save time to
  // obtain an id), or an empty list for repo-less automations.
  repositorySelections: RepositorySelection[];
  prompt: string;
  taskTitleTemplate: string;
  enabled: boolean;
  maxConcurrentRuns: number;
  continuationPolicy: ContinuationPolicy;
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
  repositories: AutomationRepository[];
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
      ? ({
          kind: "registered",
          id: resolvedIds[i],
          key: selection.key,
          branch: selection.branch || selection.defaultBranch,
        } as const)
      : selection,
  );
  const repositories = resolvedIds.flatMap((repositoryId, index) => {
    if (!repositoryId) return [];
    const selection = selections[index];
    const baseBranch =
      selection?.branch || (selection?.kind === "discovered" ? selection.defaultBranch : "");
    return [{ repository_id: repositoryId, base_branch: baseBranch }];
  });
  return {
    ids: repositories.map((repository) => repository.repository_id),
    repositories,
    selections: promoted,
  };
}

// resolveNormalizedRepositoryIds is the save-boundary entry point used by
// useSaveHandler (automation-editor.tsx). It enforces the single-repository
// invariant (see normalizeRepositorySelections) before resolving, so a form
// that had 2+ repos selected under a compatible executor, then switched to
// an incompatible executor without touching the picker, can't send stale
// repository entries. Only the first survives. Kept as a named function (not
// inlined at the call site) deliberately: it's the test seam this module's
// unit tests exercise directly, without needing a full AutomationEditor
// render harness.
export async function resolveNormalizedRepositoryIds(
  workspaceId: string,
  selections: RepositorySelection[],
  mode: { supportsMultiRepo: boolean },
): Promise<ResolvedRepositories> {
  return resolveRepositoryIds(workspaceId, normalizeRepositorySelections(selections, mode));
}

export async function resolveRepositoryIdsForMode(
  workspaceId: string,
  selections: RepositorySelection[],
  repositoryMode: RepositoryMode,
  mode: { supportsMultiRepo: boolean },
): Promise<ResolvedRepositories> {
  if (repositoryMode !== "selected") {
    return { ids: [], repositories: [], selections: [] };
  }
  return resolveNormalizedRepositoryIds(workspaceId, selections, mode);
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
  repositories: AutomationRepository[],
  pending: PendingTrigger[],
): CreateAutomationRequest {
  // i18n-exempt: persisted automation name. See the comment below.
  return {
    workspace_id: workspaceId,
    // Persisted as the automation's name — user data, so it stays English
    // rather than writing a locale-dependent value into the record. (Unreachable
    // in practice: canSave requires a non-empty name.)
    name: form.name || "New Automation",
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: "",
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    task_mode: form.taskMode,
    repository_mode: repositories.length > 0 ? "selected" : "none",
    repository_ids: repositories.map((repository) => repository.repository_id),
    repositories,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    max_concurrent_runs: form.maxConcurrentRuns,
    continuation_policy: form.continuationPolicy,
    triggers: pending.map((t) => ({ type: t.type, config: t.config, enabled: t.enabled })),
  };
}

export function buildUpdatePayload(
  form: FormState,
  repositories: AutomationRepository[],
): UpdateAutomationRequest {
  return {
    name: form.name,
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: "",
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    task_mode: form.taskMode,
    repository_mode: repositories.length > 0 ? "selected" : "none",
    repository_ids: repositories.map((repository) => repository.repository_id),
    repositories,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    enabled: form.enabled,
    max_concurrent_runs: form.maxConcurrentRuns,
    continuation_policy: form.continuationPolicy,
  };
}

export function buildWebhookUrl(automationId: string): string {
  if (typeof window === "undefined") return `/api/v1/automations/webhook/${automationId}`;
  return `${window.location.origin}/api/v1/automations/webhook/${automationId}`;
}

export type CreatedWebhookDetails = { url: string; secret: string };
