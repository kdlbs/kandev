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
import type { ChangedFileTarget } from "./changes-timeline-selection";
import type {
  CommitDetailTarget,
  CommitFileNavigationRequest,
  OpenDiffOptions,
} from "@/lib/state/diff-target-types";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";
import { gitOperationLabel } from "@/hooks/use-git-with-feedback";
import { getGitCredentialDisplay } from "./changes-git-credential-display";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import type { RemoteContributionResolutionTarget } from "./changes-panel-contribution-state";
import { useChangesPanelContributionState } from "./changes-panel-contribution-state";
import { useRemoteContributionResolution } from "./use-remote-contribution-resolution";
import { useTranslation } from "react-i18next";
import { useWorkspaceRestoration } from "@/hooks/domains/session/use-workspace-restoration";
import type { WorkspaceRestorationAttempt } from "@/lib/state/slices/session-runtime/workspace-restoration";
import {
  deriveChangesPanelToolbarStatus,
  useChangesPanelGitStatus,
  type ChangesPanelGitStatus,
} from "./changes-panel-git-status";
import { ChangesInlineCommitState } from "./changes-inline-commit-state";
import {
  useChangesInlineCommitDetails,
  useChangesPanelContextIdentity,
} from "./use-changes-inline-commit-details";

function useChangesPanelStoreData() {
  const { t } = useTranslation();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useEnvironmentSessionId();
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
  gitStatus: ChangesPanelGitStatus;
  isLoading: boolean;
  loadingOperation: string | null;
  dialogs: DialogsType;
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (
    target: CommitDetailTarget,
    fileNavigation?: CommitFileNavigationRequest,
  ) => void;
  onOpenReview?: () => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onStageAll: () => void;
  onUnstageAll: () => void;
  onStage: (path: string, repo?: string) => Promise<void>;
  onUnstage: (path: string, repo?: string) => Promise<void>;
  onBulkStage: (files: ChangedFileTarget[]) => void;
  onBulkUnstage: (files: ChangedFileTarget[]) => void;
  onBulkDiscard: (files: ChangedFileTarget[], anchor?: HTMLElement) => void;
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
  workspaceRestoration?: WorkspaceRestorationAttempt | null;
  onRestoreWorkspace?: () => void;
  restoreWorkspaceDisabled?: boolean;
  /** Monotonic token used to expand both histories after comparison navigation. */
  comparisonRequestToken?: number;
  inlineCommitDetails: ChangesInlineCommitState;
  inlineCommitDetailVersion: number;
  contextKey: string;
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
    repositoryScope: relationState.repositoryScope,
    repositoryName: relationState.repositoryName,
    selectedPR: taskPR,
    refreshProviderEvidence: relationState.refreshProviderEvidence,
    contributionHistoryTarget: relationState.contributionHistoryTarget,
  };
}

function hasCumulativeFiles(files: Record<string, unknown> | null | undefined): boolean {
  return Object.keys(files ?? {}).length > 0;
}

function useReviewRepositoryNames(
  repoNames: string[],
  cumulativeFiles: Parameters<typeof getCumulativeReviewRepositoryNames>[0],
) {
  return useMemo(
    () => [...repoNames, ...getCumulativeReviewRepositoryNames(cumulativeFiles)],
    [repoNames, cumulativeFiles],
  );
}

function useChangesPanelGitPresentation({
  git,
  reviews,
  prDiffFiles,
  useRepositoryKeys,
  hasPRFiles,
  baseBranch,
}: {
  git: ReturnType<typeof useSessionGit>;
  reviews: ReturnType<typeof useSessionFileReviews>["reviews"];
  prDiffFiles: PRDiffFile[];
  useRepositoryKeys: boolean;
  hasPRFiles: boolean;
  baseBranch: Parameters<typeof getBaseBranchDisplay>[0];
}) {
  const baseBranchDisplay = useMemo(() => getBaseBranchDisplay(baseBranch), [baseBranch]);
  const unstagedFiles = useMemo(
    () => mapToChangedFiles(git.displayUnstagedFiles),
    [git.displayUnstagedFiles],
  );
  const stagedFiles = useMemo(
    () => mapToChangedFiles(git.displayStagedFiles),
    [git.displayStagedFiles],
  );
  const { reviewedCount, totalFileCount } = useMemo(
    () =>
      computeReviewProgress(
        git.displayAllFiles,
        git.cumulativeDiff,
        reviews,
        prDiffFiles,
        useRepositoryKeys,
      ),
    [git.displayAllFiles, git.cumulativeDiff, reviews, prDiffFiles, useRepositoryKeys],
  );
  const staged = useMemo(
    () => computeStagedStats(git.displayStagedFiles),
    [git.displayStagedFiles],
  );
  const walkthroughRequestReady =
    unstagedFiles.length > 0 ||
    stagedFiles.length > 0 ||
    (git.statusLoaded && hasCumulativeFiles(git.cumulativeDiff?.files)) ||
    hasPRFiles;
  return {
    baseBranchDisplay,
    unstagedFiles,
    stagedFiles,
    reviewedCount,
    totalFileCount,
    staged,
    walkthroughRequestReady,
  };
}

export function useChangesPanelData() {
  const { activeTaskId, activeSessionId, baseBranch, gitCredentialDisplay } =
    useChangesPanelStoreData();
  const changesContext = useChangesPanelContextIdentity();
  const inlineCommitDetails = useChangesInlineCommitDetails(changesContext);
  const workspaceRestoration = useWorkspaceRestoration(activeTaskId, activeSessionId);
  const baseBranchByRepo = useBaseBranchByRepo(activeTaskId);
  const git = useSessionGit(activeSessionId);
  const gitStatusPresentation = useChangesPanelGitStatus(
    activeSessionId,
    git.gitStatus,
    git.statusByRepo,
  );
  const refreshStatus = deriveChangesPanelToolbarStatus(
    gitStatusPresentation,
    inlineCommitDetails.pendingRequestCount > 0,
  );
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const reviewRepositoryNames = useReviewRepositoryNames(git.repoNames, git.cumulativeDiff?.files);
  const prData = useChangesPanelPRData(reviewRepositoryNames, activeSessionId);
  const gitPresentation = useChangesPanelGitPresentation({
    git,
    reviews,
    prDiffFiles: prData.prDiffFiles,
    useRepositoryKeys: prData.useRepositoryKeys,
    hasPRFiles: prData.prFiles.length > 0,
    baseBranch,
  });
  const resolution = useRemoteContributionResolution(
    activeSessionId,
    prData.refreshProviderEvidence,
  );
  const contributionState = useChangesPanelContributionState(
    prData.relation,
    prData.repositoryScope,
    prData.selectedPR,
  );
  const vcsDialogs = useVcsDialogs();
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
  return {
    activeTaskId,
    activeSessionId,
    contextKey: changesContext.contextKey,
    inlineCommitDetails,
    refreshStatus,
    git,
    gitStatusPresentation,
    ...gitPresentation,
    baseBranchByRepo,
    gitHandlers,
    localDialogs,
    dialogs,
    repoCallbacks,
    repoDisplayName,
    prByRepo,
    existingPrUrl,
    gitCredentialDisplay,
    resolution,
    resolutionTarget: contributionState.resolutionTarget,
    workspaceRestoration,
    pushDisabled: contributionState.remoteActionPolicy.pushDisabled,
    pullDisabled: contributionState.remoteActionPolicy.pullDisabled,
    pullDisabledReason: contributionState.pullDisabledReason,
    ...prData,
  };
}

export { buildChangesPanelBodyProps } from "./changes-panel-body-props";
