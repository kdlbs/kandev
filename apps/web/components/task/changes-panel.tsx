"use client";

import { memo, useMemo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useToast } from "@/components/toast-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { DiscardDialog, AmendDialog, ResetDialog } from "./changes-panel-dialogs";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { ChangesPanelHeader } from "./changes-panel-header";
import {
  FileListSection,
  CommitsSection,
  ReviewProgressBar,
  PRFilesSection,
} from "./changes-panel-timeline";
import type { PRChangedFile } from "./changes-panel-timeline";
import { useChangesGitHandlers, useChangesDialogHandlers } from "./changes-panel-hooks";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import {
  type ChangedFile,
  computeReviewProgress,
  computeStagedStats,
  filterUnpushedCommits,
  getBaseBranchDisplay,
  mapPRFilesToChangedFiles,
  mapToChangedFiles,
  mergeCommits,
} from "./changes-panel-helpers";

export { filterUnpushedCommits, mergeCommits };

type ChangesPanelProps = {
  onOpenDiffFile: (path: string) => void;
  onEditFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
};

function useChangesPanelStoreData() {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  // Use environment-stable sessionId so git hooks (commits, cumulative diff)
  // don't re-fetch when switching between sessions in the same environment.
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
    return state.taskPRs.byTaskId[taskId]?.pr_url ?? undefined;
  });
  return { activeTaskId, activeSessionId, taskTitle, baseBranch, existingPrUrl };
}

type DialogsType = ReturnType<typeof useChangesDialogHandlers> & ReturnType<typeof useVcsDialogs>;

type ChangesPanelBodyProps = {
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
  }[];
  pendingStageFiles: Set<string>;
  reviewedCount: number;
  totalFileCount: number;
  aheadCount: number;
  isLoading: boolean;
  loadingOperation: string | null;
  dialogs: DialogsType;
  onOpenDiffFile: (path: string) => void;
  onEditFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenReview?: () => void;
  // Multi-repo: handlers carry the commit's repository_name so revert/amend/
  // reset target the right git repo. Without it the op runs at the workspace
  // root and fails ("can only revert latest commit") on multi-repo tasks.
  onRevertCommit?: (sha: string, repo?: string) => void;
  onStageAll: () => void;
  onUnstageAll: () => void;
  // Multi-repo: callers pass the file's repository_name so the agentctl op
  // routes to the right repo. Same-named files across repos can't be
  // disambiguated by path alone.
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
  // Per-repo handlers — wired only in multi-repo workspaces. Each one targets
  // a single repo subpath in agentctl. Single-repo sessions don't render the
  // per-repo group header, so these are unused in that case.
  onRepoStageAll?: (repo: string) => void;
  onRepoUnstageAll?: (repo: string) => void;
  onRepoCommit?: (repo: string) => void;
  onRepoPush?: (repo: string) => void;
  onRepoCreatePR?: (repo: string) => void;
  /** Maps a repository_name to its display label (used as the per-repo group label). */
  repoDisplayName?: (repositoryName: string) => string | undefined;
  /** Per-repo branch / ahead / behind summary; powers the "ahead" badge on Push. */
  perRepoStatus?: Array<{ repository_name: string; ahead: number }>;
  /** Existing PR URL keyed by repository_name; "" key for single-repo. */
  prByRepo?: Record<string, string | undefined>;
};

function ChangesPanelDialogsSection({
  dialogs,
  isLoading,
}: Pick<ChangesPanelBodyProps, "dialogs" | "isLoading">) {
  return (
    <>
      <DiscardDialog
        open={dialogs.showDiscardDialog}
        onOpenChange={dialogs.setShowDiscardDialog}
        fileToDiscard={dialogs.fileToDiscard}
        filesToDiscard={dialogs.filesToDiscard}
        onConfirm={dialogs.handleDiscardConfirm}
      />
      <AmendDialog
        open={dialogs.amendDialogOpen}
        onOpenChange={dialogs.setAmendDialogOpen}
        amendMessage={dialogs.amendMessage}
        onAmendMessageChange={dialogs.setAmendMessage}
        onAmend={dialogs.handleAmend}
        isLoading={isLoading}
      />
      <ResetDialog
        open={dialogs.resetDialogOpen}
        onOpenChange={dialogs.setResetDialogOpen}
        commitSha={dialogs.resetCommitSha}
        onReset={dialogs.handleReset}
        isLoading={isLoading}
      />
    </>
  );
}

type TimelineProps = Pick<
  ChangesPanelBodyProps,
  | "hasAnything"
  | "hasUnstaged"
  | "hasStaged"
  | "hasCommits"
  | "hasPRFiles"
  | "canPush"
  | "canCreatePR"
  | "existingPrUrl"
  | "unstagedFiles"
  | "stagedFiles"
  | "prFiles"
  | "prCommits"
  | "commits"
  | "pendingStageFiles"
  | "aheadCount"
  | "isLoading"
  | "loadingOperation"
  | "dialogs"
  | "onOpenDiffFile"
  | "onEditFile"
  | "onOpenCommitDetail"
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
  | "repoDisplayName"
  | "perRepoStatus"
  | "prByRepo"
>;

type WorkingTreeProps = Pick<
  TimelineProps,
  | "hasUnstaged"
  | "hasStaged"
  | "unstagedFiles"
  | "stagedFiles"
  | "pendingStageFiles"
  | "loadingOperation"
  | "dialogs"
  | "onOpenDiffFile"
  | "onEditFile"
  | "onStageAll"
  | "onUnstageAll"
  | "onStage"
  | "onUnstage"
  | "onBulkStage"
  | "onBulkUnstage"
  | "onBulkDiscard"
  | "onRepoStageAll"
  | "onRepoUnstageAll"
  | "onRepoCommit"
  | "repoDisplayName"
> & { isLastUnstaged: boolean; isLastStaged: boolean };

function WorkingTreeSections(props: WorkingTreeProps) {
  const isBulkOp = props.pendingStageFiles.size === 0;
  return (
    <>
      {props.hasUnstaged && (
        <FileListSection
          variant="unstaged"
          files={props.unstagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={props.isLastUnstaged}
          actionLabel="Stage all"
          isActionLoading={isBulkOp && props.loadingOperation === "stage"}
          onAction={props.onStageAll}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
          onBulkStage={props.onBulkStage}
          onBulkDiscard={props.onBulkDiscard}
          onRepoAction={props.onRepoStageAll}
          repoDisplayName={props.repoDisplayName}
        />
      )}
      {props.hasStaged && (
        <FileListSection
          variant="staged"
          files={props.stagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={props.isLastStaged}
          actionLabel="Commit"
          isActionLoading={props.loadingOperation === "commit"}
          onAction={() => props.dialogs.openCommitDialog()}
          secondaryActionLabel="Unstage all"
          isSecondaryActionLoading={isBulkOp && props.loadingOperation === "unstage"}
          onSecondaryAction={props.onUnstageAll}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
          onBulkUnstage={props.onBulkUnstage}
          onBulkDiscard={props.onBulkDiscard}
          onRepoAction={props.onRepoCommit}
          onRepoSecondaryAction={props.onRepoUnstageAll}
          repoDisplayName={props.repoDisplayName}
        />
      )}
    </>
  );
}

function ChangesPanelTimeline(props: TimelineProps) {
  if (!props.hasAnything) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
        Your changed files will appear here
      </div>
    );
  }

  const mergedCommits = mergeCommits(props.commits, props.prCommits);
  const hasMergedCommits = mergedCommits.length > 0;
  const hasLocalChanges = props.hasUnstaged || props.hasStaged;
  const showCommits = props.hasStaged || props.hasCommits;
  const showCommitsList = props.hasStaged || hasMergedCommits;
  const hasSomethingAfterStaged = (props.hasPRFiles && hasLocalChanges) || showCommitsList;

  return (
    <div className="flex flex-col">
      {/* PR files first when no local working-tree changes */}
      {props.hasPRFiles && !hasLocalChanges && (
        <div data-testid="pr-files-section">
          <PRFilesSection
            files={props.prFiles}
            isLast={!showCommitsList}
            onOpenDiff={props.onOpenDiffFile}
          />
        </div>
      )}

      <WorkingTreeSections
        {...props}
        isLastUnstaged={!props.hasStaged && !hasSomethingAfterStaged}
        isLastStaged={!hasSomethingAfterStaged}
      />

      {/* PR files after local changes when both exist */}
      {props.hasPRFiles && hasLocalChanges && (
        <div data-testid="pr-files-section">
          <PRFilesSection
            files={props.prFiles}
            isLast={!showCommitsList}
            onOpenDiff={props.onOpenDiffFile}
          />
        </div>
      )}

      {showCommitsList && (
        <CommitsSection
          commits={mergedCommits}
          isLast={!showCommits}
          onOpenCommitDetail={props.onOpenCommitDetail}
          onRevertCommit={props.onRevertCommit}
          onAmendCommit={props.dialogs.handleOpenAmendDialog}
          onResetToCommit={props.dialogs.handleOpenResetDialog}
          onRepoPush={props.onRepoPush}
          onRepoCreatePR={props.onRepoCreatePR}
          repoDisplayName={props.repoDisplayName}
          perRepoStatus={props.perRepoStatus}
          prByRepo={props.prByRepo}
        />
      )}
    </div>
  );
}

function ChangesPanelBody(props: ChangesPanelBodyProps) {
  return (
    <PanelBody className="flex flex-col">
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">
        <ChangesPanelTimeline {...props} />
      </div>
      <ReviewProgressBar
        reviewedCount={props.reviewedCount}
        totalFileCount={props.totalFileCount}
        onOpenReview={props.onOpenReview}
      />
      <ChangesPanelDialogsSection dialogs={props.dialogs} isLoading={props.isLoading} />
    </PanelBody>
  );
}

/**
 * Adapts the existing single-repo handlers to the per-repo invocation sites
 * (group-header buttons). Each callback simply forwards `repo` to the
 * underlying op — the routing happens inside `useSessionGit` /
 * `useGitOperations`. Single-repo passes the empty string, which the backend
 * treats as the workspace root.
 */
function usePerRepoCallbacks(
  git: ReturnType<typeof useSessionGit>,
  vcsDialogs: ReturnType<typeof useVcsDialogs>,
  gitHandlers: ReturnType<typeof useChangesGitHandlers>,
) {
  // Bug 9: route per-repo stage/unstage through `handleGitOperation` so toasts
  // fire for both success and failure. The previous `.catch(() => undefined)`
  // silenced rejections — the user got no feedback when an op failed.
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
  const taskPR = useActiveTaskPR();
  const refreshKey = taskPR?.last_synced_at ?? null;
  const { files: prDiffFiles } = usePRDiff(
    taskPR?.owner ?? null,
    taskPR?.repo ?? null,
    taskPR?.pr_number ?? null,
    refreshKey,
  );
  const { commits: prCommitsList } = usePRCommits(
    taskPR?.owner ?? null,
    taskPR?.repo ?? null,
    taskPR?.pr_number ?? null,
    refreshKey,
  );
  const hasPRFiles = prDiffFiles.length > 0;
  const hasPRCommits = prCommitsList.length > 0;
  const prFiles = useMemo(() => mapPRFilesToChangedFiles(prDiffFiles), [prDiffFiles]);
  return { prDiffFiles, prCommitsList, hasPRFiles, hasPRCommits, prFiles };
}

function useChangesPanelData() {
  const { activeSessionId, baseBranch, existingPrUrl } = useChangesPanelStoreData();
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
  // Existing PR URL is currently workspace-scoped — surface it under the
  // empty (single-repo) key so per-repo Create PR can show "PR exists".
  const prByRepo = useMemo<Record<string, string | undefined>>(
    () => ({ "": existingPrUrl }),
    [existingPrUrl],
  );
  return {
    git,
    baseBranchDisplay,
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
    ...prData,
  };
}

function buildChangesPanelBodyProps(
  data: ReturnType<typeof useChangesPanelData>,
  callbacks: ChangesPanelProps,
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

const ChangesPanel = memo(function ChangesPanel(props: ChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const data = useChangesPanelData();
  if (isArchived) return <ArchivedPanelPlaceholder />;
  return (
    <PanelRoot data-testid="changes-panel">
      <ChangesPanelHeader
        hasChanges={data.git.hasChanges}
        hasCommits={data.git.hasCommits}
        hasPRFiles={data.hasPRFiles}
        displayBranch={data.git.branch}
        baseBranchDisplay={data.baseBranchDisplay}
        behindCount={data.git.behind}
        isLoading={data.git.isLoading}
        loadingOperation={data.git.loadingOperation}
        onOpenDiffAll={props.onOpenDiffAll}
        onOpenReview={props.onOpenReview}
        repoNames={data.git.repoNames}
        perRepoStatus={data.git.perRepoStatus}
        onRepoPull={data.repoCallbacks.onRepoPull}
        onRepoRebase={data.repoCallbacks.onRepoRebase}
        onRepoMerge={data.repoCallbacks.onRepoMerge}
        repoDisplayName={data.repoDisplayName}
      />
      <ChangesPanelBody {...buildChangesPanelBodyProps(data, props)} />
    </PanelRoot>
  );
});

export { ChangesPanel };
