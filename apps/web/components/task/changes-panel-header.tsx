"use client";

import { useState } from "react";
import {
  IconCloudDownload,
  IconEye,
  IconChevronDown,
  IconGitBranch,
  IconGitCherryPick,
  IconGitMerge,
  IconArrowRight,
  IconLoader2,
  IconEdit,
  IconRoute,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { PanelHeaderBarSplit } from "./panel-primitives";
import { BaseBranchPicker } from "./base-branch-picker";

type PerRepoStatus = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

type BranchRow = {
  repoLabel: string | null;
  branch: string;
  baseBranch: string;
  /** Name agentctl emits for this repo (= worktree dir basename). Empty for
   *  single-repo workspaces; passed to the BaseBranchPicker so it can resolve
   *  the task_repositories row to PATCH. */
  repositoryName: string;
};

type RenameBranchResult = {
  success: boolean;
  error?: string;
};

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
    repositoryName: s.repository_name,
  }));
}

function RenameBranchButton({
  branch,
  repositoryName,
  onRenameBranch,
  isRenaming,
}: {
  branch: string;
  repositoryName: string;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [newBranchName, setNewBranchName] = useState(branch);
  const [error, setError] = useState<string | null>(null);
  const trimmedBranchName = newBranchName.trim();
  const canRename = !!onRenameBranch && trimmedBranchName !== "" && trimmedBranchName !== branch;
  const submitRename = async () => {
    if (!onRenameBranch || !canRename) return;
    setError(null);
    try {
      const result = await onRenameBranch(trimmedBranchName, repositoryName);
      if (!result.success) {
        setError(result.error || "Failed to rename branch");
        return;
      }
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to rename branch");
    }
  };
  const openDialog = () => {
    setNewBranchName(branch);
    setError(null);
    setOpen(true);
  };
  return (
    <>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        className="h-6 w-6 shrink-0 text-muted-foreground hover:text-foreground"
        disabled={!onRenameBranch || isRenaming}
        aria-label={`Edit branch ${branch}`}
        onClick={openDialog}
      >
        <IconEdit className="h-3.5 w-3.5" />
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Edit branch</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor={`branch-name-${repositoryName || "default"}`}>Branch name</Label>
            <Input
              id={`branch-name-${repositoryName || "default"}`}
              value={newBranchName}
              onChange={(event) => setNewBranchName(event.target.value)}
              autoFocus
            />
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              data-dialog-default-action
              disabled={!canRename || isRenaming}
              onClick={submitRename}
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function BranchRowView({
  repoLabel,
  branch,
  baseBranch,
  repositoryName,
  taskId,
  onRenameBranch,
  isRenaming,
}: BranchRow & {
  taskId: string | null;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      {repoLabel && (
        <span className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[10px] font-medium text-muted-foreground max-w-[8rem] truncate">
          {repoLabel}
        </span>
      )}
      <span className="flex min-w-0 items-center gap-1.5 text-foreground font-medium">
        <IconGitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{branch}</span>
      </span>
      <RenameBranchButton
        branch={branch}
        repositoryName={repositoryName}
        onRenameBranch={onRenameBranch}
        isRenaming={isRenaming}
      />
      <div className="flex min-w-8 flex-1 items-center text-muted-foreground/40">
        <div className="h-px flex-1 bg-muted-foreground/20" />
        <IconArrowRight className="-ml-px h-3 w-3 shrink-0" />
      </div>
      <BaseBranchPicker
        taskId={taskId}
        repositoryName={repositoryName}
        fallbackBaseBranch={baseBranch}
      />
    </div>
  );
}

function BranchHoverCard({
  displayBranch,
  baseBranchDisplay,
  rows,
  taskId,
  onRenameBranch,
  isRenaming,
}: {
  displayBranch: string;
  baseBranchDisplay: string;
  /** When non-empty, the card renders one row per repo instead of the single
   *  workspace-level pair. Single-repo workspaces leave this undefined. */
  rows?: BranchRow[];
  /** Active task id, plumbed into BaseBranchPicker for the PATCH call. */
  taskId: string | null;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  const isMulti = rows && rows.length > 0;
  const headerLabel = isMulti ? "Your branches:" : "Your code lives in:";
  const trailerLabel = "comparing against:";
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
      <HoverCardContent forceMount side="bottom" align="end" className="w-auto p-3">
        <div className="flex flex-col gap-2.5 text-xs">
          <div className="flex items-center justify-between gap-6">
            <span className="text-muted-foreground/60">{headerLabel}</span>
            <span className="text-muted-foreground/60">{trailerLabel}</span>
          </div>
          {isMulti ? (
            <div className="flex flex-col gap-1.5">
              {rows!.map((row) => (
                <BranchRowView
                  key={row.repoLabel ?? row.branch}
                  {...row}
                  taskId={taskId}
                  onRenameBranch={onRenameBranch}
                  isRenaming={isRenaming}
                />
              ))}
            </div>
          ) : (
            <BranchRowView
              repoLabel={null}
              branch={displayBranch}
              baseBranch={baseBranchDisplay}
              repositoryName=""
              taskId={taskId}
              onRenameBranch={onRenameBranch}
              isRenaming={isRenaming}
            />
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

function ChangesPanelHeaderLeft({
  showDiffReview,
  onOpenDiffAll,
  onOpenReview,
  onRequestWalkthrough,
  requestWalkthroughDisabled,
}: {
  showDiffReview: boolean;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
}) {
  if (!showDiffReview) return null;
  return (
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
      {onRequestWalkthrough ? (
        <ChangesPanelWalkthroughButton
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      ) : null}
    </>
  );
}

function ChangesPanelWalkthroughButton({
  onRequestWalkthrough,
  requestWalkthroughDisabled,
}: {
  onRequestWalkthrough: () => void;
  requestWalkthroughDisabled?: boolean;
}) {
  const tooltip = requestWalkthroughDisabled
    ? "Loading changed files..."
    : "Walk me through these changes";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex"
          tabIndex={requestWalkthroughDisabled ? 0 : undefined}
          aria-label={requestWalkthroughDisabled ? tooltip : undefined}
        >
          <Button
            size="sm"
            variant="ghost"
            className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
            aria-label="Walk me through these changes"
            data-testid="changes-request-walkthrough"
            disabled={requestWalkthroughDisabled}
            onClick={onRequestWalkthrough}
          >
            <IconRoute className="h-3 w-3" />
            <span className="hidden min-[430px]:inline sm:inline">Walkthrough</span>
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
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
  onRequestWalkthrough,
  requestWalkthroughDisabled,
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
  taskId,
  onRenameBranch,
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
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  repoDisplayName?: (repositoryName: string) => string | undefined;
  /** Active task id; piped into the base-branch picker so it can resolve
   *  the right task_repositories row to PATCH. Null while task data is
   *  hydrating — the picker falls back to a static label. */
  taskId: string | null;
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
        <ChangesPanelHeaderLeft
          showDiffReview={showDiffReview}
          onOpenDiffAll={onOpenDiffAll}
          onOpenReview={onOpenReview}
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      right={
        <>
          {(displayBranch || branchRows.length > 0) && (
            <BranchHoverCard
              displayBranch={displayBranch ?? ""}
              baseBranchDisplay={baseBranchDisplay}
              rows={branchRows}
              taskId={taskId}
              onRenameBranch={onRenameBranch}
              isRenaming={loadingOperation === "rename_branch"}
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
