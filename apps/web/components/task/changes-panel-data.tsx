"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useToast } from "@/components/toast-provider";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import type { PRChangedFile } from "./changes-panel-timeline";
import type { PRDiffFile } from "@/lib/types/github";
import { useChangesGitHandlers, useChangesDialogHandlers } from "./changes-panel-hooks";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { useBaseBranchByRepo } from "@/hooks/domains/session/use-base-branch-by-repo";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useActiveTaskPRsWithFiles } from "@/hooks/domains/github/use-active-task-pr-files";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import {
  type ChangedFile,
  computePRGroupStamp,
  computeReviewProgress,
  computeStagedStats,
  getBaseBranchDisplay,
  mapPRFilesToChangedFiles,
  mapToChangedFiles,
  buildPrByRepoMap,
  buildRepoNameById,
} from "./changes-panel-helpers";
import type { OpenDiffOptions } from "./changes-diff-target";

function useChangesPanelStoreData() {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useEnvironmentSessionId();
  const taskTitle = useAppStore((state) => {
    if (!state.tasks.activeTaskId) return undefined;
    return state.kanban.tasks.find((t: { id: string }) => t.id === state.tasks.activeTaskId)?.title;
  });
  const baseBranch = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.base_branch : undefined,
  );
  const existingPrUrl = useAppStore((state) => {
    const taskId = state.tasks.activeTaskId;
    if (!taskId) return undefined;
    // Multi-branch tasks hold N PRs per task. The panel-header "View PR"
    // button is a single-URL surface, so collapse only when there's exactly
    // one PR — otherwise the per-repo buttons (prByRepo) take over and the
    // generic button is hidden to avoid silently linking to a sibling.
    const taskPRs = state.taskPRs.byTaskId[taskId];
    if (Array.isArray(taskPRs) && taskPRs.length === 1) return taskPRs[0]?.pr_url;
    if (Array.isArray(taskPRs) && taskPRs.length > 1) return undefined;
    return state.pendingPrUrlByTaskId.byTaskId[taskId]?.[""];
  });
  return { activeTaskId, activeSessionId, taskTitle, baseBranch, existingPrUrl };
}

type DialogsType = ReturnType<typeof useChangesDialogHandlers> & ReturnType<typeof useVcsDialogs>;

export type ChangesPanelBodyProps = {
  hasAnything: boolean;
  hasUnstaged: boolean;
  hasStaged: boolean;
  hasCommits: boolean;
  hasPRFiles: boolean;
  hasPRCommits: boolean;
  canPush: boolean;
  canCreatePR: boolean;
  existingPrUrl: string | undefined;
  unstagedFiles: ChangedFile[];
  stagedFiles: ChangedFile[];
  prFiles: PRChangedFile[];
  prCommits: {
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    additions: number;
    deletions: number;
  }[];
  commits: {
    commit_sha: string;
    commit_message: string;
    insertions: number;
    deletions: number;
    pushed?: boolean;
    committed_at?: string;
  }[];
  pendingStageFiles: Set<string>;
  reviewedCount: number;
  totalFileCount: number;
  aheadCount: number;
  isLoading: boolean;
  loadingOperation: string | null;
  dialogs: DialogsType;
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (sha: string, repo?: string) => void;
  onOpenReview?: () => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onStageAll: () => void;
  onUnstageAll: () => void;
  onStage: (path: string, repo?: string) => Promise<void>;
  onUnstage: (path: string, repo?: string) => Promise<void>;
  onBulkStage: (paths: string[]) => void;
  onBulkUnstage: (paths: string[]) => void;
  onBulkDiscard: (paths: string[]) => void;
  onPush: () => void;
  onForcePush: () => void;
  stagedFileCount: number;
  stagedAdditions: number;
  stagedDeletions: number;
  onRepoStageAll?: (repo: string) => void;
  onRepoUnstageAll?: (repo: string) => void;
  onRepoCommit?: (repo: string) => void;
  onRepoPush?: (repo: string) => void;
  onRepoCreatePR?: (repo: string) => void;
  repoDisplayName?: (repositoryName: string) => string | undefined;
  perRepoStatus?: Array<{ repository_name: string; ahead: number }>;
  prByRepo?: Record<string, string | undefined>;
};

function usePerRepoCallbacks(
  git: ReturnType<typeof useSessionGit>,
  vcsDialogs: ReturnType<typeof useVcsDialogs>,
  gitHandlers: ReturnType<typeof useChangesGitHandlers>,
) {
  return useMemo(
    () => ({
      onRepoStageAll: (repo: string) => {
        gitHandlers.handleGitOperation(
          () => git.stage(undefined, repo),
          repo ? `Stage all (${repo})` : "Stage all",
        );
      },
      onRepoUnstageAll: (repo: string) => {
        gitHandlers.handleGitOperation(
          () => git.unstage(undefined, repo),
          repo ? `Unstage all (${repo})` : "Unstage all",
        );
      },
      onRepoCommit: (repo: string) => vcsDialogs.openCommitDialog(repo),
      onRepoPush: (repo: string) => gitHandlers.handlePush(repo),
      onRepoCreatePR: (repo: string) => vcsDialogs.openPRDialog(repo),
      onRepoPull: (repo: string) => gitHandlers.handlePull(repo),
      onRepoRebase: (repo: string) => gitHandlers.handleRebase(repo),
      onRepoMerge: (repo: string) => gitHandlers.handleMerge(repo),
    }),
    [git, vcsDialogs, gitHandlers],
  );
}

function useChangesPanelPRData() {
  const { prs, filesByPRKey } = useActiveTaskPRsWithFiles();
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repoNameById = useMemo(() => buildRepoNameById(reposByWorkspace), [reposByWorkspace]);
  const taskHasMultipleRepos = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return false;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return (task?.repositories?.length ?? 0) > 1;
  });
  const taskPR = useActiveTaskPR();
  const refreshKey = taskPR?.last_synced_at ?? null;
  const { commits: prCommitsList } = usePRCommits(
    taskPR?.owner ?? null,
    taskPR?.repo ?? null,
    taskPR?.pr_number ?? null,
    refreshKey,
  );
  const { prFiles, prDiffFiles } = useMemo(() => {
    const merged: PRChangedFile[] = [];
    const flat: PRDiffFile[] = [];
    // Multi-branch tasks open multiple PRs on the same repo — files must
    // split by PR so the agent's two branches don't get conflated under one
    // "kandev" header. We stamp each row with a label unique per PR.
    //
    //  - 1 PR total            → no stamp (single section header is enough)
    //  - Multi-repo, 1 PR/repo → stamp = repo name (legacy behavior)
    //  - Multi-PR per repo     → stamp = "<repo> · <head_branch>" so the
    //                            group label still names the repo but
    //                            disambiguates the branch the PR is on
    //  - Single repo + N PRs   → stamp = head_branch (repo is redundant)
    const prCountByRepoID = new Map<string, number>();
    for (const pr of prs) {
      const key = pr.repository_id ?? "";
      prCountByRepoID.set(key, (prCountByRepoID.get(key) ?? 0) + 1);
    }
    const anyRepoMultiPR = Array.from(prCountByRepoID.values()).some((n) => n > 1);
    const needsStamp = prs.length > 1;
    for (const pr of prs) {
      const key = `${pr.owner}/${pr.repo}/${pr.pr_number}/${pr.last_synced_at ?? ""}`;
      const files = filesByPRKey[key] ?? [];
      const repoName = pr.repository_id ? (repoNameById[pr.repository_id] ?? "") : "";
      const branch = pr.head_branch ?? "";
      const stamp = computePRGroupStamp({
        needsStamp,
        taskHasMultipleRepos,
        anyRepoMultiPR,
        repoName: repoName || `${pr.owner}/${pr.repo}`,
        branch,
        prNumber: pr.pr_number,
      });
      merged.push(...mapPRFilesToChangedFiles(files, stamp));
      flat.push(...files);
    }
    return { prFiles: merged, prDiffFiles: flat };
  }, [prs, filesByPRKey, repoNameById, taskHasMultipleRepos]);
  const hasPRFiles = prFiles.length > 0;
  const hasPRCommits = prCommitsList.length > 0;
  return { prDiffFiles, prCommitsList, hasPRFiles, hasPRCommits, prFiles };
}

function hasCumulativeFiles(files: Record<string, unknown> | null | undefined): boolean {
  return Object.keys(files ?? {}).length > 0;
}

export function useChangesPanelData() {
  const { activeTaskId, activeSessionId, baseBranch, existingPrUrl } = useChangesPanelStoreData();
  const baseBranchByRepo = useBaseBranchByRepo(activeTaskId);
  const git = useSessionGit(activeSessionId);
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const prData = useChangesPanelPRData();
  const vcsDialogs = useVcsDialogs();
  const baseBranchDisplay = useMemo(() => getBaseBranchDisplay(baseBranch), [baseBranch]);
  const unstagedFiles = useMemo(() => mapToChangedFiles(git.unstagedFiles), [git.unstagedFiles]);
  const stagedFiles = useMemo(() => mapToChangedFiles(git.stagedFiles), [git.stagedFiles]);
  const { reviewedCount, totalFileCount } = useMemo(
    () => computeReviewProgress(git.allFiles, git.cumulativeDiff, reviews, prData.prDiffFiles),
    [git.allFiles, git.cumulativeDiff, reviews, prData.prDiffFiles],
  );
  const staged = useMemo(() => computeStagedStats(git.stagedFiles), [git.stagedFiles]);
  const gitHandlers = useChangesGitHandlers(git, toast, baseBranch);
  const localDialogs = useChangesDialogHandlers(git, toast, gitHandlers.handleGitOperation);
  const dialogs = { ...localDialogs, ...vcsDialogs };
  const repoCallbacks = usePerRepoCallbacks(git, vcsDialogs, gitHandlers);
  const repoDisplayName = useRepoDisplayName(activeSessionId);
  const taskPRsForMap = useAppStore((state) =>
    activeTaskId ? state.taskPRs.byTaskId[activeTaskId] : undefined,
  );
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repoNameById = useMemo(() => buildRepoNameById(reposByWorkspace), [reposByWorkspace]);
  const pendingByRepo = useAppStore((state) =>
    activeTaskId ? state.pendingPrUrlByTaskId.byTaskId[activeTaskId] : undefined,
  );
  const prByRepo = useMemo(
    () => buildPrByRepoMap(taskPRsForMap, repoNameById, pendingByRepo),
    [taskPRsForMap, repoNameById, pendingByRepo],
  );
  const walkthroughRequestReady =
    unstagedFiles.length > 0 ||
    stagedFiles.length > 0 ||
    (git.statusLoaded && hasCumulativeFiles(git.cumulativeDiff?.files)) ||
    prData.prFiles.length > 0;
  return {
    activeTaskId,
    activeSessionId,
    git,
    baseBranchDisplay,
    baseBranchByRepo,
    unstagedFiles,
    stagedFiles,
    reviewedCount,
    totalFileCount,
    staged,
    gitHandlers,
    localDialogs,
    dialogs,
    repoCallbacks,
    repoDisplayName,
    prByRepo,
    existingPrUrl,
    walkthroughRequestReady,
    ...prData,
  };
}

type ChangesPanelCallbacks = {
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (sha: string, repo?: string) => void;
  onOpenReview?: () => void;
};

export function buildChangesPanelBodyProps(
  data: ReturnType<typeof useChangesPanelData>,
  callbacks: ChangesPanelCallbacks,
): ChangesPanelBodyProps {
  const { git, gitHandlers, localDialogs, repoCallbacks, staged } = data;
  return {
    hasAnything: git.hasAnything || data.hasPRFiles || data.hasPRCommits,
    hasUnstaged: git.hasUnstaged,
    hasStaged: git.hasStaged,
    hasCommits: git.hasCommits,
    hasPRFiles: data.hasPRFiles,
    hasPRCommits: data.hasPRCommits,
    canPush: git.canPush,
    canCreatePR: git.canCreatePR,
    existingPrUrl: data.existingPrUrl,
    unstagedFiles: data.unstagedFiles,
    stagedFiles: data.stagedFiles,
    prFiles: data.prFiles,
    prCommits: data.prCommitsList,
    commits: git.commits,
    pendingStageFiles: git.pendingStageFiles,
    reviewedCount: data.reviewedCount,
    totalFileCount: data.totalFileCount,
    aheadCount: git.ahead,
    isLoading: git.isLoading,
    loadingOperation: git.loadingOperation,
    dialogs: data.dialogs,
    onOpenDiffFile: callbacks.onOpenDiffFile,
    onEditFile: callbacks.onEditFile,
    onOpenCommitDetail: callbacks.onOpenCommitDetail,
    onRevertCommit: gitHandlers.handleRevertCommit,
    onOpenReview: callbacks.onOpenReview,
    onStageAll: git.stageAll,
    onUnstageAll: git.unstageAll,
    onStage: (path, repo) => git.stageFile([path], repo).then(() => undefined),
    onUnstage: (path, repo) => git.unstageFile([path], repo).then(() => undefined),
    onBulkStage: (paths) => {
      git.stageFile(paths).catch(() => undefined);
    },
    onBulkUnstage: (paths) => {
      git.unstageFile(paths).catch(() => undefined);
    },
    onBulkDiscard: localDialogs.handleBulkDiscardClick,
    onPush: () => gitHandlers.handlePush(),
    onForcePush: () => gitHandlers.handleForcePush(),
    stagedFileCount: staged.stagedFileCount,
    stagedAdditions: staged.stagedAdditions,
    stagedDeletions: staged.stagedDeletions,
    onRepoStageAll: repoCallbacks.onRepoStageAll,
    onRepoUnstageAll: repoCallbacks.onRepoUnstageAll,
    onRepoCommit: repoCallbacks.onRepoCommit,
    onRepoPush: repoCallbacks.onRepoPush,
    onRepoCreatePR: repoCallbacks.onRepoCreatePR,
    repoDisplayName: data.repoDisplayName,
    perRepoStatus: git.perRepoStatus,
    prByRepo: data.prByRepo,
  };
}
