"use client";

import {
  IconCloudDownload,
  IconEye,
  IconChevronDown,
  IconGitBranch,
  IconGitCherryPick,
  IconGitMerge,
  IconArrowRight,
  IconLoader2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { PanelHeaderBarSplit } from "./panel-primitives";

type PerRepoStatus = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

type BranchRow = { repoLabel: string | null; branch: string; baseBranch: string };

/**
 * Builds per-repo rows for the branch hover card. Returns [] for single-repo
 * workspaces (callers fall back to the single-row layout); otherwise one row
 * per named repo with that repo's task base_branch (or the workspace-level
 * fallback when none was recorded).
 */
function buildBranchRows(
  perRepoStatus: PerRepoStatus[],
  baseBranchByRepo: Record<string, string> | undefined,
  baseBranchFallback: string,
  repoDisplayName: ((name: string) => string | undefined) | undefined,
): BranchRow[] {
  const named = perRepoStatus.filter((s) => s.repository_name !== "" && s.branch);
  if (named.length <= 1) return [];
  return named.map((s) => ({
    repoLabel: repoDisplayName?.(s.repository_name) || s.repository_name,
    branch: s.branch ?? "",
    baseBranch: baseBranchByRepo?.[s.repository_name] || baseBranchFallback,
  }));
}

function BranchRowView({ repoLabel, branch, baseBranch }: BranchRow) {
  return (
    <div className="flex items-center gap-2">
      {repoLabel && (
        <span className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[10px] font-medium text-muted-foreground max-w-[8rem] truncate">
          {repoLabel}
        </span>
      )}
      <span className="flex items-center gap-1.5 text-foreground font-medium">
        <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground" />
        {branch}
      </span>
      <div className="flex-1 border-t border-muted-foreground/20 min-w-8" />
      <IconArrowRight className="h-3 w-3 text-muted-foreground/40" />
      <span className="text-foreground font-medium">{baseBranch}</span>
    </div>
  );
}

function BranchHoverCard({
  displayBranch,
  baseBranchDisplay,
  rows,
}: {
  displayBranch: string;
  baseBranchDisplay: string;
  /** When non-empty, the card renders one row per repo instead of the single
   *  workspace-level pair. Single-repo workspaces leave this undefined. */
  rows?: BranchRow[];
}) {
  const isMulti = rows && rows.length > 0;
  const headerLabel = isMulti ? "Your branches:" : "Your code lives in:";
  const trailerLabel = "and will be merged into:";
  return (
    <HoverCard openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          className="flex items-center justify-center size-5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors cursor-default"
        >
          <IconGitBranch className="h-3.5 w-3.5" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent side="bottom" align="end" className="w-auto p-3">
        <div className="flex flex-col gap-2.5 text-xs">
          <div className="flex items-center justify-between gap-6">
            <span className="text-muted-foreground/60">{headerLabel}</span>
            <span className="text-muted-foreground/60">{trailerLabel}</span>
          </div>
          {isMulti ? (
            <div className="flex flex-col gap-1.5">
              {rows!.map((row) => (
                <BranchRowView key={row.repoLabel ?? row.branch} {...row} />
              ))}
            </div>
          ) : (
            <BranchRowView repoLabel={null} branch={displayBranch} baseBranch={baseBranchDisplay} />
          )}
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}

function PullTriggerContent({
  behindCount,
  isPulling,
  isRebasing,
}: {
  behindCount: number;
  isPulling: boolean;
  isRebasing: boolean;
}) {
  const isPullRelated = isPulling || isRebasing;
  let label: string;
  if (isPulling) label = "Pulling…";
  else if (isRebasing) label = "Rebasing…";
  else label = "Pull";
  return (
    <>
      {isPullRelated ? (
        <IconLoader2 className="h-3 w-3 animate-spin" />
      ) : (
        <IconCloudDownload className="h-3 w-3" />
      )}
      {label}
      {behindCount > 0 && !isPullRelated && (
        <span className="text-yellow-500 text-[10px]">{behindCount}</span>
      )}
      {!isPullRelated && <IconChevronDown className="h-2.5 w-2.5 text-muted-foreground" />}
    </>
  );
}

function PullDropdown({
  behindCount,
  isLoading,
  loadingOperation,
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
}: {
  behindCount: number;
  isLoading: boolean;
  loadingOperation: string | null;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  /** Maps a repository_name to its display label. */
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const isPulling = loadingOperation === "pull";
  const isRebasing = loadingOperation === "rebase";
  // For single-repo (empty repo entry), the trigger label uses the global
  // behindCount; for multi-repo we show the per-repo behinds inside the menu
  // labels and the trigger summarises with the max.
  const triggerBehind =
    perRepoStatus.length > 0
      ? Math.max(behindCount, ...perRepoStatus.map((s) => s.behind))
      : behindCount;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
          disabled={isLoading}
        >
          <PullTriggerContent
            behindCount={triggerBehind}
            isPulling={isPulling}
            isRebasing={isRebasing}
          />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <PerRepoPullMenu
          repoNames={repoNames}
          perRepoStatus={perRepoStatus}
          onRepoPull={onRepoPull}
          onRepoRebase={onRepoRebase}
          onRepoMerge={onRepoMerge}
          repoDisplayName={repoDisplayName}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function PerRepoPullMenu({
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
}: {
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const statusByName = new Map(perRepoStatus.map((s) => [s.repository_name, s]));
  return (
    <>
      {repoNames.map((repo, idx) => {
        const s = statusByName.get(repo);
        const behind = s?.behind ?? 0;
        const label = repoDisplayName?.(repo) || repo || "Repository";
        return (
          <div key={repo || "__no_repo__"}>
            {idx > 0 && <DropdownMenuSeparator />}
            <DropdownMenuLabel className="text-[10px] text-muted-foreground/70 uppercase tracking-wide flex items-center justify-between">
              <span className="truncate">{label}</span>
              {behind > 0 && (
                <span className="text-yellow-500 normal-case tracking-normal">{behind} behind</span>
              )}
            </DropdownMenuLabel>
            <DropdownMenuItem
              onClick={() => onRepoPull(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconCloudDownload className="h-3.5 w-3.5 text-muted-foreground" />
              Pull
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onRepoRebase(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconGitCherryPick className="h-3.5 w-3.5 text-muted-foreground" />
              Rebase
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onRepoMerge(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconGitMerge className="h-3.5 w-3.5 text-muted-foreground" />
              Merge
            </DropdownMenuItem>
          </div>
        );
      })}
    </>
  );
}

export function ChangesPanelHeader({
  hasChanges,
  hasCommits,
  hasPRFiles,
  displayBranch,
  baseBranchDisplay,
  baseBranchByRepo,
  behindCount,
  isLoading,
  loadingOperation,
  onOpenDiffAll,
  onOpenReview,
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
}: {
  hasChanges: boolean;
  hasCommits: boolean;
  hasPRFiles?: boolean;
  displayBranch: string | null;
  baseBranchDisplay: string;
  /** Per-repo merge target, keyed by repository_name. Undefined entries fall
   *  back to baseBranchDisplay. Empty/missing for single-repo workspaces. */
  baseBranchByRepo?: Record<string, string>;
  behindCount: number;
  isLoading: boolean;
  loadingOperation: string | null;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const branchRows = buildBranchRows(
    perRepoStatus,
    baseBranchByRepo,
    baseBranchDisplay,
    repoDisplayName,
  );
  const showDiffReview = hasChanges || hasCommits || !!hasPRFiles;
  return (
    <PanelHeaderBarSplit
      left={
        showDiffReview ? (
          <>
            <Button
              size="sm"
              variant="ghost"
              className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
              onClick={onOpenDiffAll}
            >
              <IconGitMerge className="h-3 w-3" />
              Diff
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
              onClick={onOpenReview}
            >
              <IconEye className="h-3 w-3" />
              Review
            </Button>
          </>
        ) : undefined
      }
      right={
        <>
          {(displayBranch || branchRows.length > 0) && (
            <BranchHoverCard
              displayBranch={displayBranch ?? ""}
              baseBranchDisplay={baseBranchDisplay}
              rows={branchRows}
            />
          )}
          <PullDropdown
            behindCount={behindCount}
            isLoading={isLoading}
            loadingOperation={loadingOperation}
            repoNames={repoNames}
            perRepoStatus={perRepoStatus}
            onRepoPull={onRepoPull}
            onRepoRebase={onRepoRebase}
            onRepoMerge={onRepoMerge}
            repoDisplayName={repoDisplayName}
          />
        </>
      }
    />
  );
}
