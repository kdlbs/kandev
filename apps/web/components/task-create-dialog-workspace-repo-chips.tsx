"use client";

import { useMemo } from "react";
import { IconPlus, IconX, IconCode, IconGitBranch, IconFolderPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import type { LocalRepository, Repository } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { cn, formatUserHomePath } from "@/lib/utils";
import { scoreBranch } from "@/lib/utils/branch-filter";
import { scoreRepo } from "@/lib/utils/repo-filter";
import {
  Pill,
  sortBranches,
  branchToOption,
  computeBranchPlaceholder,
  type PillAction,
  type PillOption,
} from "@/components/task-create-dialog-pill";
import {
  computeBranchPrefix,
  computeBranchTooltip,
  computeBranchDisabledReason,
} from "@/components/task-create-dialog-branch-utils";
import { useRepoBranchAutoselect } from "@/components/task-create-dialog-repo-branch-autoselect";

type WorkspaceRepoChipsProps = {
  rows: TaskRepoRow[];
  repositories: Repository[];
  discoveredRepositories?: LocalRepository[];
  workspaceId: string | null;
  branchLocked?: boolean;
  isLocalExecutor?: boolean;
  currentLocalBranch?: string;
  currentLocalBranchLoading?: boolean;
  freshBranchEnabled?: boolean;
  canAddMore: boolean;
  addHint?: string;
  addLabel?: string;
  allowDuplicateRepositories?: boolean;
  freshBranchToggle?: React.ReactNode;
  onAdd: () => void;
  onRemove: (key: string) => void;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onCreateRepository?: (key: string) => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
};

/**
 * Renders the list of repo chips plus the trailing "+ add repository"
 * button. Extracted from RepoChipsRow so the parent stays under the
 * function-length cap; logic is unchanged.
 */
export function WorkspaceRepoChips({
  rows,
  repositories,
  discoveredRepositories,
  workspaceId,
  branchLocked,
  isLocalExecutor,
  currentLocalBranch,
  currentLocalBranchLoading,
  freshBranchEnabled,
  canAddMore,
  addHint,
  addLabel,
  allowDuplicateRepositories = true,
  freshBranchToggle,
  onAdd,
  onRemove,
  onRowRepositoryChange,
  onRowBranchChange,
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
  lastUsedBranch,
  userSettingsLoaded,
}: WorkspaceRepoChipsProps) {
  return (
    <>
      {rows.map((row) => (
        <RepoChip
          key={row.key}
          row={row}
          workspaceId={workspaceId}
          repositories={repositories}
          discoveredRepositories={discoveredRepositories ?? []}
          // Task creation allows the same repository on different branches;
          // quick chat excludes a repository as soon as another row uses it.
          excludedRepoIds={collectExcludedRepoIds(rows, row, allowDuplicateRepositories)}
          branchLocked={branchLocked}
          // For local-executor rows, seed row.branch with the workspace's
          // current branch via this prop. Non-local rows leave it undefined
          // and fall back to the existing last-used / preferred-default
          // autoselect path.
          preferredDefaultBranch={isLocalExecutor ? currentLocalBranch : undefined}
          preferredDefaultBranchLoading={isLocalExecutor ? currentLocalBranchLoading : false}
          lastUsedBranch={lastUsedBranch}
          userSettingsLoaded={userSettingsLoaded}
          branchPrefix={computeBranchPrefix({
            isLocalExecutor: !!isLocalExecutor,
            rowBranch: row.branch,
            currentLocalBranch: currentLocalBranch ?? "",
            freshBranchEnabled: !!freshBranchEnabled,
          })}
          onRepositoryChange={(value) => onRowRepositoryChange(row.key, value)}
          onBranchChange={(value) => onRowBranchChange(row.key, value)}
          onCreateRepository={
            rows.length === 1 && onCreateRepository ? () => onCreateRepository(row.key) : undefined
          }
          onRefreshRepositories={rows.length === 1 ? onRefreshRepositories : undefined}
          repositoriesRefreshing={repositoriesRefreshing}
          onRemove={() => onRemove(row.key)}
        />
      ))}
      {freshBranchToggle}
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex" tabIndex={canAddMore ? undefined : 0}>
            <button
              type="button"
              onClick={onAdd}
              disabled={!canAddMore}
              aria-label="Add repository"
              data-testid="add-repository"
              className={cn(
                "inline-flex items-center justify-center gap-1.5 rounded-md text-muted-foreground",
                addLabel ? "h-9 px-2 text-xs" : "h-7 w-7",
                canAddMore
                  ? "hover:bg-muted hover:text-foreground cursor-pointer"
                  : "opacity-40 cursor-not-allowed",
              )}
            >
              <IconPlus className="h-3.5 w-3.5" />
              {addLabel ? <span>{addLabel}</span> : null}
            </button>
          </span>
        </TooltipTrigger>
        <TooltipContent>{addHint ?? "Add another repository"}</TooltipContent>
      </Tooltip>
    </>
  );
}

/**
 * Returns the repo ids/paths that should be hidden from `currentRow` based on
 * the caller's repository-duplication mode.
 *
 * When duplicates are allowed, only an exact (repo, branch) pairing is
 * excluded, so task creation can target multiple branches from one repo.
 * When duplicates are disabled, quick chat excludes the entire repository
 * after another row selects it, regardless of branch.
 *
 * Same-row entries are skipped so the current row's own pick remains
 * selectable; without that, after the user pairs (repo, branch) the chip
 * would suddenly render its current repo as unavailable.
 */
function collectExcludedRepoIds(
  rows: TaskRepoRow[],
  currentRow: TaskRepoRow,
  allowDuplicateRepositories: boolean,
): Set<string> {
  const ids = new Set<string>();
  for (const r of rows) {
    if (r.key === currentRow.key) continue;
    if (allowDuplicateRepositories && (!r.branch || r.branch !== currentRow.branch)) continue;
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
  onCreateRepository?: () => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
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
  const repoLabel = workspaceRepo?.name ?? (discoveredRepo ? leafSegment(discoveredRepo.path) : "");
  const repoPath = workspaceRepo?.local_path || discoveredRepo?.path || "";
  const repoTooltip = repoPath ? `Repository · ${formatUserHomePath(repoPath)}` : "Repository";
  return { repoLabel, repoTooltip };
}

function buildCreateRepositoryAction(onSelect?: () => void): PillAction | undefined {
  if (!onSelect) return undefined;
  return {
    label: "Create new repository",
    icon: <IconFolderPlus className="h-3.5 w-3.5" />,
    onSelect,
  };
}

function RepoChipRemoveButton({ onRemove }: { onRemove: () => void }) {
  return (
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
  );
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
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
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
      data-repo-row-key={row.key}
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
        action={buildCreateRepositoryAction(onCreateRepository)}
        onRefresh={onRefreshRepositories}
        refreshing={repositoriesRefreshing}
        refreshLabel="repositories"
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
      <RepoChipRemoveButton onRemove={onRemove} />
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
