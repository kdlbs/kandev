"use client";

import { useState, useMemo, useCallback } from "react";
import {
  IconGitPullRequest,
  IconCloudUpload,
  IconChevronDown,
  IconChevronRight,
  IconLoader2,
} from "@tabler/icons-react";
import { useMultiSelect } from "@/hooks/use-multi-select";

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
import { FileRow, BulkActionBar } from "./changes-panel-file-row";
import { type CommitItem } from "./commit-row";
import { groupByRepositoryName } from "@/lib/group-by-repo";
import { CommitsRepoGroup, RepoGroupItem } from "./changes-panel-repo-groups";
import { PRFilesGroupedList } from "./changes-panel-pr-files";

// --- Timeline visual components ---

type ChangedFile = {
  path: string;
  status: FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
  /** Repository this file belongs to in multi-repo workspaces; empty for single-repo. */
  repositoryName?: string;
};

// Per-repo grouping is shared with the Review dialog — see @/lib/group-by-repo.

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
  collapsible = true,
  defaultCollapsed = false,
  "data-testid": testId,
}: {
  dotColor: string;
  label?: string;
  count?: number;
  action?: React.ReactNode;
  isLast?: boolean;
  children?: React.ReactNode;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  "data-testid"?: string;
}) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  const canCollapse = collapsible && !!label;

  return (
    <div className="relative flex gap-2.5" data-testid={testId}>
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
            {canCollapse ? (
              <button
                type="button"
                className="flex items-center gap-1 text-[11px] font-medium uppercase tracking-wider text-foreground/70 cursor-pointer hover:text-foreground/90"
                onClick={() => setCollapsed((c) => !c)}
                aria-expanded={!collapsed}
                data-testid={`${testId ?? label.toLowerCase()}-collapse-toggle`}
              >
                {label}
                {typeof count === "number" && (
                  <span className="text-muted-foreground/50 font-normal">({count})</span>
                )}
                {collapsed ? (
                  <IconChevronRight className="h-3 w-3 text-muted-foreground/50" />
                ) : (
                  <IconChevronDown className="h-3 w-3 text-muted-foreground/50" />
                )}
              </button>
            ) : (
              <span className="text-[11px] font-medium uppercase tracking-wider text-foreground/70">
                {label}
                {typeof count === "number" && (
                  <span className="ml-1 text-muted-foreground/50 font-normal">({count})</span>
                )}
              </span>
            )}
            {action}
          </div>
        )}

        {/* Children (file list, buttons, etc.) */}
        {!collapsed && children}
      </div>
    </div>
  );
}

// --- Commits section ---

type CommitsSectionProps = {
  commits: CommitItem[];
  isLast: boolean;
  onOpenCommitDetail?: (sha: string, repo?: string) => void;
  // Handlers receive the commit's repository_name so amend/revert/reset land
  // in the right git repo. The empty string routes to the workspace root for
  // single-repo workspaces.
  onRevertCommit?: (sha: string, repo?: string) => void;
  onAmendCommit?: (currentMessage: string, repo?: string) => void;
  onResetToCommit?: (sha: string, repo?: string) => void;
  /** Per-repo Push button rendered in each commits group header. */
  onRepoPush?: (repo: string) => void;
  /** Per-repo Create PR button rendered in each commits group header. */
  onRepoCreatePR?: (repo: string) => void;
  /** Maps a repository_name to its display label (used for the empty single-repo case). */
  repoDisplayName?: (repositoryName: string) => string | undefined;
  /** The session's base branch — passed through to the per-repo PR dialog. */
  repoBaseBranch?: string;
  /** Per-repo branch / ahead / behind summary; used for the "ahead" indicator. */
  perRepoStatus?: Array<{ repository_name: string; ahead: number }>;
  /** Existing PR URL keyed by repository_name; "" key for single-repo. */
  prByRepo?: Record<string, string | undefined>;
};

// Commits grouping shares the helper above with files — see @/lib/group-by-repo.

export function CommitsSection({
  commits,
  isLast,
  onOpenCommitDetail,
  onRevertCommit,
  onAmendCommit,
  onResetToCommit,
  onRepoPush,
  onRepoCreatePR,
  repoDisplayName,
  repoBaseBranch,
  perRepoStatus,
  prByRepo,
}: CommitsSectionProps) {
  // Always group by repo. Single-repo shows up as one group with an empty
  // repository_name; the group header still renders with a display label and
  // per-repo buttons (Push, Create PR) so the UX is uniform across modes.
  const groups = groupByRepositoryName(commits, (c) => c.repository_name);
  const aheadByRepo = new Map((perRepoStatus ?? []).map((s) => [s.repository_name, s.ahead]));

  return (
    <TimelineSection
      dotColor={DOT_COLORS.commits}
      label="Commits"
      count={commits.length}
      isLast={isLast}
      data-testid="commits-section"
    >
      <ul data-testid="commits-list" className="space-y-0.5">
        {groups.map((g) => (
          <CommitsRepoGroup
            key={g.repositoryName || "__no_repo__"}
            repositoryName={g.repositoryName}
            displayName={repoDisplayName?.(g.repositoryName)}
            groupCommits={g.items}
            aheadCount={aheadByRepo.get(g.repositoryName) ?? 0}
            existingPrUrl={prByRepo?.[g.repositoryName] ?? prByRepo?.[""]}
            baseBranch={repoBaseBranch}
            onOpenCommitDetail={onOpenCommitDetail}
            onAmendCommit={onAmendCommit}
            onRevertCommit={onRevertCommit}
            onResetToCommit={onResetToCommit}
            onRepoPush={onRepoPush}
            onRepoCreatePR={onRepoCreatePR}
          />
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
  loadingOperation: string | null;
  aheadCount: number;
  canPush: boolean;
  canCreatePR: boolean;
  existingPrUrl?: string;
  isLast?: boolean;
  /** Hide the Push button (per-repo Push is rendered elsewhere in multi-repo). */
  hidePush?: boolean;
};

export function ActionButtonsSection({
  onOpenPRDialog,
  onPush,
  onForcePush,
  isLoading,
  loadingOperation,
  aheadCount,
  canPush,
  canCreatePR,
  existingPrUrl,
  isLast = true,
  hidePush = false,
}: ActionButtonsSectionProps) {
  const prExists = !!existingPrUrl;
  const createPrDisabled = !canCreatePR || prExists || isLoading;
  const pushDisabled = !canPush || isLoading;
  const isPushing = loadingOperation === "push";
  const isCreatingPR = loadingOperation === "create_pr";
  let pushTooltip: string | null = null;
  if (isLoading) pushTooltip = "A git operation is in progress";
  else if (!canPush) pushTooltip = "No commits ahead of remote";
  return (
    <TimelineSection dotColor={DOT_COLORS.action} isLast={isLast}>
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
                {isCreatingPR ? (
                  <IconLoader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <IconGitPullRequest className="h-3 w-3" />
                )}
                Create PR
              </Button>
            </span>
          </TooltipTrigger>
          {prExists && <TooltipContent>A pull request already exists for this task</TooltipContent>}
        </Tooltip>
        {!hidePush && (
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
                  {isPushing ? (
                    <IconLoader2 className="h-3 w-3 animate-spin" />
                  ) : (
                    <IconCloudUpload className="h-3 w-3" />
                  )}
                  Push
                  {aheadCount > 0 && !isPushing && (
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
                    <DropdownMenuItem
                      className="cursor-pointer gap-2 text-xs"
                      onClick={onForcePush}
                    >
                      <IconCloudUpload className="h-3.5 w-3.5 shrink-0" />
                      Force Push (with lease)
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </span>
            </TooltipTrigger>
            {pushTooltip && <TooltipContent>{pushTooltip}</TooltipContent>}
          </Tooltip>
        )}
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
  isActionLoading?: boolean;
  onAction: () => void;
  secondaryActionLabel?: string;
  isSecondaryActionLoading?: boolean;
  onSecondaryAction?: () => void;
  onOpenDiff: (path: string) => void;
  onEditFile: (path: string) => void;
  // Multi-repo: handlers receive the file's repositoryName so each per-file op
  // hits the right git repo. Same-named files across repos collide by path.
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
  onDiscard: (path: string, repo?: string) => void;
  onBulkStage?: (paths: string[]) => void;
  onBulkUnstage?: (paths: string[]) => void;
  onBulkDiscard?: (paths: string[]) => void;
  // Per-repo action handlers shown inline with each repo group header. Always
  // rendered, including single-repo (with one entry pre-selected) — the empty
  // `repo` argument routes ops to the workspace root in that case.
  onRepoAction?: (repo: string) => void;
  onRepoSecondaryAction?: (repo: string) => void;
  /** Maps a repository_name to its display label; called per group header. */
  repoDisplayName?: (repositoryName: string) => string | undefined;
};

/**
 * Renders the body of a file-list section. For multi-repo workspaces it
 * groups files by repository under per-repo subheaders; single-repo
 * workspaces fall back to a flat list (no header) so the existing UI is
 * unchanged.
 */
type FileListBodyProps = {
  variant: "unstaged" | "staged";
  files: ChangedFile[];
  pendingStageFiles: Set<string>;
  multiSelect: ReturnType<typeof useMultiSelect>;
  onKeyDown: (e: React.KeyboardEvent) => void;
  onOpenDiff: (path: string) => void;
  onEditFile: (path: string) => void;
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
  onDiscard: (path: string, repo?: string) => void;
  /** Per-repo group header actions; primary = Stage all (unstaged) / Commit (staged). */
  onRepoAction?: (repo: string) => void;
  onRepoSecondaryAction?: (repo: string) => void;
  primaryLabel: string;
  secondaryLabel?: string;
  /** Maps a repository_name to its display label (e.g. "" → workspace primary repo name). */
  repoDisplayName?: (repositoryName: string) => string | undefined;
};

function FileListBody(props: FileListBodyProps) {
  const { variant, files, pendingStageFiles, multiSelect } = props;
  const groups = useMemo(() => groupByRepositoryName(files, (f) => f.repositoryName), [files]);
  // Per-repo collapsed state: keyed by repositoryName. Default expanded;
  // setting an entry to true collapses that group. Persists across re-renders
  // for the lifetime of this section instance (resets on unmount).
  const [collapsedRepos, setCollapsedRepos] = useState<Set<string>>(() => new Set());
  const toggleRepo = useCallback((name: string) => {
    setCollapsedRepos((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const renderRow = (file: ChangedFile) => (
    <FileRow
      key={`${file.repositoryName ?? ""}:${file.path}`}
      file={file}
      isPending={pendingStageFiles.has(`${file.repositoryName ?? ""}::${file.path}`)}
      isSelected={multiSelect.isSelected(file.path)}
      onSelect={multiSelect.handleClick}
      onOpenDiff={props.onOpenDiff}
      onStage={props.onStage}
      onUnstage={props.onUnstage}
      onDiscard={props.onDiscard}
      onEditFile={props.onEditFile}
    />
  );

  return (
    <div>
      <ul
        data-testid={`${variant}-file-list`}
        className="space-y-0.5"
        tabIndex={-1}
        onKeyDown={props.onKeyDown}
      >
        {groups.map((group) => (
          <RepoGroupItem
            key={group.repositoryName || "__no_repo__"}
            group={group}
            collapsed={collapsedRepos.has(group.repositoryName)}
            onToggle={() => toggleRepo(group.repositoryName)}
            renderRow={renderRow}
            primaryLabel={props.primaryLabel}
            secondaryLabel={props.secondaryLabel}
            onRepoAction={props.onRepoAction}
            onRepoSecondaryAction={props.onRepoSecondaryAction}
            displayName={props.repoDisplayName?.(group.repositoryName)}
          />
        ))}
      </ul>
    </div>
  );
}

export function FileListSection(props: FileListSectionProps) {
  const {
    variant,
    files,
    pendingStageFiles,
    isLast,
    onOpenDiff,
    onEditFile,
    onStage,
    onUnstage,
    onDiscard,
  } = props;
  const dotColor = variant === "unstaged" ? DOT_COLORS.unstaged : DOT_COLORS.staged;
  const label = variant === "unstaged" ? "Unstaged" : "Staged";

  const filePaths = useMemo(() => files.map((f) => f.path), [files]);
  const multiSelect = useMultiSelect({ items: filePaths });
  const hasSelection = multiSelect.selectedPaths.size > 0;
  // Multi-repo when any file has a repositoryName. Per-repo group headers
  // own the action buttons in this case; the bottom-of-section global
  // buttons would be redundant, so we skip them.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape" && hasSelection) multiSelect.clearSelection();
    },
    [hasSelection, multiSelect],
  );

  return (
    <TimelineSection
      dotColor={dotColor}
      label={label}
      count={files.length}
      isLast={isLast}
      data-testid={`${variant}-files-section`}
    >
      {files.length > 0 && (
        <FileListBody
          variant={variant}
          files={files}
          pendingStageFiles={pendingStageFiles}
          multiSelect={multiSelect}
          onKeyDown={handleKeyDown}
          onOpenDiff={onOpenDiff}
          onStage={onStage}
          onUnstage={onUnstage}
          onDiscard={onDiscard}
          onEditFile={onEditFile}
          primaryLabel={props.actionLabel}
          secondaryLabel={props.secondaryActionLabel}
          onRepoAction={props.onRepoAction}
          onRepoSecondaryAction={props.onRepoSecondaryAction}
          repoDisplayName={props.repoDisplayName}
        />
      )}
      {files.length > 0 && hasSelection && (
        <div className="mt-1.5 flex items-center gap-1.5">
          <BulkActionBar
            variant={variant}
            selectionCount={multiSelect.selectedPaths.size}
            selectedPaths={multiSelect.selectedPaths}
            onBulkStage={props.onBulkStage}
            onBulkUnstage={props.onBulkUnstage}
            onBulkDiscard={props.onBulkDiscard}
          />
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
  /**
   * The repository the file belongs to. Empty string for single-repo tasks
   * (the section header alone is enough). Multi-repo tasks stamp this so
   * `PRFilesSection` can group rows under per-repo subheaders, mirroring
   * how the Commits and Changes sections render.
   */
  repository_name?: string;
};

type PRFilesSectionProps = {
  files: PRChangedFile[];
  isLast: boolean;
  onOpenDiff: (path: string) => void;
  /** Maps a repository_name to a human-readable label (used for the per-repo header). */
  repoDisplayName?: (repositoryName: string) => string | undefined;
};

export function PRFilesSection({
  files,
  isLast,
  onOpenDiff,
  repoDisplayName,
}: PRFilesSectionProps) {
  return (
    <TimelineSection
      dotColor={DOT_COLORS.pr}
      label="PR Changes"
      count={files.length}
      isLast={isLast}
      data-testid="pr-changes-section"
    >
      {files.length > 0 && (
        <PRFilesGroupedList
          files={files}
          onOpenDiff={onOpenDiff}
          repoDisplayName={repoDisplayName}
        />
      )}
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
