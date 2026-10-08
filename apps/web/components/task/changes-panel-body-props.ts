import type { ChangesPanelBodyProps, useChangesPanelData } from "./changes-panel-data";
import { groupChangedFileTargetsByRepository } from "./changes-timeline-selection";
import type {
  CommitDetailTarget,
  CommitFileNavigationRequest,
  OpenDiffOptions,
} from "@/lib/state/diff-target-types";

type ChangesPanelCallbacks = {
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (
    target: CommitDetailTarget,
    fileNavigation?: CommitFileNavigationRequest,
  ) => void;
  onOpenReview?: () => void;
};

type ChangesPanelData = ReturnType<typeof useChangesPanelData>;

type ChangesPanelWorkspaceActions = Pick<
  ChangesPanelBodyProps,
  | "onRevertCommit"
  | "onStageAll"
  | "onUnstageAll"
  | "onStage"
  | "onUnstage"
  | "onBulkStage"
  | "onBulkUnstage"
  | "onBulkDiscard"
  | "onPush"
  | "onForcePush"
  | "onRepoStageAll"
  | "onRepoUnstageAll"
  | "onRepoCommit"
  | "onRepoPush"
  | "onRepoCreatePR"
>;

function buildChangesPanelWorkspaceActions(
  data: ChangesPanelData,
  workspaceBlocked: boolean,
): ChangesPanelWorkspaceActions {
  const { git, gitHandlers, localDialogs, repoCallbacks } = data;
  if (workspaceBlocked) {
    return {
      onRevertCommit: undefined,
      onStageAll: () => undefined,
      onUnstageAll: () => undefined,
      onStage: async () => undefined,
      onUnstage: async () => undefined,
      onBulkStage: () => undefined,
      onBulkUnstage: () => undefined,
      onBulkDiscard: () => undefined,
      onPush: () => undefined,
      onForcePush: () => undefined,
      onRepoStageAll: undefined,
      onRepoUnstageAll: undefined,
      onRepoCommit: undefined,
      onRepoPush: undefined,
      onRepoCreatePR: undefined,
    };
  }
  return {
    onRevertCommit: gitHandlers.handleRevertCommit,
    onStageAll: git.stageAll,
    onUnstageAll: git.unstageAll,
    onStage: (path, repo) => git.stageFile([path], repo).then(() => undefined),
    onUnstage: (path, repo) => git.unstageFile([path], repo).then(() => undefined),
    onBulkStage: (files) => {
      for (const group of groupChangedFileTargetsByRepository(files)) {
        git.stageFile(group.paths, group.repositoryName).catch(() => undefined);
      }
    },
    onBulkUnstage: (files) => {
      for (const group of groupChangedFileTargetsByRepository(files)) {
        git.unstageFile(group.paths, group.repositoryName).catch(() => undefined);
      }
    },
    onBulkDiscard: localDialogs.handleBulkDiscardClick,
    onPush: () => gitHandlers.handlePush(),
    onForcePush: () => gitHandlers.handleForcePush(),
    onRepoStageAll: repoCallbacks.onRepoStageAll,
    onRepoUnstageAll: repoCallbacks.onRepoUnstageAll,
    onRepoCommit: repoCallbacks.onRepoCommit,
    onRepoPush: repoCallbacks.onRepoPush,
    onRepoCreatePR: repoCallbacks.onRepoCreatePR,
  };
}

export function buildChangesPanelBodyProps(
  data: ChangesPanelData,
  callbacks: ChangesPanelCallbacks,
): ChangesPanelBodyProps {
  const { git, staged } = data;
  const workspaceBlocked =
    data.workspaceRestoration.status !== null && data.workspaceRestoration.status !== "ready";
  const workspaceActions = buildChangesPanelWorkspaceActions(data, workspaceBlocked);
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
    pushDisabled: data.pushDisabled || workspaceBlocked,
    pullDisabled: data.pullDisabled || workspaceBlocked,
    canPush: git.canPush && !workspaceBlocked,
    canCreatePR: git.canCreatePR && !workspaceBlocked,
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
    gitStatus: data.gitStatusPresentation,
    inlineCommitDetails: data.inlineCommitDetails.state,
    inlineCommitDetailVersion: data.inlineCommitDetails.version,
    contextKey: data.contextKey,
    isLoading: git.isLoading,
    loadingOperation: git.loadingOperation,
    dialogs: data.dialogs,
    onOpenDiffFile: callbacks.onOpenDiffFile,
    onEditFile: callbacks.onEditFile,
    onOpenCommitDetail: callbacks.onOpenCommitDetail,
    onOpenReview: callbacks.onOpenReview,
    ...workspaceActions,
    stagedFileCount: staged.stagedFileCount,
    stagedAdditions: staged.stagedAdditions,
    stagedDeletions: staged.stagedDeletions,
    repoDisplayName: data.repoDisplayName,
    perRepoStatus: git.perRepoStatus,
    prByRepo: data.prByRepo,
    workspaceRestoration: data.workspaceRestoration.attempt,
    onRestoreWorkspace: () => void data.workspaceRestoration.restore(),
    restoreWorkspaceDisabled: data.workspaceRestoration.status === "pending",
  };
}
