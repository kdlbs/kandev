"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useToast } from "@/components/toast-provider";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import type { PRChangedFile } from "./changes-panel-timeline";
import { useChangesGitHandlers, useChangesDialogHandlers } from "./changes-panel-hooks";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { useBaseBranchByRepo } from "@/hooks/domains/session/use-base-branch-by-repo";
import {
  useRemoteContributionRelation,
  type RemoteContributionRelationState,
} from "@/hooks/domains/session/use-remote-contribution-relation";
import {
  prFetchKey,
  useActiveTaskPRsWithFiles,
} from "@/hooks/domains/github/use-active-task-pr-files";
import { usePRReviewRepositoryIdentity } from "@/hooks/domains/github/use-pr-review-repository-identity";
import {
  getCumulativeReviewRepositoryNames,
  isReviewMultiRepo,
  resolvePRReviewRepositoryName,
} from "@/components/review/types";
import { prTaskKey } from "@/components/github/pr-utils";
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
  selectPRFilesForReviewProgress,
  type ReviewProgressPRSource,
  type PRCommitForMerge,
} from "./changes-panel-helpers";
import type { CommitDetailTarget, OpenDiffOptions } from "./changes-diff-target";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";
import { gitOperationLabel } from "@/hooks/use-git-with-feedback";
import { getGitCredentialDisplay } from "./changes-git-credential-display";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import {
  remoteContributionActionPolicy,
  remoteContributionActionReasonKey,
} from "@/hooks/domains/session/remote-contribution-relation";
import {
  buildRemoteContributionResolutionTarget,
  type RemoteContributionResolutionTarget,
} from "./use-remote-contribution-resolution";
import { useRemoteContributionResolution } from "./use-remote-contribution-resolution";
import { useTranslation } from "react-i18next";

function useChangesPanelStoreData() {
  const { t } = useTranslation();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useEnvironmentSessionId();
  const taskTitle = useAppStore((state) => {
    if (!state.tasks.activeTaskId) return undefined;
    return state.kanban.tasks.find((t: { id: string }) => t.id === state.tasks.activeTaskId)?.title;
  });
  const baseBranch = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.base_branch : undefined,
  );
  const activeSessionMetadata = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.metadata : undefined,
  );
  // `t` is a dependency even though the helper resolves through the module-level
  // translator: without it the memo returns the labels built under the previous
  // locale and the panel never restates them.
  const gitCredentialDisplay = useMemo(
    () => getGitCredentialDisplay(activeSessionMetadata),
    [activeSessionMetadata, t],
  );
  return {
    activeTaskId,
    activeSessionId,
    taskTitle,
    baseBranch,
    gitCredentialDisplay,
  };
}

type DialogsType = ReturnType<typeof useChangesDialogHandlers> & ReturnType<typeof useVcsDialogs>;

export type ChangesPanelBodyProps = {
  hasAnything: boolean;
  hasUnstaged: boolean;
  hasStaged: boolean;
  hasCommits: boolean;
  hasPRFiles: boolean;
  hasPRCommits: boolean;
  relation: RemoteContributionRelation;
  resolution: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget: RemoteContributionResolutionTarget | null;
  providerPRNumber: number | undefined;
  pushDisabled: boolean;
  pullDisabled: boolean;
  canPush: boolean;
  canCreatePR: boolean;
  existingPrUrl: string | undefined;
  unstagedFiles: ChangedFile[];
  stagedFiles: ChangedFile[];
  prFiles: PRChangedFile[];
  prCommits: PRCommitForMerge[];
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
  comparisonTargets: string[];
  comparisonUnavailable: boolean;
  comparisonErrorCode: string | null;
  isLoading: boolean;
  loadingOperation: string | null;
  dialogs: DialogsType;
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (target: CommitDetailTarget) => void;
  onOpenReview?: () => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onStageAll: () => void;
  onUnstageAll: () => void;
  onStage: (path: string, repo?: string) => Promise<void>;
  onUnstage: (path: string, repo?: string) => Promise<void>;
  onBulkStage: (paths: string[]) => void;
  onBulkUnstage: (paths: string[]) => void;
  onBulkDiscard: (paths: string[], anchor?: HTMLElement) => void;
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
  perRepoStatus?: Array<{
    repository_name: string;
    ahead: number;
    pushAhead: number;
    pullBehind: number;
  }>;
  prByRepo?: Record<string, string | undefined>;
};

function usePerRepoCallbacks(
  git: ReturnType<typeof useSessionGit>,
  vcsDialogs: ReturnType<typeof useVcsDialogs>,
  gitHandlers: ReturnType<typeof useChangesGitHandlers>,
) {
  const { t } = useTranslation();
  return useMemo(
    () => ({
      onRepoStageAll: (repo: string) => {
        gitHandlers.handleGitOperation(
          () => git.stage(undefined, repo),
          gitOperationLabel(t, "task:stageAll", repo),
        );
      },
      onRepoUnstageAll: (repo: string) => {
        gitHandlers.handleGitOperation(
          () => git.unstage(undefined, repo),
          gitOperationLabel(t, "task:unstageAll", repo),
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

type ChangesPanelPRBuildInput = {
  prs: TaskPR[];
  filesByPRKey: Record<string, PRDiffFile[]>;
  repoNameById: Record<string, string>;
  taskHasMultipleRepos: boolean;
  selectedPRId: string | undefined;
  selectedPRRepositoryName: string | undefined;
  useRepositoryKeys: boolean;
};

function countPRsByRepository(prs: TaskPR[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const pr of prs) {
    const repositoryId = pr.repository_id ?? "";
    counts.set(repositoryId, (counts.get(repositoryId) ?? 0) + 1);
  }
  return counts;
}

function repositoryNameForPR(pr: TaskPR, repoNameById: Record<string, string>): string {
  if (!pr.repository_id) return "";
  return repoNameById[pr.repository_id] ?? "";
}

function progressRepositoryName(
  pr: TaskPR,
  selectedPRId: string | undefined,
  selectedPRRepositoryName: string | undefined,
  workspaceRepositoryName: string,
): string {
  if (pr.id === selectedPRId && selectedPRRepositoryName) return selectedPRRepositoryName;
  return resolvePRReviewRepositoryName(pr, workspaceRepositoryName || undefined) ?? pr.repo;
}

function buildChangesPanelPRData({
  prs,
  filesByPRKey,
  repoNameById,
  taskHasMultipleRepos,
  selectedPRId,
  selectedPRRepositoryName,
  useRepositoryKeys,
}: ChangesPanelPRBuildInput): { prFiles: PRChangedFile[]; prDiffFiles: PRDiffFile[] } {
  const merged: PRChangedFile[] = [];
  const progressSources = new Map<string, ReviewProgressPRSource>();
  const prCounts = countPRsByRepository(prs);
  const anyRepoMultiPR = Array.from(prCounts.values()).some((count) => count > 1);
  const needsStamp = prs.length > 1;

  for (const pr of prs) {
    const files = filesByPRKey[prFetchKey(pr)] ?? [];
    const repositoryName = repositoryNameForPR(pr, repoNameById);
    const stamp = computePRGroupStamp({
      needsStamp,
      taskHasMultipleRepos,
      anyRepoMultiPR,
      repoName: repositoryName || `${pr.owner}/${pr.repo}`,
      branch: pr.head_branch ?? "",
      prNumber: pr.pr_number,
    });
    merged.push(...mapPRFilesToChangedFiles(files, stamp, prTaskKey(pr)));
    progressSources.set(pr.id, {
      repositoryName: progressRepositoryName(
        pr,
        selectedPRId,
        selectedPRRepositoryName,
        repositoryName,
      ),
      files,
    });
  }

  return {
    prFiles: merged,
    prDiffFiles: selectPRFilesForReviewProgress(progressSources, selectedPRId, useRepositoryKeys),
  };
}

function useChangesPanelPRData(repositoryNames: string[], sessionId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const relationState: RemoteContributionRelationState = useRemoteContributionRelation(sessionId);
  const { filesByPRKey } = useActiveTaskPRsWithFiles(relationState.prs);
  const prs = relationState.prs;
  const taskPR = relationState.selectedPR;
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repoNameById = useMemo(() => buildRepoNameById(reposByWorkspace), [reposByWorkspace]);
  const taskRepositoryCount = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return 0;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.repositories?.length ?? 0;
  });
  const taskHasMultipleRepos = taskRepositoryCount > 1;
  const useRepositoryKeys = isReviewMultiRepo(taskRepositoryCount, repositoryNames);
  const selectedPRRepositoryName = usePRReviewRepositoryIdentity(activeTaskId, sessionId, taskPR);
  const prCommitsList = relationState.commits;
  const prCommits = useMemo<PRCommitForMerge[]>(() => {
    if (!workspaceId?.trim() || !taskPR?.owner?.trim() || !taskPR.repo?.trim()) return [];
    return prCommitsList.map((commit) => ({
      ...commit,
      workspace_id: workspaceId,
      owner: taskPR.owner,
      repo: taskPR.repo,
      repository_name: useRepositoryKeys ? selectedPRRepositoryName : undefined,
    }));
  }, [
    prCommitsList,
    workspaceId,
    taskPR?.owner,
    taskPR?.repo,
    selectedPRRepositoryName,
    useRepositoryKeys,
  ]);
  const { prFiles, prDiffFiles } = useMemo(
    () =>
      buildChangesPanelPRData({
        prs,
        filesByPRKey,
        repoNameById,
        taskHasMultipleRepos,
        selectedPRId: taskPR?.id,
        selectedPRRepositoryName,
        useRepositoryKeys,
      }),
    [
      prs,
      filesByPRKey,
      repoNameById,
      taskHasMultipleRepos,
      taskPR?.id,
      selectedPRRepositoryName,
      useRepositoryKeys,
    ],
  );
  const hasPRFiles = prFiles.length > 0;
  const hasPRCommits = prCommits.length > 0;
  return {
    prDiffFiles,
    prCommits,
    hasPRFiles,
    hasPRCommits,
    prFiles,
    useRepositoryKeys,
    relation: relationState.relation,
    prs,
    repositoryName: relationState.repositoryName,
    selectedPR: taskPR,
    refreshProviderEvidence: relationState.refreshProviderEvidence,
  };
}

function hasCumulativeFiles(files: Record<string, unknown> | null | undefined): boolean {
  return Object.keys(files ?? {}).length > 0;
}

function useChangesPanelResolutionTarget(
  relation: RemoteContributionRelation,
  repositoryName: string | undefined,
  selectedPR: TaskPR | null | undefined,
  t: (key: string) => string,
) {
  const remoteRepositoryLabel = t("task:remoteRepository");
  return useMemo(
    () =>
      buildRemoteContributionResolutionTarget(
        relation,
        repositoryName,
        selectedPR,
        remoteRepositoryLabel,
      ),
    [relation, repositoryName, selectedPR, remoteRepositoryLabel],
  );
}

export function useChangesPanelData() {
  const { t } = useTranslation();
  const { activeTaskId, activeSessionId, baseBranch, gitCredentialDisplay } =
    useChangesPanelStoreData();
  const baseBranchByRepo = useBaseBranchByRepo(activeTaskId);
  const git = useSessionGit(activeSessionId);
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const reviewRepositoryNames = useMemo(
    () => [...git.repoNames, ...getCumulativeReviewRepositoryNames(git.cumulativeDiff?.files)],
    [git.repoNames, git.cumulativeDiff],
  );
  const prData = useChangesPanelPRData(reviewRepositoryNames, activeSessionId);
  const resolution = useRemoteContributionResolution(
    activeSessionId,
    prData.refreshProviderEvidence,
  );
  const resolutionTarget = useChangesPanelResolutionTarget(
    prData.relation,
    prData.repositoryName,
    prData.selectedPR,
    t,
  );
  const remoteActionPolicy = useMemo(
    () => remoteContributionActionPolicy(prData.relation),
    [prData.relation],
  );
  const pullDisabledReason = useMemo(() => {
    const key = remoteContributionActionReasonKey(prData.relation, "pull");
    return key ? t(key) : undefined;
  }, [prData.relation, t]);
  const vcsDialogs = useVcsDialogs();
  const baseBranchDisplay = useMemo(() => getBaseBranchDisplay(baseBranch), [baseBranch]);
  const unstagedFiles = useMemo(() => mapToChangedFiles(git.unstagedFiles), [git.unstagedFiles]);
  const stagedFiles = useMemo(() => mapToChangedFiles(git.stagedFiles), [git.stagedFiles]);
  const { reviewedCount, totalFileCount } = useMemo(
    () =>
      computeReviewProgress(
        git.allFiles,
        git.cumulativeDiff,
        reviews,
        prData.prDiffFiles,
        prData.useRepositoryKeys,
      ),
    [git.allFiles, git.cumulativeDiff, reviews, prData.prDiffFiles, prData.useRepositoryKeys],
  );
  const staged = useMemo(() => computeStagedStats(git.stagedFiles), [git.stagedFiles]);
  const gitHandlers = useChangesGitHandlers(git, toast, baseBranch);
  const localDialogs = useChangesDialogHandlers(git, toast, gitHandlers.handleGitOperation);
  const dialogs = { ...localDialogs, ...vcsDialogs };
  const repoCallbacks = usePerRepoCallbacks(git, vcsDialogs, gitHandlers);
  const repoDisplayName = useRepoDisplayName(activeSessionId);
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repoNameById = useMemo(() => buildRepoNameById(reposByWorkspace), [reposByWorkspace]);
  const pendingByRepo = useAppStore((state) =>
    activeTaskId ? state.pendingPrUrlByTaskId.byTaskId[activeTaskId] : undefined,
  );
  const existingPrUrl = prData.selectedPR?.pr_url ?? pendingByRepo?.[""];
  const prByRepo = useMemo(
    () => buildPrByRepoMap(prData.prs, repoNameById, pendingByRepo),
    [prData.prs, repoNameById, pendingByRepo],
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
    gitCredentialDisplay,
    walkthroughRequestReady,
    resolution,
    resolutionTarget,
    pushDisabled: remoteActionPolicy.pushDisabled,
    pullDisabled: remoteActionPolicy.pullDisabled,
    pullDisabledReason,
    ...prData,
  };
}

type ChangesPanelCallbacks = {
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (target: CommitDetailTarget) => void;
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
    relation: data.relation,
    resolution: data.resolution,
    resolutionTarget: data.resolutionTarget,
    providerPRNumber: data.selectedPR?.pr_number,
    pushDisabled: data.pushDisabled,
    pullDisabled: data.pullDisabled,
    canPush: git.canPush,
    canCreatePR: git.canCreatePR,
    existingPrUrl: data.existingPrUrl,
    unstagedFiles: data.unstagedFiles,
    stagedFiles: data.stagedFiles,
    prFiles: data.prFiles,
    prCommits: data.prCommits,
    commits: git.commits,
    pendingStageFiles: git.pendingStageFiles,
    reviewedCount: data.reviewedCount,
    totalFileCount: data.totalFileCount,
    aheadCount: git.ahead,
    comparisonTargets: git.comparisonTargets,
    comparisonUnavailable: git.comparisonUnavailable,
    comparisonErrorCode: git.comparisonErrorCode,
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
