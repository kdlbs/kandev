"use client";

import { useMemo } from "react";
import { IconX } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import type { LocalRepository, Repository, RepositoryBranchPolicy } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { type PillOption } from "@/components/task-create-dialog-pill";
import { branchToOption, sortBranches } from "@/components/task-create-dialog-branch-options";
import { buildRepoBaseBranchData } from "@/components/task-create-dialog-repo-base-branch";
import {
  buildRepoOptions,
  computeRepoChipDisplay,
  normalizeRepoPath,
} from "@/components/task-create-dialog-repo-chip-utils";
import {
  computeBranchIntent,
  type BranchIntent,
} from "@/components/task-create-dialog-branch-utils";
import { useRepoBranchAutoselect } from "@/components/task-create-dialog-repo-branch-autoselect";
import { useRepositoryBranchPolicies } from "@/hooks/domains/workspace/use-repository-branch-policies";
import {
  RepoChipBaseBranchPill,
  RepoChipBranchPill,
  RepoChipRepositoryPill,
  useRepoChipBranchPicker,
} from "@/components/task-create-dialog-repo-chip-parts";
import { AddRepositoryButton } from "@/components/task-create-dialog-add-repository-button";
import { useTranslation } from "react-i18next";

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
  branchPolicyDisabledReason?: string;
  showBranchPolicies?: boolean;
  canAddMore: boolean;
  addHint?: string;
  addLabel?: string;
  allowDuplicateRepositories?: boolean;
  freshBranchToggle?: React.ReactNode;
  onAdd: () => void;
  onRemove: (key: string) => void;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onRowBaseBranchChange?: (key: string, value: string) => void;
  onRowPolicyChange?: (key: string, policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
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
  branchPolicyDisabledReason,
  showBranchPolicies = false,
  canAddMore,
  addHint,
  addLabel,
  allowDuplicateRepositories = true,
  freshBranchToggle,
  onAdd,
  onRemove,
  onRowRepositoryChange,
  onRowBranchChange,
  onRowBaseBranchChange,
  onRowPolicyChange,
  onPolicySelected,
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
          // Task creation marks other rows' picks but keeps every option
          // selectable; quick chat excludes a repository once another row uses it.
          excludedRepoIds={collectExcludedRepoIds(rows, row, allowDuplicateRepositories)}
          selectedElsewhere={collectSelectedRepoIdentities(rows, row)}
          branchLocked={branchLocked}
          // For local-executor rows, seed row.branch with the workspace's
          // current branch via this prop. Non-local rows leave it undefined
          // and fall back to the existing last-used / preferred-default
          // autoselect path.
          preferredDefaultBranch={isLocalExecutor ? currentLocalBranch : undefined}
          preferredDefaultBranchLoading={isLocalExecutor ? currentLocalBranchLoading : false}
          lastUsedBranch={lastUsedBranch}
          userSettingsLoaded={userSettingsLoaded}
          isLocalExecutor={!!isLocalExecutor}
          branchValue={isLocalExecutor ? row.branch : row.baseBranch || row.branch}
          savedBaseBranch={row.baseBranch}
          branchIntent={computeBranchIntent({
            isLocalExecutor: !!isLocalExecutor,
            rowBranch: isLocalExecutor ? row.branch : row.baseBranch || row.branch,
            currentLocalBranch: currentLocalBranch ?? "",
            freshBranchEnabled: !!freshBranchEnabled,
          })}
          branchPolicyDisabledReason={branchPolicyDisabledReason}
          onRepositoryChange={(value) => onRowRepositoryChange(row.key, value)}
          onBranchChange={(value) => onRowBranchChange(row.key, value)}
          onBaseBranchChange={(value) => onRowBaseBranchChange?.(row.key, value)}
          onPolicyChange={
            onRowPolicyChange
              ? (policyId, baseBranch) => onRowPolicyChange(row.key, policyId, baseBranch)
              : undefined
          }
          onPolicySelected={onPolicySelected}
          showBranchPolicies={showBranchPolicies}
          onCreateRepository={
            rows.length === 1 && onCreateRepository ? () => onCreateRepository(row.key) : undefined
          }
          onRefreshRepositories={rows.length === 1 ? onRefreshRepositories : undefined}
          repositoriesRefreshing={repositoriesRefreshing}
          onRemove={() => onRemove(row.key)}
        />
      ))}
      {freshBranchToggle}
      <AddRepositoryButton
        canAddMore={canAddMore}
        addHint={addHint}
        addLabel={addLabel}
        onAdd={onAdd}
      />
    </>
  );
}

/**
 * Returns the repo ids/paths that should be hidden from `currentRow` based on
 * the caller's repository-duplication mode.
 *
 * When duplicates are allowed, task creation keeps every repository available
 * so a user can intentionally pick the same repository for another branch.
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
  if (allowDuplicateRepositories) return new Set();

  const ids = new Set<string>();
  for (const r of rows) {
    if (r.key === currentRow.key) continue;
    if (r.repositoryId) ids.add(r.repositoryId);
    if (r.localPath) ids.add(r.localPath);
  }
  return ids;
}

function collectSelectedRepoIdentities(rows: TaskRepoRow[], currentRow: TaskRepoRow): Set<string> {
  const identities = new Set<string>();
  for (const row of rows) {
    if (row.key === currentRow.key) continue;
    if (row.repositoryId) identities.add(repoIdIdentity(row.repositoryId));
    if (row.localPath) identities.add(repoPathIdentity(row.localPath));
  }
  return identities;
}

function repoIdIdentity(id: string): string {
  return `id:${id}`;
}

function repoPathIdentity(path: string): string {
  return `path:${normalizeRepoPath(path)}`;
}

type RepoChipProps = {
  row: TaskRepoRow;
  /** Required for path-based branch loading on discovered rows. */
  workspaceId: string | null;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  /** Repo IDs/paths to filter out of the dropdown (already in use elsewhere). */
  excludedRepoIds: Set<string>;
  /** Repository identities selected in another row, rendered as a marker. */
  selectedElsewhere: Set<string>;
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
  isLocalExecutor?: boolean;
  branchValue?: string;
  savedBaseBranch?: string;
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
  branchIntent?: BranchIntent;
  onRepositoryChange: (value: string) => void;
  onBranchChange: (value: string) => void;
  onBaseBranchChange?: (value: string) => void;
  onPolicyChange?: (policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
  branchPolicyDisabledReason?: string;
  showBranchPolicies?: boolean;
  onRemove: () => void;
  onCreateRepository?: () => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
};

function useRepoChipBranchData({
  row,
  workspaceId,
  onBranchChange,
  branchValue,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  lastUsedBranch,
  userSettingsLoaded,
}: Pick<
  RepoChipProps,
  | "row"
  | "workspaceId"
  | "onBranchChange"
  | "branchValue"
  | "preferredDefaultBranch"
  | "preferredDefaultBranchLoading"
  | "lastUsedBranch"
  | "userSettingsLoaded"
  | "savedBaseBranch"
  | "isLocalExecutor"
>) {
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
    isLoaded: branchesLoaded,
  } = useBranches(branchSource, !!branchSource);
  useRepoBranchAutoselect({
    branchSource,
    branchesLoading,
    branches,
    rowBranch: branchValue,
    onBranchChange,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
  });
  return { branches, branchesLoading, branchesLoaded, refreshBranches };
}

function useRepoChipData({
  row,
  workspaceId,
  repositories,
  discoveredRepositories,
  excludedRepoIds,
  selectedElsewhere,
  onBranchChange,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  lastUsedBranch,
  userSettingsLoaded,
  branchValue,
  savedBaseBranch,
  isLocalExecutor,
}: Pick<
  RepoChipProps,
  | "row"
  | "workspaceId"
  | "repositories"
  | "discoveredRepositories"
  | "excludedRepoIds"
  | "selectedElsewhere"
  | "onBranchChange"
  | "preferredDefaultBranch"
  | "preferredDefaultBranchLoading"
  | "lastUsedBranch"
  | "userSettingsLoaded"
  | "branchValue"
  | "savedBaseBranch"
  | "isLocalExecutor"
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
  const { branches, branchesLoading, branchesLoaded, refreshBranches } = useRepoChipBranchData({
    row,
    workspaceId,
    onBranchChange,
    branchValue,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
  });

  const repoOptions: PillOption[] = useMemo(
    () => buildRepoOptions(filteredRepos, filteredDiscovered, selectedElsewhere),
    [filteredRepos, filteredDiscovered, selectedElsewhere],
  );
  const { baseBranchOptions, defaultBranch } = useMemo(
    () =>
      buildRepoBaseBranchData({
        branches,
        branchesLoaded,
        savedBaseBranch,
        row,
        repositories,
        discoveredRepositories,
      }),
    [branches, branchesLoaded, discoveredRepositories, repositories, row, savedBaseBranch],
  );
  const branchOptions: PillOption[] = useMemo(() => {
    if (!isLocalExecutor && savedBaseBranch) return baseBranchOptions;
    return sortBranches(branches).map(branchToOption);
  }, [baseBranchOptions, branches, isLocalExecutor, savedBaseBranch]);
  return {
    repoOptions,
    branchOptions,
    baseBranchOptions,
    defaultBranch,
    branches,
    branchesLoading,
    refreshBranches,
  };
}

type RepoChipData = ReturnType<typeof useRepoChipData>;

function RepoChip(props: RepoChipProps) {
  const {
    row,
    workspaceId,
    repositories,
    discoveredRepositories,
    excludedRepoIds,
    selectedElsewhere,
    onBranchChange,
    showBranchPolicies,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
    isLocalExecutor,
    branchValue,
    savedBaseBranch,
  } = props;
  const data = useRepoChipData({
    row,
    workspaceId,
    repositories,
    discoveredRepositories,
    excludedRepoIds,
    selectedElsewhere,
    onBranchChange,
    isLocalExecutor,
    branchValue: branchValue ?? row.branch,
    savedBaseBranch,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
  });
  if (showBranchPolicies) {
    return <RepoChipWithPolicies {...props} data={data} />;
  }
  return <RepoChipContent {...props} data={data} branchPolicies={[]} />;
}

function RepoChipWithPolicies({ data, ...props }: RepoChipProps & { data: RepoChipData }) {
  const { row } = props;
  const { policies: branchPolicies } = useRepositoryBranchPolicies(
    row.repositoryId ?? null,
    !!row.repositoryId,
  );
  return <RepoChipContent {...props} data={data} branchPolicies={branchPolicies} />;
}

function RepoChipContent({
  data,
  branchPolicies,
  row,
  repositories,
  discoveredRepositories,
  branchLocked,
  branchValue,
  isLocalExecutor,
  savedBaseBranch,
  preferredDefaultBranchLoading,
  branchIntent,
  onRepositoryChange,
  onBranchChange,
  onBaseBranchChange,
  onPolicyChange,
  onPolicySelected,
  branchPolicyDisabledReason,
  onRemove,
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
}: RepoChipProps & { data: RepoChipData; branchPolicies: RepositoryBranchPolicy[] }) {
  const {
    repoOptions,
    branchOptions,
    baseBranchOptions,
    defaultBranch,
    branches,
    branchesLoading,
    refreshBranches,
  } = data;
  const { repoLabel, repoTooltip } = computeRepoChipDisplay(
    row,
    repositories,
    discoveredRepositories,
  );
  const branchPicker = useRepoChipBranchPicker({
    row,
    branchPolicies,
    branches,
    branchOptions,
    branchesLoading,
    branchValue: branchValue ?? row.branch,
    preferredDefaultBranchLoading,
    policyDisabledReason: branchPolicyDisabledReason,
    onBranchChange,
    onPolicyChange,
    onPolicySelected,
  });
  return (
    <span
      className="inline-flex items-center rounded-md border border-input bg-input/20 dark:bg-input/30 pr-0.5"
      data-testid="repo-chip"
      data-repository-id={row.repositoryId || row.localPath || ""}
      data-repo-row-key={row.key}
    >
      <RepoChipRepositoryPill
        repoLabel={repoLabel}
        repoTooltip={repoTooltip}
        repositoryValue={row.repositoryId || row.localPath || ""}
        repoOptions={repoOptions}
        onRepositoryChange={onRepositoryChange}
        onCreateRepository={onCreateRepository}
        onRefreshRepositories={onRefreshRepositories}
        repositoriesRefreshing={repositoriesRefreshing}
      />
      <RepoChipBranchPill
        branchPicker={branchPicker}
        branchIntent={branchIntent}
        branchLocked={branchLocked}
        branchesLoading={branchesLoading}
        refreshBranches={refreshBranches}
      />
      {isLocalExecutor && savedBaseBranch ? (
        <RepoChipBaseBranchPill
          options={baseBranchOptions}
          value={savedBaseBranch}
          defaultBranch={defaultBranch}
          hasRepo={!!(row.repositoryId || row.localPath)}
          branchesLoading={branchesLoading}
          onSelect={onBaseBranchChange ?? (() => undefined)}
          refreshBranches={refreshBranches}
        />
      ) : null}
      <RepoChipRemoveButton onRemove={onRemove} />
    </span>
  );
}

function RepoChipRemoveButton({ onRemove }: { onRemove: () => void }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onRemove}
          aria-label={t("task:removeRepository")}
          className="h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer"
          data-testid="remove-repo-chip"
        >
          <IconX className="h-3 w-3" />
        </button>
      </TooltipTrigger>
      <TooltipContent>{t("task:removeRepository")}</TooltipContent>
    </Tooltip>
  );
}
