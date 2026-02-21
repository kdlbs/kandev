"use client";

import { memo, useMemo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { hashDiff, normalizeDiffContent } from "@/components/review/types";
import type { FileInfo } from "@/lib/state/store";
import { useToast } from "@/components/toast-provider";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { DiscardDialog, CommitDialog, PRDialog } from "./changes-panel-dialogs";
import { ChangesPanelHeader } from "./changes-panel-header";
import {
  FileListSection,
  CommitsSection,
  ActionButtonsSection,
  ReviewProgressBar,
} from "./changes-panel-timeline";
import {
  useChangesGitHandlers,
  useChangesStageHandlers,
  useChangesDialogHandlers,
} from "./changes-panel-hooks";

type ChangesPanelProps = {
  onOpenDiffFile: (path: string) => void;
  onEditFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
};

type CumulativeDiffFiles = Record<
  string,
  { diff?: string; status?: string; additions?: number; deletions?: number }
>;

function collectReviewPaths(
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
): Set<string> {
  const paths = new Set<string>();
  if (gitStatus?.files) {
    for (const [path, file] of Object.entries(gitStatus.files)) {
      if (file.diff && normalizeDiffContent(file.diff)) paths.add(path);
    }
  }
  if (cumulativeDiffFiles) {
    for (const [path, file] of Object.entries(cumulativeDiffFiles)) {
      if (paths.has(path)) continue;
      if (file.diff && normalizeDiffContent(file.diff)) paths.add(path);
    }
  }
  return paths;
}

function getDiffContentForPath(
  path: string,
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
): string {
  const uncommitted = gitStatus?.files?.[path];
  if (uncommitted?.diff) return normalizeDiffContent(uncommitted.diff);
  const cumDiff = cumulativeDiffFiles?.[path]?.diff;
  if (cumDiff) return normalizeDiffContent(cumDiff);
  return "";
}

function isFileReviewStale(diffContent: string, diffHash: string | undefined): boolean {
  return !!(diffContent && diffHash && diffHash !== hashDiff(diffContent));
}

/** Compute review progress across uncommitted + committed files */
function computeReviewProgress(
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  cumulativeDiff: { files?: CumulativeDiffFiles } | null,
  reviews: Map<string, { reviewed: boolean; diffHash?: string }>,
) {
  const cumulativeDiffFiles = cumulativeDiff?.files;
  const paths = collectReviewPaths(gitStatus, cumulativeDiffFiles);
  let reviewed = 0;
  for (const path of paths) {
    const state = reviews.get(path);
    if (!state?.reviewed) continue;
    const diffContent = getDiffContentForPath(path, gitStatus, cumulativeDiffFiles);
    if (isFileReviewStale(diffContent, state.diffHash)) continue;
    reviewed++;
  }
  return { reviewedCount: reviewed, totalFileCount: paths.size };
}

function computeHasAnything(
  hasUnstaged: boolean,
  hasStaged: boolean,
  hasCommits: boolean,
): boolean {
  return hasUnstaged || hasStaged || hasCommits;
}

/** Determine the last timeline section for isLast logic */
function getLastTimelineSection(hasCommits: boolean, hasStaged: boolean): string {
  if (hasCommits) return "action";
  if (hasStaged) return "staged";
  return "unstaged";
}

function getBaseBranchDisplay(baseBranch: string | undefined): string {
  return baseBranch ? baseBranch.replace(/^origin\//, "") : "main";
}

function mapChangedFiles(gitStatus: ReturnType<typeof useSessionGitStatus>) {
  if (!gitStatus?.files || Object.keys(gitStatus.files).length === 0) return [];
  return (Object.values(gitStatus.files) as FileInfo[]).map((file: FileInfo) => ({
    path: file.path,
    status: file.status,
    staged: file.staged,
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
  }));
}

function computeStagedStats(
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  stagedFiles: unknown[],
) {
  let additions = 0;
  let deletions = 0;
  const count = stagedFiles.length;
  if (gitStatus?.files && count > 0) {
    for (const file of Object.values(gitStatus.files) as FileInfo[]) {
      if (file.staged) {
        additions += file.additions || 0;
        deletions += file.deletions || 0;
      }
    }
  }
  return { stagedFileCount: count, stagedAdditions: additions, stagedDeletions: deletions };
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
  const displayBranch = useAppStore((state) =>
    activeSessionId ? (state.gitStatus.bySessionId[activeSessionId]?.branch ?? null) : null,
  );
  return { activeSessionId, taskTitle, baseBranch, displayBranch };
}

type ChangesPanelBodyProps = {
  hasAnything: boolean;
  hasUnstaged: boolean;
  hasStaged: boolean;
  hasCommits: boolean;
  unstagedFiles: ReturnType<typeof mapChangedFiles>;
  stagedFiles: ReturnType<typeof mapChangedFiles>;
  commits: ReturnType<typeof useSessionCommits>["commits"];
  pendingStageFiles: Set<string>;
  lastTimelineSection: string;
  reviewedCount: number;
  totalFileCount: number;
  aheadCount: number;
  isLoading: boolean;
  dialogs: ReturnType<typeof useChangesDialogHandlers>;
  onOpenDiffFile: (path: string) => void;
  onEditFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenReview?: () => void;
  onStageAll: () => void;
  onStage: (path: string) => Promise<void>;
  onUnstage: (path: string) => Promise<void>;
  onPush: () => void;
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
}: Pick<
  ChangesPanelBodyProps,
  | "dialogs"
  | "isLoading"
  | "stagedFileCount"
  | "stagedAdditions"
  | "stagedDeletions"
  | "displayBranch"
  | "baseBranch"
>) {
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
    </>
  );
}

function ChangesPanelTimeline(
  props: Pick<
    ChangesPanelBodyProps,
    | "hasAnything"
    | "hasUnstaged"
    | "hasStaged"
    | "hasCommits"
    | "unstagedFiles"
    | "stagedFiles"
    | "commits"
    | "pendingStageFiles"
    | "lastTimelineSection"
    | "aheadCount"
    | "isLoading"
    | "dialogs"
    | "onOpenDiffFile"
    | "onEditFile"
    | "onOpenCommitDetail"
    | "onStageAll"
    | "onStage"
    | "onUnstage"
    | "onPush"
  >,
) {
  if (!props.hasAnything) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
        Your changed files will appear here
      </div>
    );
  }
  return (
    <div className="flex flex-col">
      {props.hasUnstaged && (
        <FileListSection
          variant="unstaged"
          files={props.unstagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={props.lastTimelineSection === "unstaged"}
          actionLabel="Stage all"
          onAction={props.onStageAll}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
        />
      )}
      {props.hasStaged && (
        <FileListSection
          variant="staged"
          files={props.stagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={props.lastTimelineSection === "staged"}
          actionLabel="Commit"
          onAction={props.dialogs.handleOpenCommitDialog}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
        />
      )}
      {props.hasCommits && (
        <CommitsSection
          commits={props.commits}
          isLast={!props.hasCommits}
          onOpenCommitDetail={props.onOpenCommitDetail}
        />
      )}
      {props.hasCommits && (
        <ActionButtonsSection
          onOpenPRDialog={props.dialogs.handleOpenPRDialog}
          onPush={props.onPush}
          isLoading={props.isLoading}
          aheadCount={props.aheadCount}
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
      <ChangesPanelDialogsSection
        dialogs={props.dialogs}
        isLoading={props.isLoading}
        stagedFileCount={props.stagedFileCount}
        stagedAdditions={props.stagedAdditions}
        stagedDeletions={props.stagedDeletions}
        displayBranch={props.displayBranch}
        baseBranch={props.baseBranch}
      />
    </PanelBody>
  );
}

const ChangesPanel = memo(function ChangesPanel({
  onOpenDiffFile,
  onEditFile,
  onOpenCommitDetail,
  onOpenDiffAll,
  onOpenReview,
}: ChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const { activeSessionId, taskTitle, baseBranch, displayBranch } = useChangesPanelStoreData();

  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const gitOps = useGitOperations(activeSessionId ?? null);
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const { diff: cumulativeDiff } = useCumulativeDiff(activeSessionId);

  const aheadCount = gitStatus?.ahead ?? 0;
  const behindCount = gitStatus?.behind ?? 0;
  const baseBranchDisplay = useMemo(() => getBaseBranchDisplay(baseBranch), [baseBranch]);
  const changedFiles = useMemo(() => mapChangedFiles(gitStatus), [gitStatus]);
  const unstagedFiles = useMemo(() => changedFiles.filter((f) => !f.staged), [changedFiles]);
  const stagedFiles = useMemo(() => changedFiles.filter((f) => f.staged), [changedFiles]);
  const { reviewedCount, totalFileCount } = useMemo(
    () => computeReviewProgress(gitStatus, cumulativeDiff, reviews),
    [gitStatus, cumulativeDiff, reviews],
  );
  const { stagedFileCount, stagedAdditions, stagedDeletions } = useMemo(
    () => computeStagedStats(gitStatus, stagedFiles),
    [gitStatus, stagedFiles],
  );

  const { handleGitOperation, handlePull, handleRebase, handlePush } = useChangesGitHandlers(
    gitOps,
    toast,
    baseBranch,
  );
  const { pendingStageFiles, handleStageAll, handleStage, handleUnstage } = useChangesStageHandlers(
    gitOps,
    changedFiles,
  );
  const dialogs = useChangesDialogHandlers(
    gitOps,
    toast,
    handleGitOperation,
    taskTitle,
    baseBranch,
  );

  const hasUnstaged = unstagedFiles.length > 0;
  const hasStaged = stagedFiles.length > 0;
  const hasCommits = commits.length > 0;
  const hasChanges = changedFiles.length > 0;
  const hasAnything = computeHasAnything(hasUnstaged, hasStaged, hasCommits);
  const lastTimelineSection = getLastTimelineSection(hasCommits, hasStaged);

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <ChangesPanelHeader
        hasChanges={hasChanges}
        hasCommits={hasCommits}
        displayBranch={displayBranch}
        baseBranchDisplay={baseBranchDisplay}
        behindCount={behindCount}
        isLoading={gitOps.isLoading}
        onOpenDiffAll={onOpenDiffAll}
        onOpenReview={onOpenReview}
        onPull={handlePull}
        onRebase={handleRebase}
      />
      <ChangesPanelBody
        hasAnything={hasAnything}
        hasUnstaged={hasUnstaged}
        hasStaged={hasStaged}
        hasCommits={hasCommits}
        unstagedFiles={unstagedFiles}
        stagedFiles={stagedFiles}
        commits={commits}
        pendingStageFiles={pendingStageFiles}
        lastTimelineSection={lastTimelineSection}
        reviewedCount={reviewedCount}
        totalFileCount={totalFileCount}
        aheadCount={aheadCount}
        isLoading={gitOps.isLoading}
        dialogs={dialogs}
        onOpenDiffFile={onOpenDiffFile}
        onEditFile={onEditFile}
        onOpenCommitDetail={onOpenCommitDetail}
        onOpenReview={onOpenReview}
        onStageAll={handleStageAll}
        onStage={handleStage}
        onUnstage={handleUnstage}
        onPush={handlePush}
        stagedFileCount={stagedFileCount}
        stagedAdditions={stagedAdditions}
        stagedDeletions={stagedDeletions}
        displayBranch={displayBranch}
        baseBranch={baseBranch}
      />
    </PanelRoot>
  );
});

export { ChangesPanel };
