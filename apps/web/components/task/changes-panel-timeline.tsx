'use client';

import {
  IconGitCommit,
  IconGitPullRequest,
  IconCloudUpload,
} from '@tabler/icons-react';

import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import type { FileInfo } from '@/lib/state/store';

// --- Timeline visual components ---

type ChangedFile = {
  path: string;
  status: FileInfo['status'];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

// --- Timeline section dot colors ---
const DOT_COLORS = {
  unstaged: 'bg-yellow-500',
  staged: 'bg-emerald-500',
  commits: 'bg-blue-500',
  action: 'bg-muted-foreground/25',
} as const;

function TimelineDot({ color }: { color: string }) {
  return (
    <div
      className={cn(
        'relative z-10 size-1.5 rounded-full shrink-0 mt-[5px]',
        color,
      )}
    />
  );
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
        {!isLast && (
          <div className="w-px flex-1 bg-border/60" />
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 pb-3">
        {/* Header */}
        {label && (
          <div className="flex items-center justify-between gap-2 -mt-0.5 mb-1">
            <span className="text-[11px] font-medium uppercase tracking-wider text-foreground/70">
              {label}
              {typeof count === 'number' && count > 0 && (
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
};

export function CommitsSection({ commits, isLast, onOpenCommitDetail }: CommitsSectionProps) {
  return (
    <TimelineSection
      dotColor={DOT_COLORS.commits}
      label="Commits"
      count={commits.length}
      isLast={isLast}
    >
      <ul className="space-y-0.5">
        {commits.map((commit) => (
          <li
            key={commit.commit_sha}
            className="flex items-center gap-2 text-xs rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
            onClick={() => onOpenCommitDetail?.(commit.commit_sha)}
          >
            <IconGitCommit className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
            <code className="font-mono text-muted-foreground text-[11px]">
              {commit.commit_sha.slice(0, 7)}
            </code>
            <span className="flex-1 min-w-0 truncate text-foreground">
              {commit.commit_message}
            </span>
            <span className="shrink-0 text-[11px]">
              <span className="text-emerald-500">+{commit.insertions}</span>
              {' '}
              <span className="text-rose-500">-{commit.deletions}</span>
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
  isLoading: boolean;
  aheadCount: number;
};

export function ActionButtonsSection({ onOpenPRDialog, onPush, isLoading, aheadCount }: ActionButtonsSectionProps) {
  return (
    <TimelineSection dotColor={DOT_COLORS.action} isLast>
      <div className="flex items-center gap-2 -mt-0.5">
        <Button
          size="sm"
          variant="outline"
          className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
          onClick={onOpenPRDialog}
        >
          <IconGitPullRequest className="h-3 w-3" />
          Create PR
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
          onClick={onPush}
          disabled={isLoading}
        >
          <IconCloudUpload className="h-3 w-3" />
          Push
          {aheadCount > 0 && (
            <span className="text-muted-foreground">{aheadCount} ahead</span>
          )}
        </Button>
      </div>
    </TimelineSection>
  );
}

// --- File list sections (Unstaged / Staged) ---

import { FileRow } from './changes-panel-file-row';

type FileListSectionProps = {
  variant: 'unstaged' | 'staged';
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
  const dotColor = variant === 'unstaged' ? DOT_COLORS.unstaged : DOT_COLORS.staged;
  const label = variant === 'unstaged' ? 'Unstaged' : 'Staged';

  return (
    <TimelineSection
      dotColor={dotColor}
      label={label}
      count={files.length}
      isLast={isLast}
      action={
        <Button
          size="sm"
          variant="outline"
          className="h-5 text-[10px] px-1.5 cursor-pointer"
          onClick={onAction}
        >
          {actionLabel}
        </Button>
      }
    >
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
    </TimelineSection>
  );
}

// --- Review progress bar ---

type ReviewProgressBarProps = {
  reviewedCount: number;
  totalFileCount: number;
  onOpenReview?: () => void;
};

export function ReviewProgressBar({ reviewedCount, totalFileCount, onOpenReview }: ReviewProgressBarProps) {
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
