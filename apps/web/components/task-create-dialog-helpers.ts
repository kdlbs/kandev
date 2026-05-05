import type { useRouter } from "next/navigation";
import type { Task, Branch, LocalRepository, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AppState } from "@/lib/state/store";
import type { StepType, TaskRepoRow } from "@/components/task-create-dialog-types";
import { selectPreferredBranch } from "@/lib/utils";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { linkToTask } from "@/lib/links";
import { INTENT_PLAN } from "@/lib/state/layout-manager";
import { createTask } from "@/lib/api";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { MessageAttachment } from "@/lib/services/session-launch-service";

type CreateTaskParams = Parameters<typeof createTask>[0];

export type { CreateTaskParams };

/** Converts FileAttachment array to MessageAttachment array for the launch request. */
export function toMessageAttachments(
  attachments: FileAttachment[],
): MessageAttachment[] | undefined {
  if (attachments.length === 0) return undefined;
  return attachments.map((att) =>
    att.isImage
      ? { type: "image" as const, data: att.data, mime_type: att.mimeType }
      : {
          type: "resource" as const,
          data: att.data,
          mime_type: att.mimeType,
          name: att.fileName,
        },
  );
}

export function autoSelectBranch(branchList: Branch[], setBranch: (value: string) => void): void {
  const lastUsedBranch = getLocalStorage<string | null>(STORAGE_KEYS.LAST_BRANCH, null);
  if (
    lastUsedBranch &&
    branchList.some((b) => {
      const displayName = b.type === "remote" && b.remote ? `${b.remote}/${b.name}` : b.name;
      return displayName === lastUsedBranch;
    })
  ) {
    setBranch(lastUsedBranch);
    return;
  }
  const preferredBranch = selectPreferredBranch(branchList);
  if (preferredBranch) setBranch(preferredBranch);
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
  };
}

export function validateCreateInputs(inputs: {
  trimmedTitle: string;
  workspaceId: string | null;
  effectiveWorkflowId: string | null;
  /** Unified repos list. The form is valid if any row has a repo set OR URL mode is filled. */
  repositories: TaskRepoRow[];
  githubUrl?: string;
  agentProfileId: string;
}): boolean {
  const hasRepo =
    inputs.repositories.some((r) => r.repositoryId || r.localPath) ||
    Boolean(inputs.githubUrl?.trim());
  return Boolean(
    inputs.trimmedTitle &&
    inputs.workspaceId &&
    inputs.effectiveWorkflowId &&
    inputs.agentProfileId &&
    hasRepo,
  );
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
  useGitHubUrl: boolean;
  githubUrl: string;
  githubBranch: string;
  githubPrHeadBranch: string | null;
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
  if (opts.useGitHubUrl && opts.githubUrl) {
    return [
      {
        repository_id: "",
        base_branch: opts.githubBranch || undefined,
        checkout_branch: opts.githubPrHeadBranch || undefined,
        github_url: opts.githubUrl.trim(),
      },
    ];
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
  if (!args.isLocalExecutor) {
    return { base_branch: args.rowBranch || undefined, checkout_branch: undefined };
  }
  const base = args.defaultBranch || args.rowBranch || undefined;
  const checkout =
    args.rowBranch && args.rowBranch !== args.defaultBranch ? args.rowBranch : undefined;
  return { base_branch: base, checkout_branch: checkout };
}
