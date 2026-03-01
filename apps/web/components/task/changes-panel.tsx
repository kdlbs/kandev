"use client";

import { memo, useMemo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { hashDiff, normalizeDiffContent } from "@/components/review/types";
import type { FileInfo } from "@/lib/state/store";
import { useToast } from "@/components/toast-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { DiscardDialog, CommitDialog, PRDialog, AmendDialog, ResetDialog } from "./changes-panel-dialogs";
import { ChangesPanelHeader } from "./changes-panel-header";
import {
  FileListSection,
  CommitsSection,
  ActionButtonsSection,
  ReviewProgressBar,
  PRFilesSection,
  PRCommitsSection,
} from "./changes-panel-timeline";
import type { PRChangedFile } from "./changes-panel-timeline";
import { useChangesGitHandlers, useChangesDialogHandlers } from "./changes-panel-hooks";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import type { PRDiffFile } from "@/lib/types/github";

type ChangesPanelProps = {
  onOpenDiffFile: (path: string) => void;
  onEditFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
};

// Maps FileInfo (store) to the display format expected by FileListSection
type ChangedFile = {
  path: string;
  status: FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

function mapToChangedFiles(files: FileInfo[]): ChangedFile[] {
  return files.map((file) => ({
    path: file.path,
    status: file.status,
    staged: file.staged,
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
  }));
}

type CumulativeDiffFiles = Record<
  string,
  { diff?: string; status?: string; additions?: number; deletions?: number }
>;

function addUncommittedPaths(paths: Set<string>, files: FileInfo[]) {
  for (const file of files) {
    if (file.diff && normalizeDiffContent(file.diff)) paths.add(file.path);
  }
}

function addCumulativePaths(paths: Set<string>, files: CumulativeDiffFiles) {
  for (const [path, file] of Object.entries(files)) {
    if (!paths.has(path) && file.diff && normalizeDiffContent(file.diff)) paths.add(path);
  }
}

function addPRPaths(paths: Set<string>, files: PRDiffFile[]) {
  for (const file of files) {
    if (!paths.has(file.filename) && file.patch) paths.add(file.filename);
  }
}

function collectReviewPaths(
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
  prFiles?: PRDiffFile[],
): Set<string> {
  const paths = new Set<string>();
  addUncommittedPaths(paths, uncommittedFiles);
  if (cumulativeDiffFiles) addCumulativePaths(paths, cumulativeDiffFiles);
  if (prFiles) addPRPaths(paths, prFiles);
  return paths;
}

function getDiffForPath(
  path: string,
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
  prFiles?: PRDiffFile[],
): string {
  const uncommitted = uncommittedFiles.find((f) => f.path === path);
  if (uncommitted?.diff) return normalizeDiffContent(uncommitted.diff);
  const cumDiff = cumulativeDiffFiles?.[path]?.diff;
  if (cumDiff) return normalizeDiffContent(cumDiff);
  if (prFiles) {
    const prFile = prFiles.find((f) => f.filename === path);
    if (prFile?.patch) return normalizeDiffContent(prFile.patch);
  }
  return "";
}

function computeReviewProgress(
  uncommittedFiles: FileInfo[],
  cumulativeDiff: { files?: CumulativeDiffFiles } | null,
  reviews: Map<string, { reviewed: boolean; diffHash?: string }>,
  prFiles?: PRDiffFile[],
) {
  const cumulativeDiffFiles = cumulativeDiff?.files;
  const paths = collectReviewPaths(uncommittedFiles, cumulativeDiffFiles, prFiles);
  let reviewed = 0;
  for (const path of paths) {
    const state = reviews.get(path);
    if (!state?.reviewed) continue;
    const diffContent = getDiffForPath(path, uncommittedFiles, cumulativeDiffFiles, prFiles);
    if (diffContent && state.diffHash && state.diffHash !== hashDiff(diffContent)) continue;
    reviewed++;
  }
  return { reviewedCount: reviewed, totalFileCount: paths.size };
}

function computeStagedStats(stagedFiles: FileInfo[]) {
  let additions = 0;
  let deletions = 0;
  for (const file of stagedFiles) {
    additions += file.additions || 0;
    deletions += file.deletions || 0;
  }
  return {
    stagedFileCount: stagedFiles.length,
    stagedAdditions: additions,
    stagedDeletions: deletions,
  };
}

function mapPRFilesToChangedFiles(files: PRDiffFile[]): PRChangedFile[] {
  return files.map((file) => {
    let status: FileInfo["status"];
    switch (file.status) {
      case "added":
        status = "added";
        break;
      case "removed":
        status = "deleted";
        break;
      case "renamed":
        status = "renamed";
        break;
      default:
        status = "modified";
    }
    return {
      path: file.filename,
      status,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
    };
  });
}

function getBaseBranchDisplay(baseBranch: string | undefined): string {
  return baseBranch ? baseBranch.replace(/^origin\//, "") : "main";
}

function useChangesPanelStoreData() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
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
  return { activeSessionId, taskTitle, baseBranch, existingPrUrl };
}

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
  prCommits: { sha: string; message: string; author_login: string; author_date: string }[];
  commits: ReturnType<typeof useSessionGit>["commits"];
  pendingStageFiles: Set<string>;
  reviewedCount: number;
  totalFileCount: number;
  aheadCount: number;
  isLoading: boolean;
  dialogs: ReturnType<typeof useChangesDialogHandlers>;
  onOpenDiffFile: (path: string) => void;
  onEditFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenReview?: () => void;
  onRevertCommit?: (sha: string) => void;
  onStageAll: () => void;
  onStage: (path: string) => Promise<void>;
  onUnstage: (path: string) => Promise<void>;
  onPush: () => void;
  onForcePush: () => void;
  stagedFileCount: number;
  stagedAdditions: number;
  stagedDeletions: number;
  displayBranch: string | null;
  baseBranch: string | undefined;
};

function ChangesPanelDialogsSection({
  dialogs,
  isLoading,
  stagedFileCount,
  stagedAdditions,
  stagedDeletions,
  displayBranch,
  baseBranch,
  lastCommitMessage,
}: Pick<
  ChangesPanelBodyProps,
  | "dialogs"
  | "isLoading"
  | "stagedFileCount"
  | "stagedAdditions"
  | "stagedDeletions"
  | "displayBranch"
  | "baseBranch"
> & { lastCommitMessage?: string | null }) {
  return (
    <>
      <DiscardDialog
        open={dialogs.showDiscardDialog}
        onOpenChange={dialogs.setShowDiscardDialog}
        fileToDiscard={dialogs.fileToDiscard}
        onConfirm={dialogs.handleDiscardConfirm}
      />
      <CommitDialog
        open={dialogs.commitDialogOpen}
        onOpenChange={dialogs.setCommitDialogOpen}
        commitMessage={dialogs.commitMessage}
        onCommitMessageChange={dialogs.setCommitMessage}
        onCommit={dialogs.handleCommit}
        isLoading={isLoading}
        stagedFileCount={stagedFileCount}
        stagedAdditions={stagedAdditions}
        stagedDeletions={stagedDeletions}
        isAmend={dialogs.isAmendCommit}
        onAmendChange={dialogs.setIsAmendCommit}
        lastCommitMessage={lastCommitMessage}
      />
      <PRDialog
        open={dialogs.prDialogOpen}
        onOpenChange={dialogs.setPrDialogOpen}
        prTitle={dialogs.prTitle}
        onPrTitleChange={dialogs.setPrTitle}
        prBody={dialogs.prBody}
        onPrBodyChange={dialogs.setPrBody}
        prDraft={dialogs.prDraft}
        onPrDraftChange={dialogs.setPrDraft}
        onCreatePR={dialogs.handleCreatePR}
        isLoading={isLoading}
        displayBranch={displayBranch}
        baseBranch={baseBranch}
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
  | "hasPRCommits"
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
  | "dialogs"
  | "onOpenDiffFile"
  | "onEditFile"
  | "onOpenCommitDetail"
  | "onRevertCommit"
  | "onStageAll"
  | "onStage"
  | "onUnstage"
  | "onPush"
  | "onForcePush"
>;

function TimelineLocalChanges(props: TimelineProps) {
  const showStaged = props.hasUnstaged || props.hasStaged;
  const showCommits = props.hasStaged || props.hasCommits;

  return (
    <>
      {props.hasUnstaged && (
        <FileListSection
          variant="unstaged"
          files={props.unstagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={!showStaged}
          actionLabel="Stage all"
          onAction={props.onStageAll}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
        />
      )}
      {showStaged && (
        <FileListSection
          variant="staged"
          files={props.stagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={!showCommits}
          actionLabel="Commit"
          onAction={props.dialogs.handleOpenCommitDialog}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
        />
      )}
      {showCommits && (
        <CommitsSection
          commits={props.commits}
          isLast={false}
          onOpenCommitDetail={props.onOpenCommitDetail}
          onRevertCommit={props.onRevertCommit}
          onAmendCommit={props.dialogs.handleOpenAmendDialog}
          onResetToCommit={props.dialogs.handleOpenResetDialog}
        />
      )}
      {showCommits && (
        <ActionButtonsSection
          onOpenPRDialog={props.dialogs.handleOpenPRDialog}
          onPush={props.onPush}
          isLoading={props.isLoading}
          aheadCount={props.aheadCount}
          canPush={props.canPush}
          canCreatePR={props.canCreatePR}
          existingPrUrl={props.existingPrUrl}
          onForcePush={props.onForcePush}
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

  const hasLocalChanges = props.hasUnstaged || props.hasStaged || props.hasCommits;

  return (
    <div className="flex flex-col">
      {props.hasPRFiles && (
        <PRFilesSection
          files={props.prFiles}
          isLast={!props.hasPRCommits && !hasLocalChanges}
          onOpenDiff={props.onOpenDiffFile}
        />
      )}
      {props.hasPRCommits && (
        <PRCommitsSection
          commits={props.prCommits}
          isLast={!hasLocalChanges}
          onOpenCommitDetail={props.onOpenCommitDetail}
        />
      )}
      <TimelineLocalChanges {...props} />
    </div>
  );
}

function ChangesPanelBody(props: ChangesPanelBodyProps) {
  // Get last commit message for amend feature
  const lastCommitMessage = props.commits?.[0]?.commit_message ?? null;

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
      <ChangesPanelDialogsSection
        dialogs={props.dialogs}
        isLoading={props.isLoading}
        stagedFileCount={props.stagedFileCount}
        stagedAdditions={props.stagedAdditions}
        stagedDeletions={props.stagedDeletions}
        displayBranch={props.displayBranch}
        baseBranch={props.baseBranch}
        lastCommitMessage={lastCommitMessage}
      />
    </PanelBody>
  );
}

function useChangesPanelPRData() {
  const taskPR = useActiveTaskPR();
  const { files: prDiffFiles } = usePRDiff(
    taskPR?.owner ?? null,
    taskPR?.repo ?? null,
    taskPR?.pr_number ?? null,
  );
  const { commits: prCommitsList } = usePRCommits(
    taskPR?.owner ?? null,
    taskPR?.repo ?? null,
    taskPR?.pr_number ?? null,
  );
  const hasPRFiles = prDiffFiles.length > 0;
  const hasPRCommits = prCommitsList.length > 0;
  const prFiles = useMemo(() => mapPRFilesToChangedFiles(prDiffFiles), [prDiffFiles]);
  return { prDiffFiles, prCommitsList, hasPRFiles, hasPRCommits, prFiles };
}

const ChangesPanel = memo(function ChangesPanel({
  onOpenDiffFile,
  onEditFile,
  onOpenCommitDetail,
  onOpenDiffAll,
  onOpenReview,
}: ChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const { activeSessionId, taskTitle, baseBranch, existingPrUrl } = useChangesPanelStoreData();

  const git = useSessionGit(activeSessionId);
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const { prDiffFiles, prCommitsList, hasPRFiles, hasPRCommits, prFiles } = useChangesPanelPRData();

  const baseBranchDisplay = useMemo(() => getBaseBranchDisplay(baseBranch), [baseBranch]);
  const unstagedFiles = useMemo(() => mapToChangedFiles(git.unstagedFiles), [git.unstagedFiles]);
  const stagedFiles = useMemo(() => mapToChangedFiles(git.stagedFiles), [git.stagedFiles]);

  const { reviewedCount, totalFileCount } = useMemo(
    () => computeReviewProgress(git.allFiles, git.cumulativeDiff, reviews, prDiffFiles),
    [git.allFiles, git.cumulativeDiff, reviews, prDiffFiles],
  );
  const staged = useMemo(() => computeStagedStats(git.stagedFiles), [git.stagedFiles]);

  const gitHandlers = useChangesGitHandlers(git, toast, baseBranch);
  const dialogs = useChangesDialogHandlers(
    git,
    toast,
    gitHandlers.handleGitOperation,
    taskTitle,
    baseBranch,
  );

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <ChangesPanelHeader
        hasChanges={git.hasChanges}
        hasCommits={git.hasCommits}
        hasPRFiles={hasPRFiles}
        displayBranch={git.branch}
        baseBranchDisplay={baseBranchDisplay}
        behindCount={git.behind}
        isLoading={git.isLoading}
        onOpenDiffAll={onOpenDiffAll}
        onOpenReview={onOpenReview}
        onPull={gitHandlers.handlePull}
        onRebase={gitHandlers.handleRebase}
      />
      <ChangesPanelBody
        hasAnything={git.hasAnything || hasPRFiles || hasPRCommits}
        hasUnstaged={git.hasUnstaged}
        hasStaged={git.hasStaged}
        hasCommits={git.hasCommits}
        hasPRFiles={hasPRFiles}
        hasPRCommits={hasPRCommits}
        canPush={git.canPush}
        canCreatePR={git.canCreatePR}
        existingPrUrl={existingPrUrl}
        unstagedFiles={unstagedFiles}
        stagedFiles={stagedFiles}
        prFiles={prFiles}
        prCommits={prCommitsList}
        commits={git.commits}
        pendingStageFiles={git.pendingStageFiles}
        reviewedCount={reviewedCount}
        totalFileCount={totalFileCount}
        aheadCount={git.ahead}
        isLoading={git.isLoading}
        dialogs={dialogs}
        onOpenDiffFile={onOpenDiffFile}
        onEditFile={onEditFile}
        onOpenCommitDetail={onOpenCommitDetail}
        onRevertCommit={gitHandlers.handleRevertCommit}
        onOpenReview={onOpenReview}
        onStageAll={git.stageAll}
        onStage={(path) => git.stage([path]).then(() => undefined)}
        onUnstage={(path) => git.unstage([path]).then(() => undefined)}
        onPush={gitHandlers.handlePush}
        onForcePush={gitHandlers.handleForcePush}
        stagedFileCount={staged.stagedFileCount}
        stagedAdditions={staged.stagedAdditions}
        stagedDeletions={staged.stagedDeletions}
        displayBranch={git.branch}
        baseBranch={baseBranch}
      />
    </PanelRoot>
  );
});

export { ChangesPanel };
