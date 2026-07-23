import type { useRouter } from "@/lib/routing/client-router";
import type { Task, Branch, LocalRepository, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AppState } from "@/lib/state/store";
import type {
  StepType,
  TaskRemoteRepoRow,
  TaskRepoRow,
} from "@/components/task-create-dialog-types";
import type { UsePRInfoByURLResult } from "@/hooks/domains/github/use-pr-info-by-url";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import { selectPreferredBranch } from "@/lib/utils";
import { createDebugLogger } from "@/lib/debug/log";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { linkToTask } from "@/lib/links";
import { INTENT_PLAN } from "@/lib/state/layout-manager";
import { createTask } from "@/lib/api";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { MessageAttachment } from "@/lib/services/session-launch-service";

type CreateTaskParams = Parameters<typeof createTask>[0];
type CreateTaskRepositoryPayload = NonNullable<CreateTaskParams["repositories"]>[number];
type RemoteRepoPRMetadata = {
  headBranch?: string;
  baseBranch?: string;
  number?: number;
};
const selectionDebug = createDebugLogger("task-create:selection");
const BRANCH_AUTOPICK_DEBUG = "branch-autopick";

export type { CreateTaskParams };

/** Converts FileAttachment array to MessageAttachment array for the launch request. */
export function toMessageAttachments(
  attachments: FileAttachment[],
): MessageAttachment[] | undefined {
  if (attachments.length === 0) return undefined;
  return attachments.map((att) =>
    att.isImage
      ? {
          type: "image" as const,
          data: att.data,
          mime_type: att.mimeType,
          name: att.fileName,
          ...(att.deliveryMode === "path" && { delivery_mode: "path" as const }),
        }
      : {
          type: "resource" as const,
          data: att.data,
          mime_type: att.mimeType,
          name: att.fileName,
          delivery_mode: "path" as const,
        },
  );
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
  return Boolean(value && branchList.some((branch) => branchDisplayName(branch) === value));
}

function branchDisplayName(branch: Branch) {
  return branch.type === "remote" && branch.remote
    ? `${branch.remote}/${branch.name}`
    : branch.name;
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

export function shouldShowTaskTitleField(
  isCreateMode: boolean,
  isEditMode: boolean,
  isTaskStarted: boolean,
): boolean {
  return isEditMode || (isCreateMode && !isTaskStarted);
}

export type ActivatePlanModeArgs = {
  sessionId: string;
  taskId: string;
  setActiveDocument: AppState["setActiveDocument"];
  setPlanMode: AppState["setPlanMode"];
  router: ReturnType<typeof useRouter>;
};

export function activatePlanMode({
  sessionId,
  taskId,
  setActiveDocument,
  setPlanMode,
  router,
}: ActivatePlanModeArgs) {
  setActiveDocument(sessionId, { type: "plan", taskId });
  setPlanMode(sessionId, true);
  useContextFilesStore.getState().addFile(sessionId, { path: "plan:context", name: "Plan" });
  router.push(linkToTask(taskId, INTENT_PLAN));
}

export type BuildCreatePayloadArgs = {
  workspaceId: string;
  effectiveWorkflowId: string;
  trimmedTitle: string;
  trimmedDescription: string;
  repositoriesPayload: CreateTaskParams["repositories"];
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
  withAgent: boolean;
  planMode?: boolean;
  attachments?: MessageAttachment[];
  parentId?: string;
  workspacePath?: string;
};

export function buildCreateTaskPayload(args: BuildCreatePayloadArgs): CreateTaskParams {
  return {
    workspace_id: args.workspaceId,
    workflow_id: args.effectiveWorkflowId,
    title: args.trimmedTitle,
    description: args.trimmedDescription,
    repositories: args.repositoriesPayload,
    state: args.withAgent ? "IN_PROGRESS" : "CREATED",
    start_agent: args.withAgent ? true : undefined,
    prepare_session: args.withAgent ? undefined : true,
    agent_profile_id: args.agentProfileId || undefined,
    executor_id: args.executorId || undefined,
    executor_profile_id: args.executorProfileId || undefined,
    plan_mode: args.planMode || undefined,
    attachments: args.attachments,
    parent_id: args.parentId || undefined,
    workspace_path: args.workspacePath || undefined,
  };
}

export function validateCreateInputs(inputs: {
  trimmedTitle: string;
  workspaceId: string | null;
  effectiveWorkflowId: string | null;
  /** Unified repos list. The form is valid if any row has a repo set OR URL mode is filled. */
  repositories: TaskRepoRow[];
  /** Remote URL rows. The form is valid when at least one has a non-empty URL. */
  remoteRepos?: TaskRemoteRepoRow[];
  agentProfileId: string;
  noRepository?: boolean;
}): boolean {
  const hasRemoteRepo = (inputs.remoteRepos ?? []).some((r) => r.url.trim() !== "");
  const hasRepo =
    inputs.noRepository ||
    inputs.repositories.some((r) => r.repositoryId || r.localPath) ||
    hasRemoteRepo;
  return Boolean(
    inputs.trimmedTitle &&
    inputs.workspaceId &&
    inputs.effectiveWorkflowId &&
    inputs.agentProfileId &&
    hasRepo,
  );
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
  /**
   * Optional fresh-branch metadata. The UI gates this to single-row + local
   * executor; when present we apply it to every row (which is at most one).
   */
  freshBranch?: { confirmDiscard: boolean; consentedDirtyFiles: string[] };
}): NonNullable<CreateTaskParams["repositories"]> {
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
    .map((row) => {
      const defaultBranch = resolveRowDefaultBranch(row, opts);
      const branches = splitLocalExecutorBranches({
        rowBranch: row.branch,
        defaultBranch,
        isLocalExecutor,
      });
      if (row.repositoryId) {
        return {
          repository_id: row.repositoryId,
          base_branch: branches.base_branch,
          checkout_branch: branches.checkout_branch,
          ...fresh,
        };
      }
      return {
        repository_id: "",
        base_branch: branches.base_branch,
        checkout_branch: branches.checkout_branch,
        local_path: row.localPath,
        default_branch: defaultBranch || undefined,
        ...fresh,
      };
    });
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
  if (metadata) return buildRemoteRepoPRPayload(row, url, metadata);
  return buildPlainRemoteRepoPayload(row, url);
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
  "github_url" | "remote_url" | "provider" | "provider_repo_id" | "provider_owner" | "provider_name"
> {
  if (!row.provider || row.provider === "github") {
    return { github_url: url };
  }
  return {
    remote_url: url,
    provider: row.provider,
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
function splitLocalExecutorBranches(args: {
  rowBranch?: string;
  defaultBranch?: string;
  isLocalExecutor: boolean;
}): { base_branch: string | undefined; checkout_branch: string | undefined } {
  // Without a known default_branch we can't anchor base_branch to the
  // integration ref — and using rowBranch as a stand-in reproduces the
  // exact bug this PR fixes (changes panel collapses to HEAD on refresh).
  // Fall through to the legacy non-split shape: a workspace repo with an
  // unset default_branch is no worse off than before, and the backend's
  // resolveRepoInput probe will populate it on the next CreateRepository
  // call. Wait for that probe rather than synthesizing a guess here.
  if (!args.isLocalExecutor || !args.defaultBranch) {
    return { base_branch: args.rowBranch || undefined, checkout_branch: undefined };
  }
  const base = args.defaultBranch;
  const checkout =
    args.rowBranch && args.rowBranch !== args.defaultBranch ? args.rowBranch : undefined;
  return { base_branch: base, checkout_branch: checkout };
}
