"use client";

import {
  IconGitCommit,
  IconGitPullRequest,
  IconCloudUpload,
  IconChevronDown,
  IconArrowBackUp,
} from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { FileInfo } from "@/lib/state/store";
import { LineStat } from "@/components/diff-stat";
import { FileStatusIcon } from "./file-status-icon";
import { FileRow } from "./changes-panel-file-row";

// --- Timeline visual components ---

type ChangedFile = {
  path: string;
  status: FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

// --- Timeline section dot colors ---
const DOT_COLORS = {
  unstaged: "bg-yellow-500",
  staged: "bg-emerald-500",
  commits: "bg-blue-500",
  pr: "bg-purple-500",
  action: "bg-muted-foreground/25",
} as const;

function TimelineDot({ color }: { color: string }) {
  return <div className={cn("relative z-10 size-1.5 rounded-full shrink-0 mt-[5px]", color)} />;
}

function TimelineSection({
  dotColor,
  label,
  count,
  action,
  isLast,
  children,
}: {
  dotColor: string;
  label?: string;
  count?: number;
  action?: React.ReactNode;
  isLast?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="relative flex gap-2.5">
      {/* Vertical line + dot */}
      <div className="flex flex-col items-center">
        <TimelineDot color={dotColor} />
        {!isLast && <div className="w-px flex-1 bg-border/60" />}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 pb-3">
        {/* Header */}
        {label && (
          <div className="flex items-center justify-between gap-2 -mt-0.5 mb-1">
            <span className="text-[11px] font-medium uppercase tracking-wider text-foreground/70">
              {label}
              {typeof count === "number" && (
                <span className="ml-1 text-muted-foreground/50 font-normal">({count})</span>
              )}
            </span>
            {action}
          </div>
        )}

        {/* Children (file list, buttons, etc.) */}
        {children}
      </div>
    </div>
  );
}

// --- Commits section ---

type CommitItem = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
};

type CommitsSectionProps = {
  commits: CommitItem[];
  isLast: boolean;
  onOpenCommitDetail?: (sha: string) => void;
  onRevertCommit?: (sha: string) => void;
};

export function CommitsSection({
  commits,
  isLast,
  onOpenCommitDetail,
  onRevertCommit,
}: CommitsSectionProps) {
  return (
    <TimelineSection
      dotColor={DOT_COLORS.commits}
      label="Commits"
      count={commits.length}
      isLast={isLast}
    >
      <ul className="space-y-0.5">
        {commits.map((commit, index) => (
          <li
            key={`${commit.commit_sha}-${index}`}
            className="group flex items-center gap-2 text-xs rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
            onClick={() => onOpenCommitDetail?.(commit.commit_sha)}
          >
            <IconGitCommit className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
            <code className="font-mono text-muted-foreground text-[11px]">
              {commit.commit_sha.slice(0, 7)}
            </code>
            <span className="flex-1 min-w-0 truncate text-foreground">{commit.commit_message}</span>
            <span className="shrink-0 text-[11px] flex items-center gap-1">
              <span className="text-emerald-500">+{commit.insertions}</span>{" "}
              <span className="text-rose-500">-{commit.deletions}</span>
              {index === 0 && onRevertCommit && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      aria-label="Revert commit"
                      className="ml-1 text-muted-foreground hover:text-foreground cursor-pointer opacity-0 group-hover:opacity-100 focus:opacity-100"
                      onClick={(e) => {
                        e.stopPropagation();
                        onRevertCommit(commit.commit_sha);
                      }}
                    >
                      <IconArrowBackUp className="h-3.5 w-3.5" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>Revert commit</TooltipContent>
                </Tooltip>
              )}
            </span>
          </li>
        ))}
      </ul>
    </TimelineSection>
  );
}

// --- Action buttons section (Create PR / Push) ---

type ActionButtonsSectionProps = {
  onOpenPRDialog: () => void;
  onPush: () => void;
  onForcePush: () => void;
  isLoading: boolean;
  aheadCount: number;
  canPush: boolean;
  canCreatePR: boolean;
  existingPrUrl?: string;
};

export function ActionButtonsSection({
  onOpenPRDialog,
  onPush,
  onForcePush,
  isLoading,
  aheadCount,
  canPush,
  canCreatePR,
  existingPrUrl,
}: ActionButtonsSectionProps) {
  const prExists = !!existingPrUrl;
  const createPrDisabled = !canCreatePR || prExists;
  const pushDisabled = !canPush || isLoading;
  let pushTooltip: string | null = null;
  if (isLoading) pushTooltip = "A git operation is in progress";
  else if (!canPush) pushTooltip = "No commits ahead of remote";
  return (
    <TimelineSection dotColor={DOT_COLORS.action} isLast>
      <div className="flex items-center gap-2 -mt-0.5">
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button
                size="sm"
                variant="outline"
                className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
                onClick={onOpenPRDialog}
                disabled={createPrDisabled}
              >
                <IconGitPullRequest className="h-3 w-3" />
                Create PR
              </Button>
            </span>
          </TooltipTrigger>
          {prExists && <TooltipContent>A pull request already exists for this task</TooltipContent>}
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="flex items-center">
              <Button
                size="sm"
                variant="outline"
                className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer rounded-r-none border-r-0"
                onClick={onPush}
                disabled={pushDisabled}
              >
                <IconCloudUpload className="h-3 w-3" />
                Push
                {aheadCount > 0 && (
                  <span className="text-muted-foreground">{aheadCount} ahead</span>
                )}
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-6 w-5 px-0 cursor-pointer rounded-l-none"
                    disabled={pushDisabled}
                  >
                    <IconChevronDown className="h-3 w-3" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-48">
                  <DropdownMenuItem className="cursor-pointer gap-2 text-xs" onClick={onForcePush}>
                    <IconCloudUpload className="h-3.5 w-3.5 shrink-0" />
                    Force Push (with lease)
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </span>
          </TooltipTrigger>
          {pushTooltip && <TooltipContent>{pushTooltip}</TooltipContent>}
        </Tooltip>
      </div>
    </TimelineSection>
  );
}

// --- File list sections (Unstaged / Staged) ---

type FileListSectionProps = {
  variant: "unstaged" | "staged";
  files: ChangedFile[];
  pendingStageFiles: Set<string>;
  isLast: boolean;
  actionLabel: string;
  onAction: () => void;
  onOpenDiff: (path: string) => void;
  onEditFile: (path: string) => void;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
  onDiscard: (path: string) => void;
};

export function FileListSection({
  variant,
  files,
  pendingStageFiles,
  isLast,
  actionLabel,
  onAction,
  onOpenDiff,
  onEditFile,
  onStage,
  onUnstage,
  onDiscard,
}: FileListSectionProps) {
  const dotColor = variant === "unstaged" ? DOT_COLORS.unstaged : DOT_COLORS.staged;
  const label = variant === "unstaged" ? "Unstaged" : "Staged";

  return (
    <TimelineSection dotColor={dotColor} label={label} count={files.length} isLast={isLast}>
      {files.length > 0 && (
        <ul className="space-y-0.5">
          {files.map((file) => (
            <FileRow
              key={file.path}
              file={file}
              isPending={pendingStageFiles.has(file.path)}
              onOpenDiff={onOpenDiff}
              onStage={onStage}
              onUnstage={onUnstage}
              onDiscard={onDiscard}
              onEditFile={onEditFile}
            />
          ))}
        </ul>
      )}
      {files.length > 0 && (
        <div className="mt-1.5">
          <Button
            size="sm"
            variant="outline"
            className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
            onClick={onAction}
          >
            {actionLabel}
          </Button>
        </div>
      )}
    </TimelineSection>
  );
}

// --- PR files section (read-only, from GitHub PR diff) ---

export type PRChangedFile = {
  path: string;
  status: FileInfo["status"];
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

type PRFilesSectionProps = {
  files: PRChangedFile[];
  isLast: boolean;
  onOpenDiff: (path: string) => void;
};

export function PRFilesSection({ files, isLast, onOpenDiff }: PRFilesSectionProps) {
  return (
    <TimelineSection
      dotColor={DOT_COLORS.pr}
      label="PR Changes"
      count={files.length}
      isLast={isLast}
    >
      {files.length > 0 && (
        <ul className="space-y-0.5">
          {files.map((file) => (
            <PRFileRow key={file.path} file={file} onOpenDiff={onOpenDiff} />
          ))}
        </ul>
      )}
    </TimelineSection>
  );
}

function PRFileRow({
  file,
  onOpenDiff,
}: {
  file: PRChangedFile;
  onOpenDiff: (path: string) => void;
}) {
  const lastSlash = file.path.lastIndexOf("/");
  const folder = lastSlash === -1 ? "" : file.path.slice(0, lastSlash);
  const name = lastSlash === -1 ? file.path : file.path.slice(lastSlash + 1);

  return (
    <li
      className="group flex items-center justify-between gap-2 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
      onClick={() => onOpenDiff(file.path)}
    >
      <div className="flex items-center gap-2 min-w-0">
        <div className="flex-shrink-0 flex items-center justify-center size-4">
          <IconGitPullRequest className="h-3 w-3 text-purple-500" />
        </div>
        <button type="button" className="min-w-0 text-left cursor-pointer" title={file.path}>
          <p className="flex text-foreground text-xs min-w-0">
            {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
            <span className="font-medium text-foreground whitespace-nowrap shrink-0">{name}</span>
          </p>
        </button>
      </div>
      <div className="flex items-center gap-2">
        <LineStat added={file.plus} removed={file.minus} />
        <FileStatusIcon status={file.status} />
      </div>
    </li>
  );
}

// --- PR commits section ---

type PRCommitItem = {
  sha: string;
  message: string;
  author_login: string;
  author_date: string;
};

type PRCommitsSectionProps = {
  commits: PRCommitItem[];
  isLast: boolean;
  onOpenCommitDetail?: (sha: string) => void;
};

export function PRCommitsSection({ commits, isLast, onOpenCommitDetail }: PRCommitsSectionProps) {
  return (
    <TimelineSection
      dotColor={DOT_COLORS.pr}
      label="PR Commits"
      count={commits.length}
      isLast={isLast}
    >
      <ul className="space-y-0.5">
        {commits.map((commit, index) => (
          <li
            key={`${commit.sha}-${index}`}
            className="group flex items-center gap-2 text-xs rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
            onClick={() => onOpenCommitDetail?.(commit.sha)}
          >
            <IconGitCommit className="h-3.5 w-3.5 text-purple-500 shrink-0" />
            <code className="font-mono text-muted-foreground text-[11px]">
              {commit.sha.slice(0, 7)}
            </code>
            <span className="flex-1 min-w-0 truncate text-foreground">{commit.message}</span>
            {commit.author_login && (
              <span className="shrink-0 text-[10px] text-muted-foreground">
                {commit.author_login}
              </span>
            )}
          </li>
        ))}
      </ul>
    </TimelineSection>
  );
}

// --- Review progress bar ---

type ReviewProgressBarProps = {
  reviewedCount: number;
  totalFileCount: number;
  onOpenReview?: () => void;
};

export function ReviewProgressBar({
  reviewedCount,
  totalFileCount,
  onOpenReview,
}: ReviewProgressBarProps) {
  const progressPercent = totalFileCount > 0 ? (reviewedCount / totalFileCount) * 100 : 0;

  if (totalFileCount <= 0) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className="shrink-0 flex items-center gap-2 pt-2 border-t border-border/40 cursor-pointer transition-colors"
          onClick={onOpenReview}
        >
          <div className="flex-1 h-0.5 rounded-full bg-muted-foreground/10 overflow-hidden">
            <div
              className="h-full bg-muted-foreground/25 rounded-full transition-all duration-300"
              style={{ width: `${progressPercent}%` }}
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
