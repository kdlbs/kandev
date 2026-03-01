"use client";

import {
  IconGitCommit,
  IconArrowBackUp,
  IconPencil,
  IconHistoryToggle,
} from "@tabler/icons-react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";

export type CommitItem = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
};

/** Context menu for commit items */
function CommitContextMenu({
  children,
  commit,
  isLatest,
  onAmendCommit,
  onRevertCommit,
  onResetToCommit,
}: {
  children: React.ReactNode;
  commit: CommitItem;
  isLatest: boolean;
  onAmendCommit?: (currentMessage: string) => void;
  onRevertCommit?: (sha: string) => void;
  onResetToCommit?: (sha: string) => void;
}) {
  const hasActions = onAmendCommit || onRevertCommit || onResetToCommit;

  if (!hasActions) {
    return <>{children}</>;
  }

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>
        {isLatest && onAmendCommit && (
          <ContextMenuItem onSelect={() => onAmendCommit(commit.commit_message)}>
            <IconPencil className="h-3.5 w-3.5" />
            Amend message
          </ContextMenuItem>
        )}
        {isLatest && onRevertCommit && (
          <ContextMenuItem onSelect={() => onRevertCommit(commit.commit_sha)}>
            <IconArrowBackUp className="h-3.5 w-3.5" />
            Revert commit
          </ContextMenuItem>
        )}
        {onResetToCommit && (
          <ContextMenuItem onSelect={() => onResetToCommit(commit.commit_sha)}>
            <IconHistoryToggle className="h-3.5 w-3.5" />
            Reset to this commit
          </ContextMenuItem>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

/** Hover action buttons for a commit row */
function CommitRowActions({
  commit,
  isLatest,
  onAmendCommit,
  onRevertCommit,
  onResetToCommit,
}: {
  commit: CommitItem;
  isLatest: boolean;
  onAmendCommit?: (currentMessage: string) => void;
  onRevertCommit?: (sha: string) => void;
  onResetToCommit?: (sha: string) => void;
}) {
  return (
    <span className="absolute right-2 top-0 bottom-0 hidden group-hover:flex items-center gap-1">
      {isLatest && onAmendCommit && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label="Amend commit message"
              className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
              onClick={(e) => {
                e.stopPropagation();
                onAmendCommit(commit.commit_message);
              }}
            >
              <IconPencil className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>Amend commit message</TooltipContent>
        </Tooltip>
      )}
      {isLatest && onRevertCommit && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label="Revert commit"
              className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
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
      {onResetToCommit && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label="Reset to this commit"
              className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
              onClick={(e) => {
                e.stopPropagation();
                onResetToCommit(commit.commit_sha);
              }}
            >
              <IconHistoryToggle className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>Reset to this commit</TooltipContent>
        </Tooltip>
      )}
    </span>
  );
}

/** Individual commit row with hover actions */
export function CommitRow({
  commit,
  isLatest,
  onOpenCommitDetail,
  onAmendCommit,
  onRevertCommit,
  onResetToCommit,
}: {
  commit: CommitItem;
  isLatest: boolean;
  onOpenCommitDetail?: (sha: string) => void;
  onAmendCommit?: (currentMessage: string) => void;
  onRevertCommit?: (sha: string) => void;
  onResetToCommit?: (sha: string) => void;
}) {
  const showActions = onResetToCommit || (isLatest && (onAmendCommit || onRevertCommit));

  return (
    <CommitContextMenu
      commit={commit}
      isLatest={isLatest}
      onAmendCommit={onAmendCommit}
      onRevertCommit={onRevertCommit}
      onResetToCommit={onResetToCommit}
    >
      <li
        className="group relative flex items-center gap-2 text-xs rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
        onClick={() => onOpenCommitDetail?.(commit.commit_sha)}
      >
        <IconGitCommit className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
        <code className="font-mono text-muted-foreground text-[11px]">
          {commit.commit_sha.slice(0, 7)}
        </code>
        <span className="flex-1 min-w-0 truncate text-foreground">{commit.commit_message}</span>
        <span className="shrink-0 text-[11px] flex items-center gap-1 mr-1 group-hover:opacity-0">
          <span className="text-emerald-500">+{commit.insertions}</span>{" "}
          <span className="text-rose-500">-{commit.deletions}</span>
        </span>
        {showActions && (
          <CommitRowActions
            commit={commit}
            isLatest={isLatest}
            onAmendCommit={onAmendCommit}
            onRevertCommit={onRevertCommit}
            onResetToCommit={onResetToCommit}
          />
        )}
      </li>
    </CommitContextMenu>
  );
}

