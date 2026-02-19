"use client";

import {
  IconArrowBackUp,
  IconExternalLink,
  IconPlus,
  IconMinus,
  IconCheck,
  IconChevronRight,
  IconGitCommit,
  IconLoader2,
  IconColumns,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { LineStat } from "@/components/diff-stat";
import { cn } from "@/lib/utils";
import type { FileInfo } from "@/lib/state/store";
import { FileStatusIcon } from "./file-status-icon";

export type ChangedFileItem = {
  path: string;
  status: FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

export const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf("/");
  if (lastSlash === -1) return { folder: "", file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

export function DiscardDialog({
  open,
  onOpenChange,
  fileToDiscard,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileToDiscard: string | null;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Discard changes?</AlertDialogTitle>
          <AlertDialogDescription>
            This will permanently discard all changes to{" "}
            <span className="font-semibold">{fileToDiscard}</span>. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Discard
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function DiffTabContent({
  changedFiles,
  pendingStageFiles,
  commits,
  expandedCommit,
  commitDiffs,
  loadingCommitSha,
  reviewedCount,
  totalFileCount,
  reviewProgressPercent,
  onSelectDiff,
  onStage,
  onUnstage,
  onDiscard,
  onOpenInPanel,
  onOpenInEditor,
  onToggleCommit,
}: {
  changedFiles: ChangedFileItem[];
  pendingStageFiles: Set<string>;
  commits: Array<{
    commit_sha: string;
    commit_message: string;
    insertions: number;
    deletions: number;
  }>;
  expandedCommit: string | null;
  commitDiffs: Record<string, Record<string, FileInfo>>;
  loadingCommitSha: string | null;
  reviewedCount: number;
  totalFileCount: number;
  reviewProgressPercent: number;
  onSelectDiff: (path: string, content?: string) => void;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
  onDiscard: (path: string) => void;
  onOpenInPanel: (path: string) => void;
  onOpenInEditor: (path: string) => void;
  onToggleCommit: (sha: string) => void;
}) {
  return (
    <>
      <div className="space-y-4">
        {changedFiles.length > 0 && (
          <UncommittedFilesSection
            changedFiles={changedFiles}
            pendingStageFiles={pendingStageFiles}
            onSelectDiff={onSelectDiff}
            onStage={onStage}
            onUnstage={onUnstage}
            onDiscard={onDiscard}
            onOpenInPanel={onOpenInPanel}
            onOpenInEditor={onOpenInEditor}
          />
        )}
        {commits.length > 0 && (
          <CommitsExpandableSection
            commits={commits}
            expandedCommit={expandedCommit}
            commitDiffs={commitDiffs}
            loadingCommitSha={loadingCommitSha}
            onToggleCommit={onToggleCommit}
            onSelectDiff={onSelectDiff}
          />
        )}
        {changedFiles.length === 0 && commits.length === 0 && (
          <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
            Your changed files will appear here
          </div>
        )}
      </div>
      <FilesPanelReviewProgress
        reviewedCount={reviewedCount}
        totalFileCount={totalFileCount}
        reviewProgressPercent={reviewProgressPercent}
      />
    </>
  );
}

function UncommittedFilesSection({
  changedFiles,
  pendingStageFiles,
  onSelectDiff,
  onStage,
  onUnstage,
  onDiscard,
  onOpenInPanel,
  onOpenInEditor,
}: {
  changedFiles: ChangedFileItem[];
  pendingStageFiles: Set<string>;
  onSelectDiff: (path: string) => void;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
  onDiscard: (path: string) => void;
  onOpenInPanel: (path: string) => void;
  onOpenInEditor: (path: string) => void;
}) {
  return (
    <div>
      <div className="text-xs font-medium text-muted-foreground mb-2">
        Uncommitted ({changedFiles.length})
      </div>
      <ul className="space-y-1">
        {changedFiles.map((file) => (
          <UncommittedFileRow
            key={file.path}
            file={file}
            isPending={pendingStageFiles.has(file.path)}
            onSelectDiff={onSelectDiff}
            onStage={onStage}
            onUnstage={onUnstage}
            onDiscard={onDiscard}
            onOpenInPanel={onOpenInPanel}
            onOpenInEditor={onOpenInEditor}
          />
        ))}
      </ul>
    </div>
  );
}

function UncommittedFileRow({
  file,
  isPending,
  onSelectDiff,
  onStage,
  onUnstage,
  onDiscard,
  onOpenInPanel,
  onOpenInEditor,
}: {
  file: ChangedFileItem;
  isPending: boolean;
  onSelectDiff: (path: string) => void;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
  onDiscard: (path: string) => void;
  onOpenInPanel: (path: string) => void;
  onOpenInEditor: (path: string) => void;
}) {
  const { folder, file: name } = splitPath(file.path);
  return (
    <li
      className="group flex items-center justify-between gap-2 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
      onClick={() => onSelectDiff(file.path)}
    >
      <div className="flex items-center gap-2 min-w-0">
        <FileStageIcon
          isPending={isPending}
          staged={file.staged}
          path={file.path}
          onStage={onStage}
          onUnstage={onUnstage}
        />
        <button type="button" className="min-w-0 text-left cursor-pointer" title={file.path}>
          <p className="flex text-foreground text-xs min-w-0">
            {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
            <span className="font-medium text-foreground whitespace-nowrap shrink-0">{name}</span>
          </p>
        </button>
      </div>
      <div className="flex items-center gap-2">
        <FileRowActionButtons
          path={file.path}
          onDiscard={onDiscard}
          onOpenInPanel={onOpenInPanel}
          onOpenInEditor={onOpenInEditor}
        />
        <LineStat added={file.plus} removed={file.minus} />
        <FileStatusIcon status={file.status} />
      </div>
    </li>
  );
}

function FileStageIcon({
  isPending,
  staged,
  path,
  onStage,
  onUnstage,
}: {
  isPending: boolean;
  staged: boolean;
  path: string;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
}) {
  if (isPending) {
    return (
      <div className="flex-shrink-0 flex items-center justify-center size-4">
        <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (staged) {
    return (
      <button
        type="button"
        title="Unstage file"
        className="group/unstage flex-shrink-0 flex items-center justify-center size-4 rounded bg-emerald-500/20 text-emerald-600 hover:bg-rose-500/20 hover:text-rose-600 cursor-pointer"
        onClick={(e) => {
          e.stopPropagation();
          void onUnstage(path);
        }}
      >
        <IconCheck className="h-3 w-3 group-hover/unstage:hidden" />
        <IconMinus className="h-2.5 w-2.5 hidden group-hover/unstage:block" />
      </button>
    );
  }
  return (
    <button
      type="button"
      title="Stage file"
      className="flex-shrink-0 flex items-center justify-center size-4 rounded border border-dashed border-muted-foreground/50 text-muted-foreground hover:border-emerald-500 hover:text-emerald-500 hover:bg-emerald-500/10 cursor-pointer"
      onClick={(e) => {
        e.stopPropagation();
        void onStage(path);
      }}
    >
      <IconPlus className="h-2.5 w-2.5" />
    </button>
  );
}

function FileRowActionButtons({
  path,
  onDiscard,
  onOpenInPanel,
  onOpenInEditor,
}: {
  path: string;
  onDiscard: (p: string) => void;
  onOpenInPanel: (p: string) => void;
  onOpenInEditor: (p: string) => void;
}) {
  return (
    <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onDiscard(path);
            }}
          >
            <IconArrowBackUp className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Discard changes</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onOpenInPanel(path);
            }}
          >
            <IconColumns className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Open side-by-side</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground"
            onClick={(e) => {
              e.stopPropagation();
              onOpenInEditor(path);
            }}
          >
            <IconExternalLink className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Open in external editor</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function CommitsExpandableSection({
  commits,
  expandedCommit,
  commitDiffs,
  loadingCommitSha,
  onToggleCommit,
  onSelectDiff,
}: {
  commits: Array<{
    commit_sha: string;
    commit_message: string;
    insertions: number;
    deletions: number;
  }>;
  expandedCommit: string | null;
  commitDiffs: Record<string, Record<string, FileInfo>>;
  loadingCommitSha: string | null;
  onToggleCommit: (sha: string) => void;
  onSelectDiff: (path: string, content?: string) => void;
}) {
  return (
    <div>
      <div className="text-xs font-medium text-muted-foreground mb-2">
        Commits ({commits.length})
      </div>
      <ul className="space-y-1">
        {commits.map((commit) => (
          <CommitRow
            key={commit.commit_sha}
            commit={commit}
            isExpanded={expandedCommit === commit.commit_sha}
            files={commitDiffs[commit.commit_sha]}
            isLoading={loadingCommitSha === commit.commit_sha}
            onToggle={onToggleCommit}
            onSelectDiff={onSelectDiff}
          />
        ))}
      </ul>
    </div>
  );
}

function CommitRow({
  commit,
  isExpanded,
  files,
  isLoading,
  onToggle,
  onSelectDiff,
}: {
  commit: { commit_sha: string; commit_message: string; insertions: number; deletions: number };
  isExpanded: boolean;
  files?: Record<string, FileInfo>;
  isLoading: boolean;
  onToggle: (sha: string) => void;
  onSelectDiff: (path: string, content?: string) => void;
}) {
  return (
    <li>
      <button
        type="button"
        className="w-full flex items-center gap-2 text-left rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
        onClick={() => onToggle(commit.commit_sha)}
      >
        <IconChevronRight
          className={cn(
            "h-3 w-3 text-muted-foreground transition-transform shrink-0",
            isExpanded && "rotate-90",
          )}
        />
        <IconGitCommit className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
        <div className="flex-1 min-w-0">
          <p className="text-xs truncate">
            <code className="font-mono text-muted-foreground">{commit.commit_sha.slice(0, 7)}</code>{" "}
            <span className="text-foreground">{commit.commit_message}</span>
          </p>
        </div>
        <span className="text-xs shrink-0">
          <span className="text-emerald-500">+{commit.insertions}</span>
          {"/"}
          <span className="text-rose-500">-{commit.deletions}</span>
        </span>
        {isLoading && <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
      </button>
      {isExpanded && files && (
        <ul className="ml-6 mt-1 space-y-0.5">
          {Object.entries(files).map(([path, file]) => {
            const { folder, file: fileName } = splitPath(path);
            return (
              <li
                key={path}
                className="flex items-center gap-2 text-xs rounded px-1 py-0.5 hover:bg-muted/50 cursor-pointer"
                onClick={() => onSelectDiff(path, file.diff)}
              >
                <FileStatusIcon status={file.status} />
                <span className="flex flex-1 min-w-0" title={path}>
                  {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
                  <span className="text-foreground whitespace-nowrap shrink-0">{fileName}</span>
                </span>
                <span className="shrink-0 text-xs">
                  <span className="text-emerald-500">+{file.additions ?? 0}</span>
                  {"/"}
                  <span className="text-rose-500">-{file.deletions ?? 0}</span>
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </li>
  );
}

export function FilesPanelReviewProgress({
  reviewedCount,
  totalFileCount,
  reviewProgressPercent,
}: {
  reviewedCount: number;
  totalFileCount: number;
  reviewProgressPercent: number;
}) {
  if (totalFileCount <= 0) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className="mt-auto flex items-center gap-2 pt-2 cursor-pointer transition-colors"
          onClick={() => window.dispatchEvent(new CustomEvent("switch-to-changes-tab"))}
        >
          <div className="flex-1 h-0.5 rounded-full bg-muted-foreground/10 overflow-hidden">
            <div
              className="h-full bg-muted-foreground/25 rounded-full transition-all duration-300"
              style={{ width: `${reviewProgressPercent}%` }}
            />
          </div>
          <span className="text-[10px] text-muted-foreground/40 whitespace-nowrap">
            {reviewedCount}/{totalFileCount} reviewed
          </span>
        </div>
      </TooltipTrigger>
      <TooltipContent>
        {reviewedCount} of {totalFileCount} files reviewed
      </TooltipContent>
    </Tooltip>
  );
}
