"use client";

import { useEffect, useMemo } from "react";
import { IconPlus, IconX, IconCode, IconGitBranch, IconGitFork } from "@tabler/icons-react";
import { cn, formatUserHomePath } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import type { LocalRepository, Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { autoSelectBranch } from "@/components/task-create-dialog-helpers";
import { scoreBranch } from "@/lib/utils/branch-filter";
import {
  Pill,
  sortBranches,
  branchToOption,
  computeBranchPlaceholder,
  type PillOption,
} from "@/components/task-create-dialog-pill";
import { GitHubUrlSection } from "@/components/task-create-dialog-github-url";
import {
  computeBranchPrefix,
  computeBranchTooltip,
  computeBranchDisabledReason,
} from "@/components/task-create-dialog-branch-utils";

function runAutoselect({
  branches,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  onBranchChange,
}: {
  branches: Array<{ name: string; type: "local" | "remote"; remote?: string }>;
  preferredDefaultBranch: string | undefined;
  preferredDefaultBranchLoading: boolean;
  onBranchChange: (value: string) => void;
}) {
  if (preferredDefaultBranchLoading) return;
  // Local-executor flow: preferredDefaultBranch is the workspace's current
  // ref (branch name OR detached-HEAD short SHA). Seed row.branch with it
  // unconditionally so the chip displays "current: <ref>". When the ref is a
  // SHA (detached HEAD) it won't appear in the branches list, but falling
  // through to autoSelectBranch would pick main/master and surface "will
  // switch to: master", which contradicts what the user actually has checked
  // out. Sending the SHA back on submit is a no-op because the backend's
  // skip-when-equal check compares against the same SHA.
  if (preferredDefaultBranch) {
    onBranchChange(preferredDefaultBranch);
    return;
  }
  autoSelectBranch(branches, onBranchChange);
}

/**
 * Chip row for the task-create dialog. Renders one chip per row in
 * `fs.repositories`, plus a trailing "+" to add another. Each chip is two
 * pills (repo, branch) and a remove (×). All chips are equivalent — there
 * is no "primary" — and any row can hold either a workspace repo or a
 * discovered on-machine path.
 *
 * In GitHub URL mode the chips are replaced by an inline URL input pill;
 * the trailing toggle flips between the two modes.
 */
type RepoChipsRowProps = {
  fs: DialogFormState;
  repositories: Repository[];
  isTaskStarted: boolean;
  /** Required for loading branches on discovered (path-keyed) rows. */
  workspaceId: string | null;
  /**
   * Per-row repo change handler. Resolves the picked value into either a
   * workspace `repositoryId` or a discovered `localPath` and writes that
   * into the row. Comes from useDialogHandlers so the resolution logic
   * stays in one place.
   */
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  /** GitHub URL flow lives alongside the chips so users can switch in place. */
  onToggleGitHubUrl?: () => void;
  onGitHubUrlChange?: (value: string) => void;
  /**
   * Fresh-branch toggle props. When `freshBranchAvailable` is true the toggle
   * renders inline at the right edge of the chip row so it sits next to the
   * branch pills it affects, instead of taking its own row under the
   * agent/executor selectors.
   */
  freshBranchAvailable?: boolean;
  freshBranchEnabled?: boolean;
  onToggleFreshBranch?: (enabled: boolean) => void;
  /**
   * When the task runs on the local executor, the chip seeds row.branch with
   * the workspace's current branch (so the user sees what's on disk and the
   * submit payload always carries an explicit value). The chip stays
   * editable — picking a different existing branch triggers `git checkout`
   * server-side; keeping the default skips git ops entirely. Fresh-branch
   * mode is independent: it creates a new branch from a chosen base.
   */
  isLocalExecutor?: boolean;
};

export function RepoChipsRow({
  fs,
  repositories,
  isTaskStarted,
  workspaceId,
  onRowRepositoryChange,
  onRowBranchChange,
  onToggleGitHubUrl,
  onGitHubUrlChange,
  freshBranchAvailable,
  freshBranchEnabled,
  onToggleFreshBranch,
  isLocalExecutor,
}: RepoChipsRowProps) {
  // Local executor branch behavior:
  //   - chip is clickable (user can switch to any existing branch on disk)
  //   - row.branch seeds from the workspace's current branch (currentLocalBranch)
  //     via the autoselect path, so the chip displays the current branch by
  //     default and the submit payload always carries an explicit value
  //   - if user keeps the default, backend's "branch == current → skip" logic
  //     runs (no git ops)
  //   - if user picks a different existing branch, backend runs `git checkout`
  //   - "Fork a new branch" toggle is a separate flow that creates a NEW branch
  //     from the selected base
  // Other executors: branch is fully editable (no special pre-fill).
  const branchLocked = false;
  // No early returns above hooks. URL mode and started-state checks happen below.
  const usedIds = useMemo(() => collectUsedRepoIds(fs.repositories), [fs.repositories]);
  if (isTaskStarted) return null;

  const remainingCount = repositories.filter((r) => !usedIds.has(r.id)).length;
  // Add stays enabled when no workspace repos are left, since users can also
  // pick from discovered on-machine paths.
  const hasDiscovered = fs.discoveredRepositories.length > 0;
  const canAddMore = remainingCount > 0 || hasDiscovered;
  const addHint = computeAddHint(canAddMore, repositories.length);

  return (
    <div className="flex flex-wrap items-center gap-2" data-testid="repo-chips-row">
      {fs.useGitHubUrl ? (
        <GitHubUrlSection
          githubUrl={fs.githubUrl}
          githubUrlError={fs.githubUrlError}
          githubBranch={fs.githubBranch}
          githubBranches={fs.githubBranches}
          githubBranchesLoading={fs.githubBranchesLoading}
          onGitHubUrlChange={onGitHubUrlChange}
          onGitHubBranchChange={fs.setGitHubBranch}
        />
      ) : (
        <ChipsList
          fs={fs}
          repositories={repositories}
          workspaceId={workspaceId}
          branchLocked={branchLocked}
          isLocalExecutor={!!isLocalExecutor}
          canAddMore={canAddMore}
          addHint={addHint}
          onRowRepositoryChange={onRowRepositoryChange}
          onRowBranchChange={onRowBranchChange}
          freshBranchToggle={
            // Multi-repo runs use worktrees, so the existing-vs-fork choice
            // is irrelevant — only surface the toggle for single-repo flows.
            freshBranchAvailable && onToggleFreshBranch && fs.repositories.length === 1 ? (
              <FreshBranchToggle enabled={!!freshBranchEnabled} onToggle={onToggleFreshBranch} />
            ) : null
          }
        />
      )}
      {onToggleGitHubUrl && (
        <button
          type="button"
          onClick={onToggleGitHubUrl}
          className="ml-auto text-[11px] text-muted-foreground hover:text-foreground cursor-pointer"
          data-testid="toggle-github-url"
        >
          {fs.useGitHubUrl ? "use a workspace repository" : "or paste a GitHub URL"}
        </button>
      )}
    </div>
  );
}

/**
 * Renders the list of repo chips plus the trailing "+ add repository"
 * button. Extracted from RepoChipsRow so the parent stays under the
 * function-length cap; logic is unchanged.
 */
function ChipsList({
  fs,
  repositories,
  workspaceId,
  branchLocked,
  isLocalExecutor,
  canAddMore,
  addHint,
  freshBranchToggle,
  onRowRepositoryChange,
  onRowBranchChange,
}: {
  fs: DialogFormState;
  repositories: Repository[];
  workspaceId: string | null;
  branchLocked: boolean;
  isLocalExecutor: boolean;
  canAddMore: boolean;
  addHint?: string;
  freshBranchToggle?: React.ReactNode;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
}) {
  return (
    <>
      {fs.repositories.map((row) => (
        <RepoChip
          key={row.key}
          row={row}
          workspaceId={workspaceId}
          repositories={repositories}
          discoveredRepositories={fs.discoveredRepositories}
          excludedRepoIds={collectUsedRepoIds(fs.repositories, row.key)}
          branchLocked={branchLocked}
          // For local-executor rows, seed row.branch with the workspace's
          // current branch via this prop. Non-local rows leave it undefined
          // and fall back to the existing last-used / preferred-default
          // autoselect path.
          preferredDefaultBranch={isLocalExecutor ? fs.currentLocalBranch : undefined}
          preferredDefaultBranchLoading={isLocalExecutor ? fs.currentLocalBranchLoading : false}
          branchPrefix={computeBranchPrefix({
            isLocalExecutor,
            rowBranch: row.branch,
            currentLocalBranch: fs.currentLocalBranch,
            freshBranchEnabled: !!fs.freshBranchEnabled,
          })}
          onRepositoryChange={(value) => onRowRepositoryChange(row.key, value)}
          onBranchChange={(value) => onRowBranchChange(row.key, value)}
          onRemove={() => fs.removeRepository(row.key)}
        />
      ))}
      {freshBranchToggle}
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex">
            <button
              type="button"
              onClick={fs.addRepository}
              disabled={!canAddMore}
              aria-label="Add repository"
              data-testid="add-repository"
              className={cn(
                "h-7 w-7 inline-flex items-center justify-center rounded-md text-muted-foreground",
                canAddMore
                  ? "hover:bg-muted hover:text-foreground cursor-pointer"
                  : "opacity-40 cursor-not-allowed",
              )}
            >
              <IconPlus className="h-3.5 w-3.5" />
            </button>
          </span>
        </TooltipTrigger>
        <TooltipContent>{addHint ?? "Add another repository"}</TooltipContent>
      </Tooltip>
    </>
  );
}

function FreshBranchToggle({
  enabled,
  onToggle,
}: {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={() => onToggle(!enabled)}
          data-testid="fresh-branch-toggle"
          aria-pressed={enabled}
          aria-label={
            enabled
              ? "Fork a new branch from a base (turn off to use current checkout)"
              : "Fork a new branch from a base instead of using current checkout"
          }
          className={cn(
            "inline-flex h-7 w-7 items-center justify-center rounded-md border border-input cursor-pointer transition-colors",
            enabled
              ? "bg-muted text-foreground"
              : "bg-transparent text-muted-foreground hover:text-foreground hover:bg-muted/60",
          )}
        >
          <IconGitFork className="h-3.5 w-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        {enabled
          ? "Fork mode: a new branch will be created from the selected base before the agent runs. Click to turn off and use your repository's current checkout instead."
          : "By default the local executor uses your repository's current checkout. Click to fork a new branch from a base instead, leaving your working tree untouched."}
      </TooltipContent>
    </Tooltip>
  );
}

function computeAddHint(canAddMore: boolean, workspaceRepoCount: number): string | undefined {
  if (canAddMore) return undefined;
  if (workspaceRepoCount === 0) return "No repositories available in this workspace";
  return "All workspace repositories are already added";
}

/** Build the set of repo identifiers (workspace id or path) currently in use. */
function collectUsedRepoIds(rows: TaskRepoRow[], exceptKey?: string): Set<string> {
  const ids = new Set<string>();
  for (const r of rows) {
    if (r.key === exceptKey) continue;
    if (r.repositoryId) ids.add(r.repositoryId);
    if (r.localPath) ids.add(r.localPath);
  }
  return ids;
}

type RepoChipProps = {
  row: TaskRepoRow;
  /** Required for path-based branch loading on discovered rows. */
  workspaceId: string | null;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  /** Repo IDs/paths to filter out of the dropdown (already in use elsewhere). */
  excludedRepoIds: Set<string>;
  /**
   * Lock the branch pill regardless of branch availability. Used for the
   * local executor where the user's actual checkout dictates the branch
   * (and changing it would mutate their working tree). Fresh-branch mode
   * unlocks it because we're explicitly creating a new branch from a base.
   */
  branchLocked?: boolean;
  /**
   * When set, seed row.branch with this value (for an empty row). Used by
   * the local-executor flow to surface the workspace's current ref — either
   * a branch name like "main" or, on detached HEAD, the short commit SHA
   * returned by the backend. The chip displays it verbatim ("current: main"
   * or "current: 4fbc5d7"); on submit the backend's skip-when-equal check
   * matches the same SHA so it's a no-op.
   *
   * When unset, the chip falls back to the existing last-used / preferred-
   * default autoselect (main / master / develop, etc.).
   */
  preferredDefaultBranch?: string;
  /**
   * True while preferredDefaultBranch is being resolved. Renders a
   * "Loading branch…" placeholder so the chip doesn't briefly show an empty
   * state in the window between dialog open and local-status resolving.
   */
  preferredDefaultBranchLoading?: boolean;
  /**
   * Muted text shown before the branch value to qualify intent:
   *   - "current: "        — local exec, picked branch == workspace current
   *   - "will switch to: " — local exec, picked branch != workspace current
   *   - "from: "           — worktree / non-local exec (picked branch is the base)
   * Empty when there's no branch value yet (chip shows the "branch"
   * placeholder unprefixed).
   */
  branchPrefix?: string;
  onRepositoryChange: (value: string) => void;
  onBranchChange: (value: string) => void;
  onRemove: () => void;
};

function useRepoChipData({
  row,
  workspaceId,
  repositories,
  discoveredRepositories,
  excludedRepoIds,
  onBranchChange,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
}: Pick<
  RepoChipProps,
  | "row"
  | "workspaceId"
  | "repositories"
  | "discoveredRepositories"
  | "excludedRepoIds"
  | "onBranchChange"
  | "preferredDefaultBranch"
  | "preferredDefaultBranchLoading"
>) {
  const filteredRepos = useMemo(
    () => repositories.filter((r) => !excludedRepoIds.has(r.id) || r.id === row.repositoryId),
    [repositories, excludedRepoIds, row.repositoryId],
  );
  // Discovered (on-machine) repos not yet imported into the workspace. Drop
  // ones whose path is already a workspace repo so the dropdown doesn't show
  // the same folder twice.
  const filteredDiscovered = useMemo(() => {
    const workspaceRepoPaths = new Set(
      filteredRepos
        .map((r) => r.local_path)
        .filter(Boolean)
        .map((path: string) => normalizeRepoPath(path)),
    );
    return discoveredRepositories.filter(
      (r) =>
        !workspaceRepoPaths.has(normalizeRepoPath(r.path)) &&
        (!excludedRepoIds.has(r.path) || r.path === row.localPath),
    );
  }, [filteredRepos, discoveredRepositories, excludedRepoIds, row.localPath]);

  // One hook for both row sources (workspace repo by id OR on-machine path).
  // Backend resolves either to an absolute path; the Zustand cache keys
  // path-based entries with a synthetic key so workspace + discovered share
  // one slice (and the same fetch dedupe).
  const branchSource = useMemo<BranchSource | null>(() => {
    if (!workspaceId) return null;
    if (row.repositoryId) {
      return { kind: "id", workspaceId, repositoryId: row.repositoryId };
    }
    if (row.localPath) {
      return { kind: "path", workspaceId, path: row.localPath };
    }
    return null;
  }, [workspaceId, row.repositoryId, row.localPath]);
  const {
    branches,
    isLoading: branchesLoading,
    refresh: refreshBranches,
  } = useBranches(branchSource, !!branchSource);

  // Once branches load for this row, pre-fill the branch pill. For
  // local-executor rows we prefer the workspace's actual current branch
  // (preferredDefaultBranch) so the chip shows what's on disk and the
  // submit always carries an explicit branch. Otherwise fall back to the
  // user's last-used branch / main / master / develop. Skipped if the user
  // already picked a branch on this row.
  //
  // The `preferredDefaultBranchLoading` gate closes a race: without it,
  // autoSelectBranch's last-used localStorage pick (e.g. "origin/master")
  // would land in row.branch before currentLocalBranch resolved, then the
  // row.branch guard above would short-circuit forever and the chip would
  // render "will switch to: origin/master" even though the user just opened
  // the dialog. With the gate, autoselect waits until local-status finishes
  // so the current-branch preference gets first crack at row.branch.
  useEffect(() => {
    if (!branchSource || branchesLoading || branches.length === 0 || row.branch) return;
    runAutoselect({
      branches,
      preferredDefaultBranch,
      preferredDefaultBranchLoading: !!preferredDefaultBranchLoading,
      onBranchChange,
    });
  }, [
    branchSource,
    branchesLoading,
    branches,
    row.branch,
    onBranchChange,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
  ]);

  const repoOptions: PillOption[] = useMemo(
    () => [
      ...filteredRepos.map((r) => ({ value: r.id, label: r.name })),
      ...filteredDiscovered.map((r) => ({
        value: r.path,
        label: shortRepoPath(r.path),
        keywords: [r.path],
      })),
    ],
    [filteredRepos, filteredDiscovered],
  );
  const branchOptions: PillOption[] = useMemo(
    () => sortBranches(branches).map(branchToOption),
    [branches],
  );
  return { repoOptions, branchOptions, branchesLoading, refreshBranches };
}

function computeRepoChipDisplay(
  row: TaskRepoRow,
  repositories: Repository[],
  discoveredRepositories: LocalRepository[],
) {
  const workspaceRepo = repositories.find((r) => r.id === row.repositoryId);
  const discoveredRepo = discoveredRepositories.find((r) => r.path === row.localPath);
  const repoLabel = workspaceRepo?.name ?? discoveredRepo?.path?.split("/").pop() ?? "";
  const repoPath = workspaceRepo?.local_path || discoveredRepo?.path || "";
  const repoTooltip = repoPath ? `Repository · ${formatUserHomePath(repoPath)}` : "Repository";
  return { repoLabel, repoTooltip };
}

function RepoChip({
  row,
  workspaceId,
  repositories,
  discoveredRepositories,
  excludedRepoIds,
  branchLocked,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  branchPrefix,
  onRepositoryChange,
  onBranchChange,
  onRemove,
}: RepoChipProps) {
  const { repoOptions, branchOptions, branchesLoading, refreshBranches } = useRepoChipData({
    row,
    workspaceId,
    repositories,
    discoveredRepositories,
    excludedRepoIds,
    onBranchChange,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
  });
  const { repoLabel, repoTooltip } = computeRepoChipDisplay(
    row,
    repositories,
    discoveredRepositories,
  );
  // Hide the value while the preferred-default-branch fetch is in flight to
  // avoid the chip briefly rendering an empty placeholder before autoselect
  // seeds row.branch with the workspace's current branch.
  const branchValue = preferredDefaultBranchLoading ? "" : row.branch;
  const hasRepo = !!(row.repositoryId || row.localPath);
  const branchPlaceholder = computeBranchPlaceholder(
    hasRepo,
    branchesLoading || !!preferredDefaultBranchLoading,
    branchOptions.length,
  );

  return (
    <span
      className="inline-flex items-center rounded-md border border-input bg-input/20 dark:bg-input/30 pr-0.5"
      data-testid="repo-chip"
      data-repository-id={row.repositoryId || row.localPath || ""}
    >
      <Pill
        icon={<IconCode className="h-3 w-3 shrink-0 text-muted-foreground" />}
        value={repoLabel}
        placeholder="repository"
        options={repoOptions}
        onSelect={onRepositoryChange}
        searchPlaceholder="Search repositories..."
        emptyMessage="No repositories"
        testId="repo-chip-trigger"
        tooltip={repoTooltip}
        flat
      />
      <Pill
        icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
        value={branchValue}
        placeholder={branchPlaceholder}
        prefix={branchPrefix}
        options={branchOptions}
        onSelect={onBranchChange}
        disabled={branchLocked || !hasRepo || branchesLoading || branchOptions.length === 0}
        disabledReason={computeBranchDisabledReason({
          branchLocked: !!branchLocked,
          hasRepo,
          branchesLoading,
          optionCount: branchOptions.length,
        })}
        searchPlaceholder="Search branches..."
        emptyMessage="No branches"
        testId="branch-chip-trigger"
        tooltip={computeBranchTooltip(branchPrefix)}
        onRefresh={refreshBranches}
        refreshing={branchesLoading}
        filter={scoreBranch}
        flat
      />
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={onRemove}
            aria-label="Remove repository"
            className="h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer"
            data-testid="remove-repo-chip"
          >
            <IconX className="h-3 w-3" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Remove repository</TooltipContent>
      </Tooltip>
    </span>
  );
}

function normalizeRepoPath(path: string): string {
  return path.replace(/\\/g, "/").replace(/\/+$/g, "");
}

function shortRepoPath(path: string): string {
  // Show the trailing folder name with one parent for context (e.g. "myorg/myrepo").
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean);
  if (parts.length <= 1) return path;
  return parts.slice(-2).join("/");
}
