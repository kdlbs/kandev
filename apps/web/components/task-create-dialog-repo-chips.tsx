"use client";

import { useMemo } from "react";
import { IconPlus, IconX, IconCode, IconGitBranch, IconGitFork } from "@tabler/icons-react";
import { cn, formatUserHomePath } from "@/lib/utils";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import type { LocalRepository, Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { scoreBranch } from "@/lib/utils/branch-filter";
import { scoreRepo } from "@/lib/utils/repo-filter";
import {
  Pill,
  sortBranches,
  branchToOption,
  computeBranchPlaceholder,
  type PillOption,
} from "@/components/task-create-dialog-pill";
import { RemoteRepoChipsRow } from "@/components/task-create-dialog-remote-repo-chips";
import { FolderPicker } from "@/components/folder-picker";
import { SourceModeSwitch } from "@/components/task-create-dialog-source-mode";
import {
  computeBranchPrefix,
  computeBranchTooltip,
  computeBranchDisabledReason,
} from "@/components/task-create-dialog-branch-utils";
import { useRepoBranchAutoselect } from "@/components/task-create-dialog-repo-branch-autoselect";

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
  /** Toggles the Remote tab on/off. Remote-mode rows live in `fs.remoteRepos`. */
  onToggleRemote?: () => void;
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
  /** "No repository" mode: replace the chip row with a folder picker. */
  onToggleNoRepository?: () => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
};

export function RepoChipsRow({
  fs,
  repositories,
  isTaskStarted,
  workspaceId,
  onRowRepositoryChange,
  onRowBranchChange,
  onToggleRemote,
  freshBranchAvailable,
  freshBranchEnabled,
  onToggleFreshBranch,
  isLocalExecutor,
  onToggleNoRepository,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
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
  if (isTaskStarted) return null;

  // Multi-branch support: the same repo can appear multiple times on a task
  // when each row picks a different branch. Uniqueness is enforced on the
  // (repository_id, checkout_branch) pair at submit time by the backend, so
  // the dropdown never filters repos out — picking "frontend" twice and
  // assigning two different branches is a supported flow.
  const hasDiscovered = fs.discoveredRepositories.length > 0;
  const canAddMore = repositories.length > 0 || hasDiscovered;
  const addHint = computeAddHint(canAddMore, repositories.length);

  return (
    // min-h-9 reserves enough vertical space for the tallest mode body so the
    // modal doesn't jump when the user toggles between Repo / URL / None
    // (None renders a single pill, Repo can render chips + branch + add and
    // sometimes wraps when the segmented control crowds the row).
    <div className="flex min-h-9 flex-wrap items-center gap-2" data-testid="repo-chips-row">
      <ModeBody
        fs={fs}
        repositories={repositories}
        workspaceId={workspaceId}
        branchLocked={branchLocked}
        isLocalExecutor={!!isLocalExecutor}
        canAddMore={canAddMore}
        addHint={addHint}
        freshBranchAvailable={freshBranchAvailable}
        freshBranchEnabled={freshBranchEnabled}
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={onRowBranchChange}
        onToggleFreshBranch={onToggleFreshBranch}
        onWorkspacePathChange={onWorkspacePathChange}
        lastUsedBranch={lastUsedBranch}
        userSettingsLoaded={userSettingsLoaded}
      />
      <SourceModeSwitch
        useRemote={fs.useRemote}
        noRepository={fs.noRepository}
        onToggleRemote={onToggleRemote}
        onToggleNoRepository={onToggleNoRepository}
      />
    </div>
  );
}

function ModeBody({
  fs,
  repositories,
  workspaceId,
  branchLocked,
  isLocalExecutor,
  canAddMore,
  addHint,
  freshBranchAvailable,
  freshBranchEnabled,
  onRowRepositoryChange,
  onRowBranchChange,
  onToggleFreshBranch,
  onWorkspacePathChange,
  lastUsedBranch,
  userSettingsLoaded,
}: {
  fs: DialogFormState;
  repositories: Repository[];
  workspaceId: string | null;
  branchLocked: boolean;
  isLocalExecutor: boolean;
  canAddMore: boolean;
  addHint: string | undefined;
  freshBranchAvailable?: boolean;
  freshBranchEnabled?: boolean;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onToggleFreshBranch?: (enabled: boolean) => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
}) {
  if (fs.noRepository) {
    return (
      <FolderPicker
        value={fs.workspacePath}
        onChange={onWorkspacePathChange ?? (() => {})}
        placeholder="pick a starting folder (optional)"
      />
    );
  }
  if (fs.useRemote) {
    return (
      <RemoteRepoChipsRow
        fs={fs}
        onUpdateRow={fs.updateRemoteRepo}
        onAddRow={fs.addRemoteRepo}
        onRemoveRow={fs.removeRemoteRepo}
      />
    );
  }
  return (
    <ChipsList
      fs={fs}
      repositories={repositories}
      workspaceId={workspaceId}
      branchLocked={branchLocked}
      isLocalExecutor={isLocalExecutor}
      canAddMore={canAddMore}
      addHint={addHint}
      onRowRepositoryChange={onRowRepositoryChange}
      onRowBranchChange={onRowBranchChange}
      lastUsedBranch={lastUsedBranch}
      userSettingsLoaded={userSettingsLoaded}
      freshBranchToggle={
        // Multi-repo runs use worktrees, so the existing-vs-fork choice
        // is irrelevant — only surface the toggle for single-repo flows.
        freshBranchAvailable && onToggleFreshBranch && fs.repositories.length === 1 ? (
          <FreshBranchToggle enabled={!!freshBranchEnabled} onToggle={onToggleFreshBranch} />
        ) : null
      }
    />
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
  lastUsedBranch,
  userSettingsLoaded,
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
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
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
          // Multi-branch: the same repository may be reused across rows when
          // each row picks a different branch. Only exclude rows that hold
          // the exact (repo, branch) pair this row would clash with on the
          // backend — empty-branch rows can't collide yet, so they pass.
          excludedRepoIds={collectExactDuplicateRepoIds(fs.repositories, row)}
          branchLocked={branchLocked}
          // For local-executor rows, seed row.branch with the workspace's
          // current branch via this prop. Non-local rows leave it undefined
          // and fall back to the existing last-used / preferred-default
          // autoselect path.
          preferredDefaultBranch={isLocalExecutor ? fs.currentLocalBranch : undefined}
          preferredDefaultBranchLoading={isLocalExecutor ? fs.currentLocalBranchLoading : false}
          lastUsedBranch={lastUsedBranch}
          userSettingsLoaded={userSettingsLoaded}
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

/**
 * Returns the set of repo ids/paths that would create a literal duplicate
 * (same repo + same branch) of an *existing* row if `currentRow` adopted
 * them — used to hide already-claimed pairings from the repo dropdown.
 *
 * Multi-branch tasks are supported: the same repo can appear across multiple
 * rows as long as each row's branch differs. Rows with empty branches don't
 * collide yet, so they don't contribute to the exclusion set.
 *
 * Same-row entries are skipped so the current row's own pick remains
 * selectable; without that, after the user pairs (repo, branch) the chip
 * would suddenly render its current repo as unavailable.
 */
function collectExactDuplicateRepoIds(rows: TaskRepoRow[], currentRow: TaskRepoRow): Set<string> {
  const ids = new Set<string>();
  for (const r of rows) {
    if (r.key === currentRow.key) continue;
    if (!r.branch || r.branch !== currentRow.branch) continue;
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
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
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
  lastUsedBranch,
  userSettingsLoaded,
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
  | "lastUsedBranch"
  | "userSettingsLoaded"
>) {
  const filteredRepos = useMemo(
    () => repositories.filter((r) => !excludedRepoIds.has(r.id) || r.id === row.repositoryId),
    [repositories, excludedRepoIds, row.repositoryId],
  );
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
  useRepoBranchAutoselect({
    branchSource,
    branchesLoading,
    branches,
    rowBranch: row.branch,
    onBranchChange,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
  });

  const repoOptions: PillOption[] = useMemo(
    () => [
      ...filteredRepos.map((r) => ({
        value: r.id,
        label: r.name,
        keywords: [r.name, r.local_path, formatUserHomePath(r.local_path)].filter(
          (s): s is string => !!s,
        ),
        renderLabel: () => renderWorkspaceRepoOption(r),
      })),
      ...filteredDiscovered.map((r) => ({
        value: r.path,
        label: leafSegment(r.path),
        keywords: [r.path, formatUserHomePath(r.path)],
        renderLabel: () => renderDiscoveredRepoOption(r.path),
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
  lastUsedBranch,
  userSettingsLoaded,
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
    lastUsedBranch,
    userSettingsLoaded,
  });
  const { repoLabel, repoTooltip } = computeRepoChipDisplay(
    row,
    repositories,
    discoveredRepositories,
  );
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
        filter={scoreRepo}
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

function renderWorkspaceRepoOption(repo: Repository) {
  const display = repo.local_path ? formatUserHomePath(repo.local_path) : "";
  return (
    <span className="flex min-w-0 flex-1 flex-col overflow-hidden" title={display || repo.name}>
      <span className="truncate">{repo.name}</span>
      {display ? (
        <span className="truncate text-[11px] text-muted-foreground">{display}</span>
      ) : null}
    </span>
  );
}

function renderDiscoveredRepoOption(path: string) {
  const display = formatUserHomePath(path);
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden" title={display}>
      <span className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <span className="truncate">{leafSegment(path)}</span>
        <span className="truncate text-[11px] text-muted-foreground">{display}</span>
      </span>
      <Badge variant="outline" className="text-[10px] text-muted-foreground shrink-0">
        on disk
      </Badge>
    </span>
  );
}

function leafSegment(path: string): string {
  const cleaned = path.replace(/\\/g, "/").replace(/\/+$/g, "");
  const idx = cleaned.lastIndexOf("/");
  return idx >= 0 ? cleaned.slice(idx + 1) : cleaned;
}
