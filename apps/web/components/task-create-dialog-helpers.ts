/* eslint-disable max-lines -- task creation payload and source mapping share one request boundary. */

import type { useRouter } from "@/lib/routing/client-router";
import type { Task, Branch, LocalRepository, Repository, TaskPriority } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AppState } from "@/lib/state/store";
import type {
  StepType,
  TaskRemoteRepoRow,
  TaskRepoRow,
  TaskRepositorySelection,
} from "@/components/task-create-dialog-types";
import { hasUnavailablePickerRemoteProvider } from "@/components/task-create-dialog-remote-provider-readiness";
import type { UsePRInfoByURLResult } from "@/hooks/domains/github/use-pr-info-by-url";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import { selectPreferredBranch } from "@/lib/utils";
import { branchOptionValue } from "@/components/branch-picker-options";
import { splitLocalExecutorBranches } from "@/components/task-create-dialog-branch-utils";
import { createDebugLogger } from "@/lib/debug/log";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { linkToTask } from "@/lib/links";
import { INTENT_PLAN } from "@/lib/state/layout-manager";
import { createTask } from "@/lib/api";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { MessageAttachment } from "@/lib/services/session-launch-service";
import type { WorkspaceSourceRequest } from "@/lib/types/http-workspace-sources";

type CreateTaskParams = Parameters<typeof createTask>[0];
type CreateTaskRepositoryPayload = NonNullable<CreateTaskParams["repositories"]>[number];
type RemoteRepoPRMetadata = {
  headBranch?: string;
  baseBranch?: string;
  number?: number;
};
type FreshRepositoryPayload = Partial<{
  fresh_branch: boolean;
  new_branch_name: string;
  confirm_discard: boolean;
  consented_dirty_files: string[];
}>;
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
  priority?: TaskPriority;
  /** Task-only replacements for fixed workflow step agent profiles. */
  workflowAgentOverrides?: Record<string, string>;
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
  const commonPayload = {
    workspace_id: args.workspaceId,
    workflow_id: args.effectiveWorkflowId,
    description: args.trimmedDescription,
    ...createTaskSourceFields(args),
    ...buildCreateTaskState(args.withAgent),
    ...buildOptionalCreateTaskFields(args),
    priority: args.priority ?? "medium",
    workflow_agent_overrides: nonEmptyRecord(args.workflowAgentOverrides),
    // Dependencies declared at creation time. With edges present the backend
    // records the requested agent start as a start-when-unblocked intent rather
    // than launching now, so a chain runs in order instead of all at once.
    blocked_by: nonEmptyArray(args.blockedBy),
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

function resolveLegacySelections(
  repositories: TaskRepoRow[],
  remoteRepos: TaskRemoteRepoRow[],
): TaskRepositorySelection[] {
  return [
    ...repositories.map((row) => ({ kind: "local" as const, ...row })),
    ...remoteRepos.map((row) => ({ kind: "remote" as const, ...row })),
  ];
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
    const parsed = parseGitHubAnyUrl(row.url ?? "");
    if (!parsed) continue;
    const label = `${parsed.owner}/${parsed.repo}`;
    const key = `${label.toLowerCase()}\u0000${row.branch}`;
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
export function buildRepositoriesPayload(opts: {
  /** Ordered mixed rows from the current task draft. */
  selections?: TaskRepositorySelection[];
  /** True when the form is in GitHub Remote (URL) mode. */
  useRemote: boolean;
  /** Remote-URL rows; non-empty `url` rows are mapped 1:1 to payload entries. */
  remoteRepos: TaskRemoteRepoRow[];
  /**
   * Per-URL PR-info cache. Consulted for each remote row whose URL is a PR
   * URL: if the row's branch equals the PR head (auto-fill or user-confirmed
   * default), the payload anchors `base_branch` to the PR's actual target
   * from the API so origin can resolve it even when the head only lives on
   * a fork. Optional — non-Remote call sites can omit it.
   */
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">;
  repositories: TaskRepoRow[];
  /** Used to look up `default_branch` for `localPath` rows. */
  discoveredRepositories: LocalRepository[];
  /** Workspace repositories — used to look up `default_branch` for `repositoryId` rows. */
  workspaceRepositories?: Repository[];
  /**
   * For the local executor (no worktree), the chip's branch field represents
   * the working branch on disk, not the parent integration branch. Send it as
   * `checkout_branch` so the session's `base_branch` stays anchored to the
   * repo's `default_branch` (which is what git log / cumulative diff use as
   * the merge-base reference). Without this, "new branch on local executor"
   * collapses the merge-base recomputation to HEAD and the changes panel
   * goes empty after a refresh.
   */
  isLocalExecutor?: boolean;
  /** True when the selected executor will materialize local repositories from their origin. */
  remoteOriginMode?: boolean;
  /**
   * Optional fresh-branch metadata. The UI gates this to single-row + local
   * executor; when present we apply it to every row (which is at most one).
   */
  freshBranch?: { confirmDiscard: boolean; consentedDirtyFiles: string[] };
}): NonNullable<CreateTaskParams["repositories"]> {
  if (opts.selections) {
    return buildMixedRepositoryPayload(opts.selections, opts);
  }
  if (opts.useRemote) {
    return buildRemoteRepoPayload(opts);
  }
  const fresh = opts.freshBranch
    ? {
        fresh_branch: true,
        confirm_discard: opts.freshBranch.confirmDiscard,
        consented_dirty_files: opts.freshBranch.consentedDirtyFiles,
      }
    : {};
  // Fresh-branch flow inverts the chip's semantics: instead of "the working
  // branch I'm on", row.branch becomes "the base I want to fork from". The
  // backend then creates a new branch and rewrites repos[i].BaseBranch to it.
  // Splitting here would force `base_branch=default_branch, checkout_branch=
  // <picked-base>` and the backend would fork from the wrong base. Skip the
  // split entirely when fresh-branch is active.
  const isLocalExecutor = !!opts.isLocalExecutor && !opts.freshBranch;
  return opts.repositories
    .filter((row) => row.repositoryId || row.localPath)
    .map((row) => buildLocalRepositoryPayloadRow(row, opts, fresh, isLocalExecutor));
}

function buildMixedRepositoryPayload(
  selections: TaskRepositorySelection[],
  opts: Parameters<typeof buildRepositoriesPayload>[0],
): NonNullable<CreateTaskParams["repositories"]> {
  const fresh = opts.freshBranch
    ? {
        fresh_branch: true,
        confirm_discard: opts.freshBranch.confirmDiscard,
        consented_dirty_files: opts.freshBranch.consentedDirtyFiles,
      }
    : {};
  const isLocalExecutor = !!opts.isLocalExecutor && !opts.freshBranch;
  return selections.flatMap((selection) => {
    if (selection.kind === "folder") return [];
    if (selection.kind === "remote") {
      return selection.url.trim() ? [buildRemoteRepoPayloadRow(selection, opts.prInfoByUrl)] : [];
    }
    if (!selection.repositoryId && !selection.localPath) return [];
    return [buildLocalRepositoryPayloadRow(selection, opts, fresh, isLocalExecutor)];
  });
}

/** Builds the ordered, tagged source payload used by the new task-creation contract. */
export function buildWorkspaceSourcesPayload(
  opts: Parameters<typeof buildRepositoriesPayload>[0],
): WorkspaceSourceRequest[] {
  const selections =
    opts.selections ?? resolveLegacySelections(opts.repositories, opts.remoteRepos);
  const fresh = opts.freshBranch
    ? {
        fresh_branch: true,
        confirm_discard: opts.freshBranch.confirmDiscard,
        consented_dirty_files: opts.freshBranch.consentedDirtyFiles,
      }
    : {};
  const isLocalExecutor = !!opts.isLocalExecutor && !opts.freshBranch;
  return selections.flatMap((selection): WorkspaceSourceRequest[] => {
    if (selection.kind === "folder") {
      const localPath = selection.localPath.trim();
      if (!localPath) return [];
      return [
        {
          kind: "folder" as const,
          local_path: localPath,
          ...(selection.displayName ? { display_name: selection.displayName } : {}),
        },
      ];
    }
    if (selection.kind === "remote") {
      if (!selection.url.trim()) return [];
      const repository = buildRemoteRepoPayloadRow(selection, opts.prInfoByUrl);
      return [workspaceRepositorySource(repository)];
    }
    if (!selection.repositoryId && !selection.localPath) return [];
    const repository = buildLocalRepositoryPayloadRow(selection, opts, fresh, isLocalExecutor);
    return [workspaceRepositorySource(repository)];
  });
}

function workspaceRepositorySource(
  repository: CreateTaskRepositoryPayload,
): WorkspaceSourceRequest & { kind: "repository" } {
  return {
    kind: "repository",
    ...(repository.checkout_options ? { checkout_options: repository.checkout_options } : {}),
    ...repositoryIdentityFields(repository),
    ...repositoryBranchFields(repository),
    ...repositoryFreshBranchFields(repository),
  };
}

type RepositorySourceFields = Partial<
  Omit<Extract<WorkspaceSourceRequest, { kind: "repository" }>, "kind">
>;

function repositoryIdentityFields(repository: CreateTaskRepositoryPayload): RepositorySourceFields {
  return {
    ...(repository.repository_id ? { repository_id: repository.repository_id } : {}),
    ...(repository.local_path ? { local_path: repository.local_path } : {}),
    ...(repository.github_url ? { github_url: repository.github_url } : {}),
    ...(repository.remote_url ? { remote_url: repository.remote_url } : {}),
    ...(repository.provider ? { provider: repository.provider } : {}),
    ...(repository.provider_host ? { provider_host: repository.provider_host } : {}),
    ...(repository.provider_scope ? { provider_scope: repository.provider_scope } : {}),
    ...(repository.provider_repo_id ? { provider_repo_id: repository.provider_repo_id } : {}),
    ...(repository.provider_owner ? { provider_owner: repository.provider_owner } : {}),
    ...(repository.provider_name ? { provider_name: repository.provider_name } : {}),
    ...(repository.checkout_source ? { checkout_source: repository.checkout_source } : {}),
    ...(repository.expected_origin ? { expected_origin: repository.expected_origin } : {}),
  };
}

function repositoryBranchFields(repository: CreateTaskRepositoryPayload): RepositorySourceFields {
  return {
    ...(repository.base_branch ? { base_branch: repository.base_branch } : {}),
    ...(repository.checkout_branch ? { checkout_branch: repository.checkout_branch } : {}),
    ...(repository.branch_policy_id ? { branch_policy_id: repository.branch_policy_id } : {}),
    ...(repository.pr_number ? { pr_number: repository.pr_number } : {}),
  };
}

function repositoryFreshBranchFields(
  repository: CreateTaskRepositoryPayload,
): RepositorySourceFields {
  return {
    ...(repository.fresh_branch ? { fresh_branch: repository.fresh_branch } : {}),
    ...(repository.new_branch_name ? { new_branch_name: repository.new_branch_name } : {}),
    ...(repository.confirm_discard !== undefined
      ? { confirm_discard: repository.confirm_discard }
      : {}),
    ...(repository.consented_dirty_files
      ? { consented_dirty_files: repository.consented_dirty_files }
      : {}),
  };
}

function buildLocalRepositoryPayloadRow(
  row: TaskRepoRow,
  opts: Parameters<typeof buildRepositoriesPayload>[0],
  fresh: FreshRepositoryPayload,
  isLocalExecutor: boolean,
): CreateTaskRepositoryPayload {
  const defaultBranch = resolveRowDefaultBranch(row, opts);
  const branches = opts.freshBranch
    ? { base_branch: row.branch || undefined, checkout_branch: undefined }
    : splitLocalExecutorBranches({
        // Restored local sources can carry both the branch checked out on the
        // host and the saved integration base. Worktree-backed executors use the
        // base while direct local execution keeps the checkout branch.
        rowBranch: isLocalExecutor ? row.branch : row.baseBranch || row.branch,
        defaultBranch,
        baseBranch: row.baseBranch,
        isLocalExecutor,
      });
  if (row.repositoryId) {
    return {
      repository_id: row.repositoryId,
      ...(row.branchPolicyId ? { branch_policy_id: row.branchPolicyId } : {}),
      base_branch: branches.base_branch,
      checkout_branch: branches.checkout_branch,
      ...localRepositoryOriginFields(row, opts),
      ...fresh,
    };
  }
  return {
    repository_id: "",
    base_branch: branches.base_branch,
    checkout_branch: branches.checkout_branch,
    local_path: row.localPath,
    default_branch: defaultBranch || undefined,
    ...localRepositoryOriginFields(row, opts),
    ...fresh,
  };
}

function localRepositoryOriginFields(
  row: TaskRepoRow,
  opts: Parameters<typeof buildRepositoriesPayload>[0],
): Pick<CreateTaskRepositoryPayload, "checkout_source" | "expected_origin"> {
  if (opts.remoteOriginMode === false) return {};
  return {
    ...(row.checkoutSource ? { checkout_source: row.checkoutSource } : {}),
    ...(row.expectedOrigin ? { expected_origin: row.expectedOrigin } : {}),
  };
}

/**
 * Builds the `repos: [{ github_url, branch }]` payload from the remote-URL
 * rows. Rows with an empty URL are dropped silently — they're partially
 * filled rows the user hasn't completed yet.
 *
 * Per-row PR-info inference: if a row's URL is a PR URL and the row's
 * branch equals the PR's head branch (auto-selected by the chip or
 * user-confirmed via "leave default"), the payload anchors `base_branch`
 * to the PR's actual target from the GitHub API and surfaces the PR head
 * as `checkout_branch`. This keeps fork PRs resolvable on `origin` (their
 * head doesn't live there, but the base does). When the user overrides
 * the branch to something other than the PR head, we treat their pick as
 * the base and drop `checkout_branch`.
 */
function buildRemoteRepoPayload(opts: {
  remoteRepos: TaskRemoteRepoRow[];
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">;
}): NonNullable<CreateTaskParams["repositories"]> {
  const nonEmpty = opts.remoteRepos.filter((r) => r.url.trim() !== "");
  if (nonEmpty.length === 0) return [];
  return nonEmpty.map((row) => buildRemoteRepoPayloadRow(row, opts.prInfoByUrl));
}

function buildRemoteRepoPayloadRow(
  row: TaskRemoteRepoRow,
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">,
): CreateTaskRepositoryPayload {
  const url = row.url.trim();
  const metadata = remoteRepoPRMetadata(row, url, prInfoByUrl);
  const payload = metadata
    ? buildRemoteRepoPRPayload(row, url, metadata)
    : buildPlainRemoteRepoPayload(row, url);
  return row.checkoutOptions ? { ...payload, checkout_options: row.checkoutOptions } : payload;
}

function remoteRepoPRMetadata(
  row: TaskRemoteRepoRow,
  url: string,
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">,
): RemoteRepoPRMetadata | null {
  // The cache is keyed on the trimmed URL (ensure() also trims), so we
  // must look it up with the trimmed value too. Passing `row.url` directly
  // would miss the cache when the user has stray whitespace around their
  // URL and silently lose the PR base-branch anchoring.
  const prInfo = prInfoByUrl?.info(url);
  const number = prInfo?.prNumber ?? row.prNumber;
  if (!prInfo && !number) return null;
  return {
    headBranch: prInfo?.prHeadBranch ?? row.prHeadBranch,
    baseBranch: prInfo?.prBaseBranch ?? row.prBaseBranch,
    number,
  };
}

function buildRemoteRepoPRPayload(
  row: TaskRemoteRepoRow,
  url: string,
  metadata: RemoteRepoPRMetadata,
): CreateTaskRepositoryPayload {
  const isPrAutoSelection = !!metadata.headBranch && row.branch === metadata.headBranch;
  const baseBranch = isPrAutoSelection ? metadata.baseBranch || undefined : row.branch || undefined;
  return {
    repository_id: "",
    base_branch: baseBranch,
    checkout_branch: isPrAutoSelection ? metadata.headBranch || undefined : undefined,
    pr_number: isPrAutoSelection ? metadata.number || undefined : undefined,
    ...remoteRepositoryLocator(row, url),
  };
}

function buildPlainRemoteRepoPayload(
  row: TaskRemoteRepoRow,
  url: string,
): CreateTaskRepositoryPayload {
  return {
    repository_id: "",
    base_branch: row.branch || undefined,
    checkout_branch: undefined,
    ...remoteRepositoryLocator(row, url),
  };
}

function remoteRepositoryLocator(
  row: TaskRemoteRepoRow,
  url: string,
): Pick<
  CreateTaskRepositoryPayload,
  | "github_url"
  | "remote_url"
  | "provider"
  | "provider_host"
  | "provider_scope"
  | "provider_repo_id"
  | "provider_owner"
  | "provider_name"
> {
  if (!row.provider || row.provider === "github") {
    return { github_url: url };
  }
  return {
    remote_url: row.remoteUrl?.trim() || url,
    provider: row.provider,
    provider_host: row.providerHost,
    provider_scope: row.providerScope,
    provider_repo_id: row.providerRepoId,
    provider_owner: row.providerOwner,
    provider_name: row.providerName,
  };
}

function resolveRowDefaultBranch(
  row: TaskRepoRow,
  opts: {
    discoveredRepositories: LocalRepository[];
    workspaceRepositories?: Repository[];
  },
): string | undefined {
  if (row.repositoryId) {
    return opts.workspaceRepositories?.find((r) => r.id === row.repositoryId)?.default_branch;
  }
  return opts.discoveredRepositories.find((d) => d.path === row.localPath)?.default_branch;
}

/**
 * For the local executor: return `base_branch=defaultBranch`,
 * `checkout_branch=rowBranch` so the session anchors merge-base to the repo's
 * integration branch while the preparer still checks out the user's working
 * branch. When `rowBranch === defaultBranch` we omit checkout_branch — the
 * preparer treats matching values as "use current state" and skips git ops.
 *
 * For non-local executors (worktree-based): keep the historical shape where
 * `base_branch=rowBranch` (the worktree creates a new branch off of it).
 */
