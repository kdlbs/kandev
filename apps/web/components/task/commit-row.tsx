"use client";

import {
  IconGitCommit,
  IconArrowBackUp,
  IconPencil,
  IconHistoryToggle,
  IconArrowUp,
  IconExternalLink,
} from "@tabler/icons-react";
import { useState } from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { timeAgo } from "@/lib/utils/time";
import type { CommitDetailTarget, CommitFileNavigationRequest } from "./changes-diff-target";
import { CommitRowFiles } from "./commit-row-files";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type CommitPresentation = "current_pr" | "local_checkout";

const provenanceAccessibleLabelKey = {
  current_pr: "task:currentPRCommit",
  local_checkout: "task:localCheckoutCommit",
  pushed: "task:pushedCommit",
  unpushed: "task:unpushedCommit",
} as const;

export type CommitItem = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
  pushed?: boolean;
  statsAvailable: boolean;
  detailTarget: CommitDetailTarget;
  /** Multi-repo: name of the repo this commit was made in. Empty for single-repo. */
  repository_name?: string;
  committed_at?: string;
  /** Explicit provenance used when provider and checkout histories diverge. */
  presentation?: CommitPresentation;
};

let nextCommitFileNavigationToken = 0;

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
  // Multi-repo: handlers receive the commit's repository_name so the
  // amend/revert/reset op runs in the right git repo. Without it, ops hit
  // the workspace root which fails on multi-repo task workspaces.
  onAmendCommit?: (currentMessage: string, repo?: string) => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onResetToCommit?: (sha: string, repo?: string) => void;
}) {
  const { t } = useTranslation();
  const hasActions =
    commit.detailTarget.source === "local" && (onAmendCommit || onRevertCommit || onResetToCommit);

  if (!hasActions) {
    return <>{children}</>;
  }

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>
        {isLatest && onAmendCommit && (
          <ContextMenuItem
            onSelect={() => onAmendCommit(commit.commit_message, commit.repository_name)}
          >
            <IconPencil className="h-3.5 w-3.5" />
            {t("task:amendMessage")}
          </ContextMenuItem>
        )}
        {isLatest && onRevertCommit && (
          <ContextMenuItem
            onSelect={() => onRevertCommit(commit.commit_sha, commit.repository_name)}
          >
            <IconArrowBackUp className="h-3.5 w-3.5" />
            {t("task:revertCommit")}
          </ContextMenuItem>
        )}
        {onResetToCommit && (
          <ContextMenuItem
            onSelect={() => onResetToCommit(commit.commit_sha, commit.repository_name)}
          >
            <IconHistoryToggle className="h-3.5 w-3.5" />
            {t("task:resetToThisCommit")}
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
  // Multi-repo: handlers receive the commit's repository_name so the
  // amend/revert/reset op runs in the right git repo. Without it, ops hit
  // the workspace root which fails on multi-repo task workspaces.
  onAmendCommit?: (currentMessage: string, repo?: string) => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onResetToCommit?: (sha: string, repo?: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <span className="hidden group-hover:flex items-center gap-1">
      {isLatest && onAmendCommit && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label={t("task:amendCommitMessage2")}
              className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
              onClick={(e) => {
                e.stopPropagation();
                onAmendCommit(commit.commit_message, commit.repository_name);
              }}
            >
              <IconPencil className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>{t("task:amendCommitMessage2")}</TooltipContent>
        </Tooltip>
      )}
      {isLatest && onRevertCommit && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label={t("task:revertCommit")}
              className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
              onClick={(e) => {
                e.stopPropagation();
                onRevertCommit(commit.commit_sha, commit.repository_name);
              }}
            >
              <IconArrowBackUp className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>{t("task:revertCommit")}</TooltipContent>
        </Tooltip>
      )}
      {onResetToCommit && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label={t("task:resetToThisCommit")}
              className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
              onClick={(e) => {
                e.stopPropagation();
                onResetToCommit(commit.commit_sha, commit.repository_name);
              }}
            >
              <IconHistoryToggle className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>{t("task:resetToThisCommit")}</TooltipContent>
        </Tooltip>
      )}
    </span>
  );
}

function CommitStatusMarker({ commit }: { commit: CommitItem }) {
  const { t } = useTranslation();
  let label: string;
  let provenance: "current_pr" | "local_checkout" | "pushed" | "unpushed";
  let marker = <IconGitCommit aria-hidden="true" className="h-3.5 w-3.5 text-muted-foreground" />;

  if (commit.presentation === "current_pr") {
    label = t("task:currentPRCommit");
    provenance = "current_pr";
    marker = <IconGitCommit aria-hidden="true" className="h-3.5 w-3.5 text-violet-500" />;
  } else if (commit.presentation === "local_checkout") {
    label = t("task:localCheckoutCommit");
    provenance = "local_checkout";
    marker = <IconGitCommit aria-hidden="true" className="h-3.5 w-3.5 text-amber-500" />;
  } else if (commit.pushed === true) {
    label = t("task:pushedToRemote");
    provenance = "pushed";
  } else {
    label = t("task:localCommitNotYetPushed");
    provenance = "unpushed";
    marker = <IconArrowUp aria-hidden="true" className="h-3.5 w-3.5 text-emerald-500" />;
  }

  const accessibleLabel = t(provenanceAccessibleLabelKey[provenance]);

  return (
    <span
      className="shrink-0"
      title={label}
      data-testid="commit-provenance"
      data-commit-provenance={provenance}
    >
      <span className="sr-only">{accessibleLabel}</span>
      {marker}
    </span>
  );
}

function CommitOpenButton({
  commitSha,
  onOpen,
  touchSized,
}: {
  commitSha: string;
  onOpen?: () => void;
  touchSized: boolean;
}) {
  const { t } = useTranslation();
  const label = t("task:openCommit");
  return (
    <button
      type="button"
      data-testid={`commit-open-${commitSha.slice(0, 7)}`}
      aria-label={label}
      title={label}
      className={cn(
        "inline-flex shrink-0 cursor-pointer items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground",
        touchSized ? "size-11" : "size-4",
      )}
      onClick={onOpen}
    >
      <IconExternalLink className="size-3.5" />
    </button>
  );
}

function CommitRowToggle({
  commit,
  expanded,
  touchSized,
  inlineFilesId,
  onToggle,
}: {
  commit: CommitItem;
  expanded: boolean;
  touchSized: boolean;
  inlineFilesId: string;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      data-testid="commit-toggle"
      aria-expanded={expanded}
      aria-controls={inlineFilesId}
      className={cn(
        "flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left",
        touchSized && "min-h-11",
      )}
      onClick={onToggle}
    >
      <span className="flex min-w-0 flex-1 flex-col gap-0.5 md:flex-row md:items-center md:gap-2">
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <CommitStatusMarker commit={commit} />
          <code className="font-mono text-muted-foreground text-[11px]">
            {commit.commit_sha.slice(0, 7)}
          </code>
          <span className="min-w-0 flex-1 truncate text-foreground" title={commit.commit_message}>
            {commit.commit_message}
          </span>
        </span>
        {commit.statsAvailable && (
          <span
            data-testid="commit-stats"
            className={`mr-1 flex shrink-0 items-center gap-1 text-[11px] ${commit.committed_at ? "group-hover:hidden" : ""}`}
          >
            <span className="text-emerald-500">+{commit.insertions}</span>{" "}
            <span className="text-rose-500">-{commit.deletions}</span>
          </span>
        )}
        {commit.committed_at && (
          <span className="mr-1 hidden shrink-0 items-center gap-1 text-[11px] text-muted-foreground group-hover:flex">
            {timeAgo(commit.committed_at)}
          </span>
        )}
      </span>
    </button>
  );
}

function useCommitFileNavigationHandler(
  commit: CommitItem,
  onOpenCommitDetail?: (
    target: CommitDetailTarget,
    fileNavigation?: CommitFileNavigationRequest,
  ) => void,
) {
  return (path: string) => {
    onOpenCommitDetail?.(commit.detailTarget, {
      path,
      token: (nextCommitFileNavigationToken += 1),
    });
  };
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
  // Multi-repo: opening the diff for a non-primary repo's commit needs the
  // repo subpath, otherwise the agentctl looks up the SHA at the workspace
  // root and finds nothing (each repo has its own commit graph).
  onOpenCommitDetail?: (
    target: CommitDetailTarget,
    fileNavigation?: CommitFileNavigationRequest,
  ) => void;
  // Multi-repo: handlers receive the commit's repository_name so the
  // amend/revert/reset op runs in the right git repo. Without it, ops hit
  // the workspace root which fails on multi-repo task workspaces.
  onAmendCommit?: (currentMessage: string, repo?: string) => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onResetToCommit?: (sha: string, repo?: string) => void;
}) {
  const isLocalCommit = commit.detailTarget.source === "local";
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touchSized = isMobile || isFinePointer === false;
  const [expanded, setExpanded] = useState(false);
  const [hasExpanded, setHasExpanded] = useState(false);
  const showActions =
    isLocalCommit && (onResetToCommit || (isLatest && (onAmendCommit || onRevertCommit)));
  const inlineFilesId = `commit-inline-files-${commit.detailTarget.source}-${commit.commit_sha}-${
    commit.repository_name ?? "root"
  }`.replace(/[^a-zA-Z0-9_-]/g, "-");
  const handleToggle = () => {
    if (!expanded) setHasExpanded(true);
    setExpanded((previous) => !previous);
  };
  const handleOpenFile = useCommitFileNavigationHandler(commit, onOpenCommitDetail);
  return (
    <CommitContextMenu
      commit={commit}
      isLatest={isLatest}
      onAmendCommit={onAmendCommit}
      onRevertCommit={onRevertCommit}
      onResetToCommit={onResetToCommit}
    >
      <li
        data-testid={`commit-row-${commit.commit_sha.slice(0, 7)}`}
        className="group relative -mx-1 rounded-md px-1 py-1 text-xs hover:bg-muted/60"
      >
        <div className="flex items-center gap-2">
          <CommitRowToggle
            commit={commit}
            expanded={expanded}
            touchSized={touchSized}
            inlineFilesId={inlineFilesId}
            onToggle={handleToggle}
          />
          <CommitOpenButton
            commitSha={commit.commit_sha}
            onOpen={() => onOpenCommitDetail?.(commit.detailTarget)}
            touchSized={touchSized}
          />
          {showActions && (
            <CommitRowActions
              commit={commit}
              isLatest={isLatest}
              onAmendCommit={onAmendCommit}
              onRevertCommit={onRevertCommit}
              onResetToCommit={onResetToCommit}
            />
          )}
        </div>
        {hasExpanded && (
          <div id={inlineFilesId} hidden={!expanded} data-testid="commit-inline-files">
            <CommitRowFiles target={commit.detailTarget} onOpenFile={handleOpenFile} />
          </div>
        )}
      </li>
    </CommitContextMenu>
  );
}
