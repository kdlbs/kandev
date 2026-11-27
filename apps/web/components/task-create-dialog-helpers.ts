import { resolveLegacySelections } from "./task-create-dialog-repository-payload";
import { t } from "@/lib/i18n";
import type { BuiltinMCPCreateDraft } from "@/lib/types/builtin-mcp";
import type { useRouter } from "@/lib/routing/client-router";
import type {
  Task,
  Branch,
  Executor,
  TaskPriority,
  InitialWorkspaceLayout,
} from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AppState } from "@/lib/state/store";
import type {
  StepType,
  TaskRemoteRepoRow,
  TaskRepoRow,
  TaskRepositorySelection,
  InitialWorkspaceLayoutMode,
} from "@/components/task-create-dialog-types";
import { hasUnavailablePickerRemoteProvider } from "@/components/task-create-dialog-remote-provider-readiness";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import { selectPreferredBranch } from "@/lib/utils";
import { branchOptionValue } from "@/components/branch-picker-options";
import { createDebugLogger } from "@/lib/debug/log";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { linkToTask } from "@/lib/links";
import { INTENT_PLAN } from "@/lib/state/layout-manager";
import type { createTask } from "@/lib/api";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { MessageAttachment } from "@/lib/services/session-launch-service";

type CreateTaskParams = Parameters<typeof createTask>[0];
const selectionDebug = createDebugLogger("task-create:selection");
const BRANCH_AUTOPICK_DEBUG = "branch-autopick";

export type { CreateTaskParams };

/** Returns true while a selected file still needs its staged upload. */
export function hasPendingAttachmentUploads(attachments: FileAttachment[]): boolean {
  return attachments.some((attachment) => attachment.file && !attachment.attachmentId);
}

/** Converts FileAttachment array to MessageAttachment array for the launch request. */
export function toMessageAttachments(
  attachments: FileAttachment[],
): MessageAttachment[] | undefined {
  if (attachments.length === 0) return undefined;
  return attachments.map((att) => ({
    type: att.isImage ? ("image" as const) : ("resource" as const),
    mime_type: att.mimeType,
    name: att.fileName,
    size_bytes: att.size,
    ...(att.attachmentId ? { attachment_id: att.attachmentId } : { data: att.data ?? "" }),
    ...(att.deliveryMode === "path" && { delivery_mode: "path" as const }),
  }));
}

export function autoSelectBranch(
  branchList: Branch[],
  setBranch: (value: string) => void,
  options: { lastUsedBranch?: string | null; userSettingsLoaded?: boolean } = {},
): void {
  const settingsBranch = options.lastUsedBranch ?? null;
  const settingsValid = isBranchSelectable(branchList, settingsBranch);
  if (settingsBranch && settingsValid) {
    selectionDebug(BRANCH_AUTOPICK_DEBUG, {
      source: "settings:taskCreateLastUsed",
      pick: settingsBranch,
      branch_count: branchList.length,
    });
    setBranch(settingsBranch);
    return;
  }
  if (options.userSettingsLoaded === false) {
    selectionDebug(BRANCH_AUTOPICK_DEBUG, {
      source: "user-settings-loading",
      pick: "-",
      branch_count: branchList.length,
    });
    return;
  }
  const preferredBranch = selectPreferredBranch(branchList);
  selectionDebug(BRANCH_AUTOPICK_DEBUG, {
    source: preferredBranch ? "preferred" : "none",
    pick: preferredBranch ?? "-",
    branch_count: branchList.length,
  });
  if (preferredBranch) setBranch(preferredBranch);
}

function isBranchSelectable(branchList: Branch[], value: string | null | undefined) {
  return Boolean(value && branchList.some((branch) => branchOptionValue(branch) === value));
}

export function computePassthroughProfile(
  agentProfileId: string,
  agentProfiles: AgentProfileOption[],
) {
  if (!agentProfileId) return false;
  return (
    agentProfiles.find((p: AgentProfileOption) => p.id === agentProfileId)?.cli_passthrough === true
  );
}

export function computeEffectiveStepId(
  selectedWorkflowId: string | null,
  workflowId: string | null,
  fetchedSteps: StepType[] | null,
  defaultStepId: string | null,
) {
  return selectedWorkflowId && selectedWorkflowId !== workflowId && fetchedSteps
    ? (fetchedSteps[0]?.id ?? null)
    : defaultStepId;
}

export function computeIsTaskStarted(
  isEditMode: boolean,
  editingTask?: { state?: Task["state"] } | null,
) {
  if (!isEditMode || !editingTask?.state) return false;
  return editingTask.state !== "TODO" && editingTask.state !== "CREATED";
}

/**
 * Whether the executor-profile selector should be offered for editing.
 * Create mode always offers it — there is no task yet for a mutability
 * verdict to apply to. Edit mode defers entirely to the projected
 * `runner_editable`, never to workflow state; an absent projection fails
 * closed.
 */
export function computeRunnerEditable(
  isEditMode: boolean,
  editingTask?: { runnerEditable?: boolean } | null,
): boolean {
  if (!isEditMode) return true;
  return editingTask?.runnerEditable ?? false;
}

/**
 * Machine-readable reason to present when {@link computeRunnerEditable}
 * returns false. Falls back to the same retriable class the backend uses
 * when a projection could not be evaluated.
 */
export function computeRunnerIneligibleReason(
  editingTask?: { runnerIneligibleReason?: string } | null,
): string {
  return editingTask?.runnerIneligibleReason ?? "evaluation_unavailable";
}

/**
 * Maps a projected `runner_ineligible_reason`, or the matching
 * `details.error_code` on a rejected `task.runner` switch, to the i18n key
 * presented to the user. Closed vocabulary; a code this dialog doesn't
 * recognize (a future reason it predates) falls back to the same retriable
 * copy as an evaluation failure rather than an empty message or a raw code.
 */
export const RUNNER_INELIGIBLE_REASON_KEYS: Record<string, string> = {
  task_archived: "task:runnerReasonTaskArchived",
  no_repository: "task:runnerReasonNoRepository",
  multiple_repositories: "task:runnerReasonMultipleRepositories",
  session_exists: "task:runnerReasonSessionExists",
  environment_exists: "task:runnerReasonEnvironmentExists",
  executor_running: "task:runnerReasonExecutorRunning",
  workspace_folder_attached: "task:runnerReasonWorkspaceFolderAttached",
  workspace_path_set: "task:runnerReasonWorkspacePathSet",
  workspace_group_member: "task:runnerReasonWorkspaceGroupMember",
  workspace_binding_not_independent: "task:runnerReasonWorkspaceBindingNotIndependent",
};

export function shouldShowTaskTitleField(
  isCreateMode: boolean,
  isEditMode: boolean,
  isTaskStarted: boolean,
): boolean {
  return isEditMode || (isCreateMode && !isTaskStarted);
}

export type ActivatePlanModeArgs = {
  autoFocus?: boolean;
  sessionId: string;
  taskId: string;
  setActiveDocument: AppState["setActiveDocument"];
  setPlanMode: AppState["setPlanMode"];
  router: ReturnType<typeof useRouter>;
};

export function activatePlanMode({
  autoFocus = true,
  sessionId,
  taskId,
  setActiveDocument,
  setPlanMode,
  router,
}: ActivatePlanModeArgs) {
  setActiveDocument(sessionId, { type: "plan", taskId });
  setPlanMode(sessionId, true);
  useContextFilesStore.getState().addFile(sessionId, { path: "plan:context", name: "Plan" });
  if (autoFocus) router.push(linkToTask(taskId, INTENT_PLAN));
}

export type BuildCreatePayloadArgs = {
  workspaceId: string;
  effectiveWorkflowId: string;
  trimmedTitle: string;
  trimmedDescription: string;
  autoTitle?: boolean;
  repositoriesPayload: CreateTaskParams["repositories"];
  /** Presence-aware ordered source payload. `[]` represents explicit scratch. */
  workspaceSourcesPayload?: CreateTaskParams["workspace_sources"];
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
  withAgent: boolean;
  planMode?: boolean;
  attachments?: MessageAttachment[];
  parentId?: string;
  workspacePath?: string;
  autopilot?: boolean;
  /** Task IDs this task must wait for. */
  blockedBy?: string[];
  relatedTaskIds?: string[];
  priority?: TaskPriority;
  /** Task-only replacements for fixed workflow step agent profiles. */
  workflowAgentOverrides?: Record<string, string>;
  /** Task-scoped MCP additions. */
  mcpServerIds?: string[];
  builtinMCPPreferences?: BuiltinMCPCreateDraft;
  initialWorkspaceLayout?: InitialWorkspaceLayout;
};

function optionalString(value?: string): string | undefined {
  return value || undefined;
}

function nonEmptyArray<T>(value?: T[]): T[] | undefined {
  return value && value.length > 0 ? value : undefined;
}

function nonEmptyRecord<T extends Record<string, string>>(value?: T): T | undefined {
  return value && Object.keys(value).length > 0 ? value : undefined;
}

type CreateTaskStateFields = {
  state: "IN_PROGRESS" | "CREATED";
  start_agent?: boolean;
  prepare_session?: boolean;
};

function buildCreateTaskState(withAgent: boolean): CreateTaskStateFields {
  if (withAgent) return { state: "IN_PROGRESS", start_agent: true };
  return { state: "CREATED", prepare_session: true };
}

type OptionalCreateTaskFields = {
  agent_profile_id?: string;
  executor_id?: string;
  executor_profile_id?: string;
  plan_mode?: boolean;
  attachments?: MessageAttachment[];
  parent_id?: string;
  workspace_path?: string;
  autopilot?: boolean;
};

function buildOptionalCreateTaskFields(args: BuildCreatePayloadArgs): OptionalCreateTaskFields {
  return {
    agent_profile_id: optionalString(args.agentProfileId),
    executor_id: optionalString(args.executorId),
    executor_profile_id: optionalString(args.executorProfileId),
    plan_mode: args.planMode || undefined,
    attachments: args.attachments,
    parent_id: optionalString(args.parentId),
    ...(args.workspaceSourcesPayload === undefined
      ? { workspace_path: optionalString(args.workspacePath) }
      : {}),
    autopilot: args.autopilot || undefined,
  };
}

export function buildCreateTaskPayload(args: BuildCreatePayloadArgs): CreateTaskParams {
  if (args.builtinMCPPreferences && args.builtinMCPPreferences.workspaceId !== args.workspaceId) {
    throw new Error(t("settings:builtinMcp.scopeChanged"));
  }
  const commonPayload = {
    workspace_id: args.workspaceId,
    workflow_id: args.effectiveWorkflowId,
    description: args.trimmedDescription,
    ...createTaskSourceFields(args),
    ...buildCreateTaskState(args.withAgent),
    ...buildOptionalCreateTaskFields(args),
    priority: args.priority ?? "medium",
    workflow_agent_overrides: nonEmptyRecord(args.workflowAgentOverrides),
    initial_workspace_layout: args.initialWorkspaceLayout,
    // Dependencies declared at creation time. With edges present the backend
    // records the requested agent start as a start-when-unblocked intent rather
    // than launching now, so a chain runs in order instead of all at once.
    blocked_by: nonEmptyArray(args.blockedBy),
    related_task_ids: nonEmptyArray(args.relatedTaskIds),
    mcp_server_ids: nonEmptyArray(args.mcpServerIds),
    ...(args.builtinMCPPreferences
      ? { builtin_mcp_preferences: args.builtinMCPPreferences.preferences }
      : {}),
  };
  if (args.autoTitle) return { ...commonPayload, auto_title: true };
  return { ...commonPayload, title: args.trimmedTitle };
}

function createTaskSourceFields(
  args: BuildCreatePayloadArgs,
): Pick<CreateTaskParams, "workspace_sources" | "repositories" | "workspace_path"> {
  if (args.workspaceSourcesPayload !== undefined) {
    return { workspace_sources: args.workspaceSourcesPayload };
  }
  return {
    repositories: args.repositoriesPayload,
    workspace_path: optionalTaskValue(args.workspacePath),
  };
}

function optionalTaskValue<T>(value: T | null | undefined): T | undefined {
  return value || undefined;
}

const PARENT_WORKSPACE_EXECUTOR_TYPES: ReadonlySet<Executor["type"]> = new Set(["worktree"]);

export function resolveSelectedExecutorType(
  executors: Executor[],
  selectedProfileId: string,
): Executor["type"] | null {
  for (const executor of executors) {
    const profile = executor.profiles?.find((candidate) => candidate.id === selectedProfileId);
    if (profile) return profile.executor_type ?? executor.type;
  }
  return null;
}

export function resolveInitialWorkspaceLayoutMode({
  isCreateMode,
  isTaskStarted,
  noRepository,
  repositoryCount,
  executorType,
}: {
  isCreateMode: boolean;
  isTaskStarted: boolean;
  noRepository: boolean;
  repositoryCount: number;
  executorType: Executor["type"] | null;
}): InitialWorkspaceLayoutMode {
  if (
    !isCreateMode ||
    isTaskStarted ||
    noRepository ||
    repositoryCount === 0 ||
    !executorType ||
    !PARENT_WORKSPACE_EXECUTOR_TYPES.has(executorType)
  ) {
    return "unavailable";
  }
  if (repositoryCount > 1) return "multiple-repositories";
  return executorType === "worktree" ? "single-repository" : "unavailable";
}

export function resolveInitialWorkspaceLayout({
  requested,
  mode,
}: {
  requested: InitialWorkspaceLayout;
  mode: InitialWorkspaceLayoutMode;
}): InitialWorkspaceLayout | undefined {
  if (mode === "multiple-repositories") return "task_root";
  if (mode === "single-repository") return requested;
  return undefined;
}

export function validateCreateInputs(inputs: {
  trimmedTitle: string;
  trimmedDescription?: string;
  autoTitle?: boolean;
  workspaceId: string | null;
  effectiveWorkflowId: string | null;
  /** Unified repos list. The form is valid if any row has a repo set OR URL mode is filled. */
  repositories: TaskRepoRow[];
  /** Ordered mixed rows from the current draft. */
  selections?: TaskRepositorySelection[];
  /** Remote URL rows. The form is valid when at least one has a non-empty URL. */
  remoteRepos?: TaskRemoteRepoRow[];
  /** Readiness for provider-backed picker rows. Pasted URLs do not use this gate. */
  remoteProviderReadiness?: import("@/components/task-create-dialog-remote-provider-readiness").TaskRemoteProviderReadinessMap;
  agentProfileId: string;
  noRepository?: boolean;
  sourcePolicyInvalid?: boolean;
}): boolean {
  const selections =
    inputs.selections ?? resolveLegacySelections(inputs.repositories, inputs.remoteRepos ?? []);
  const hasRepo = Boolean(inputs.noRepository || selections.some(selectionHasValue));
  if (
    !inputs.noRepository &&
    hasUnavailablePickerRemoteProvider(selections, inputs.remoteProviderReadiness)
  ) {
    return false;
  }
  if (inputs.sourcePolicyInvalid) return false;
  return Boolean(
    (inputs.autoTitle ? inputs.trimmedDescription : inputs.trimmedTitle) &&
    inputs.workspaceId &&
    inputs.effectiveWorkflowId &&
    inputs.agentProfileId &&
    hasRepo,
  );
}

function selectionHasValue(selection: TaskRepositorySelection): boolean {
  if (selection.kind === "remote") return selection.url.trim() !== "";
  if (selection.kind === "folder") return Boolean(selection.localPath.trim());
  return Boolean(selection.repositoryId || selection.localPath);
}

/**
 * Detects two remote-repo rows that resolve to the same GitHub repository and
 * selected branch.
 *
 * Both plain repo URLs and PR URLs are parsed via `parseGitHubAnyUrl`, so two
 * identical branch are caught. Same-repository rows on distinct branches are
 * valid multi-branch task inputs and must reach the backend.
 *
 * Rows with an empty URL, or a URL that can't be parsed to `owner/repo`
 * (garbage), are skipped — only parseable rows participate in the comparison.
 * Repository identity is case-insensitive on `owner/repo`; branch identity is
 * case-sensitive because Git branch names are case-sensitive.
 *
 * Returns the human-readable label (`owner/repo`, preserving the first row's
 * casing) of the first duplicate found, or `null` when every parseable row is
 * a distinct repo.
 */
export function findDuplicateRemoteRepo(remoteRepos: TaskRemoteRepoRow[]): string | null {
  const seen = new Map<string, string>();
  for (const row of remoteRepos) {
    const parsed = parseGitHubAnyUrl(row.url ?? "", row.providerHost);
    if (!parsed) continue;
    const label = `${parsed.owner}/${parsed.repo}`;
    const key = `${parsed.host ?? "github.com"}/${label.toLowerCase()}\u0000${row.branch}`;
    const existing = seen.get(key);
    if (existing) return existing;
    seen.set(key, label);
  }
  return null;
}

/** Returns the first plugin-owned URL whose authorized descriptor is not ready yet. */
export function findUnresolvedProviderRemote(
  remoteRepos: TaskRemoteRepoRow[],
  matchesProviderURL: (url: string) => boolean,
): TaskRemoteRepoRow | null {
  for (const row of remoteRepos) {
    const url = row.url.trim();
    if (!url || row.provider) continue;
    if (matchesProviderURL(url)) return row;
  }
  return null;
}

/**
 * Builds the repositories payload for task creation from the unified list.
 *
 * - URL mode produces a single entry with `github_url`.
 * - Otherwise each row maps to either a workspace `repository_id` or a
 *   discovered `local_path`. Empty rows are dropped silently so a user
 *   can leave an unfinished chip without blocking submit; duplicate
 *   detection happens on the backend.
 */

export {
  buildRepositoriesPayload,
  buildWorkspaceSourcesPayload,
} from "./task-create-dialog-repository-payload";
