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
import { DiscardDialog, CommitDialog, PRDialog } from "./changes-panel-dialogs";
import { ChangesPanelHeader } from "./changes-panel-header";
import {
  FileListSection,
  CommitsSection,
  ActionButtonsSection,
  ReviewProgressBar,
} from "./changes-panel-timeline";
import { useChangesGitHandlers, useChangesDialogHandlers } from "./changes-panel-hooks";

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

function collectReviewPaths(
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
): Set<string> {
  const paths = new Set<string>();
  for (const file of uncommittedFiles) {
    if (file.diff && normalizeDiffContent(file.diff)) paths.add(file.path);
  }
  if (cumulativeDiffFiles) {
    for (const [path, file] of Object.entries(cumulativeDiffFiles)) {
      if (paths.has(path)) continue;
      if (file.diff && normalizeDiffContent(file.diff)) paths.add(path);
    }
  }
  return paths;
}

function getDiffForPath(
  path: string,
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
): string {
  const uncommitted = uncommittedFiles.find((f) => f.path === path);
  if (uncommitted?.diff) return normalizeDiffContent(uncommitted.diff);
  const cumDiff = cumulativeDiffFiles?.[path]?.diff;
  if (cumDiff) return normalizeDiffContent(cumDiff);
  return "";
}

function computeReviewProgress(
  uncommittedFiles: FileInfo[],
  cumulativeDiff: { files?: CumulativeDiffFiles } | null,
  reviews: Map<string, { reviewed: boolean; diffHash?: string }>,
) {
  const cumulativeDiffFiles = cumulativeDiff?.files;
  const paths = collectReviewPaths(uncommittedFiles, cumulativeDiffFiles);
  let reviewed = 0;
  for (const path of paths) {
    const state = reviews.get(path);
    if (!state?.reviewed) continue;
    const diffContent = getDiffForPath(path, uncommittedFiles, cumulativeDiffFiles);
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
  canPush: boolean;
  canCreatePR: boolean;
  existingPrUrl: string | undefined;
  unstagedFiles: ChangedFile[];
  stagedFiles: ChangedFile[];
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
    | "canPush"
    | "canCreatePR"
    | "existingPrUrl"
    | "unstagedFiles"
    | "stagedFiles"
    | "commits"
    | "pendingStageFiles"
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
    | "onForcePush"
  >,
) {
  if (!props.hasAnything) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
        Your changed files will appear here
      </div>
    );
  }
  const showStaged = props.hasUnstaged || props.hasStaged;
  const showCommits = props.hasStaged || props.hasCommits;

  return (
    <div className="flex flex-col">
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
  const { activeSessionId, taskTitle, baseBranch, existingPrUrl } = useChangesPanelStoreData();

  const git = useSessionGit(activeSessionId);
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);

  const baseBranchDisplay = useMemo(() => getBaseBranchDisplay(baseBranch), [baseBranch]);
  const unstagedFiles = useMemo(() => mapToChangedFiles(git.unstagedFiles), [git.unstagedFiles]);
  const stagedFiles = useMemo(() => mapToChangedFiles(git.stagedFiles), [git.stagedFiles]);

  const { reviewedCount, totalFileCount } = useMemo(
    () => computeReviewProgress(git.allFiles, git.cumulativeDiff, reviews),
    [git.allFiles, git.cumulativeDiff, reviews],
  );
  const { stagedFileCount, stagedAdditions, stagedDeletions } = useMemo(
    () => computeStagedStats(git.stagedFiles),
    [git.stagedFiles],
  );

  const { handleGitOperation, handlePull, handleRebase, handlePush, handleForcePush } = useChangesGitHandlers(
    git,
    toast,
    baseBranch,
  );
  const dialogs = useChangesDialogHandlers(git, toast, handleGitOperation, taskTitle, baseBranch);

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <ChangesPanelHeader
        hasChanges={git.hasChanges}
        hasCommits={git.hasCommits}
        displayBranch={git.branch}
        baseBranchDisplay={baseBranchDisplay}
        behindCount={git.behind}
        isLoading={git.isLoading}
        onOpenDiffAll={onOpenDiffAll}
        onOpenReview={onOpenReview}
        onPull={handlePull}
        onRebase={handleRebase}
      />
      <ChangesPanelBody
        hasAnything={git.hasAnything}
        hasUnstaged={git.hasUnstaged}
        hasStaged={git.hasStaged}
        hasCommits={git.hasCommits}
        canPush={git.canPush}
        canCreatePR={git.canCreatePR}
        existingPrUrl={existingPrUrl}
        unstagedFiles={unstagedFiles}
        stagedFiles={stagedFiles}
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
        onOpenReview={onOpenReview}
        onStageAll={git.stageAll}
        onStage={(path) => git.stage([path]).then(() => undefined)}
        onUnstage={(path) => git.unstage([path]).then(() => undefined)}
        onPush={handlePush}
        onForcePush={handleForcePush}
        stagedFileCount={stagedFileCount}
        stagedAdditions={stagedAdditions}
        stagedDeletions={stagedDeletions}
        displayBranch={git.branch}
        baseBranch={baseBranch}
      />
    </PanelRoot>
  );
});

export { ChangesPanel };
