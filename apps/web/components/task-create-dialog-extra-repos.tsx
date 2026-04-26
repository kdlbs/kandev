"use client";

import { useMemo, useCallback } from "react";
import { IconPlus, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { RepositorySelector, BranchSelector } from "@/components/task-create-dialog-selectors";
import { useRepositoryBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useBranchOptions, useRepositoryOptions } from "@/components/task-create-dialog-options";
import type { Repository } from "@/lib/types/http";
import type {
  ExtraRepositoryRow,
  DialogFormState,
} from "@/components/task-create-dialog-types";

type ExtraRepositoryRowsProps = {
  fs: DialogFormState;
  repositories: Repository[];
  isTaskStarted: boolean;
  /**
   * Disable the Add button when no primary repository is selected — extra
   * rows only make sense when the task has at least one repo defined.
   */
  primarySelected: boolean;
};

/**
 * Renders the "+ Add repository" control plus one editable row per extra
 * repo on the task. The primary repo is rendered elsewhere (header +
 * CreateEditSelectors); this component only handles the 2nd, 3rd, …
 *
 * Each row is a pair of (repository combobox, base-branch combobox).
 * Branches for the chosen repo are fetched on demand via the existing
 * useRepositoryBranches hook so the user gets the right list per repo.
 */
export function ExtraRepositoryRows({
  fs,
  repositories,
  isTaskStarted,
  primarySelected,
}: ExtraRepositoryRowsProps) {
  // Hide the section in started/edit modes where the repo set is fixed.
  if (isTaskStarted) return null;

  // Count workspace repositories the user could still add: total minus primary
  // minus all already-chosen extras. When zero, we disable the Add button so
  // the user gets a clear signal instead of a row with an empty dropdown.
  const usedIds = collectUsedRepoIds(fs, "");
  const remainingCount = repositories.filter((r) => !usedIds.has(r.id)).length;
  const canAddMore = primarySelected && remainingCount > 0;
  const addButtonHint = computeAddButtonHint(primarySelected, remainingCount);

  return (
    <div className="space-y-2" data-testid="extra-repositories">
      {fs.extraRepositories.map((row) => (
        <ExtraRepositoryRowItem
          key={row.key}
          row={row}
          repositories={repositories}
          // Disallow picking the primary repo or any other extra row's repo.
          excludedRepoIds={collectUsedRepoIds(fs, row.key)}
          onRepositoryChange={(value) => fs.updateExtraRepository(row.key, { repositoryId: value })}
          onBranchChange={(value) => fs.updateExtraRepository(row.key, { branch: value })}
          onRemove={() => fs.removeExtraRepository(row.key)}
        />
      ))}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={fs.addExtraRepository}
        disabled={!canAddMore}
        title={addButtonHint}
        className="text-xs cursor-pointer text-muted-foreground hover:text-foreground"
        data-testid="add-extra-repository"
      >
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        Add repository
      </Button>
    </div>
  );
}

function computeAddButtonHint(
  primarySelected: boolean,
  remainingCount: number,
): string | undefined {
  if (!primarySelected) return "Select the primary repository first";
  if (remainingCount === 0) return "All workspace repositories are already added";
  return undefined;
}

function computeBranchPlaceholder(
  repoId: string,
  branchesLoading: boolean,
  branchCount: number,
): string {
  if (!repoId) return "Select repository first";
  if (branchesLoading) return "Loading branches...";
  if (branchCount === 0) return "No branches found";
  return "Select branch";
}

function collectUsedRepoIds(fs: DialogFormState, exceptKey: string): Set<string> {
  const ids = new Set<string>();
  if (fs.repositoryId) ids.add(fs.repositoryId);
  for (const r of fs.extraRepositories) {
    if (r.key !== exceptKey && r.repositoryId) ids.add(r.repositoryId);
  }
  return ids;
}

type ExtraRepositoryRowItemProps = {
  row: ExtraRepositoryRow;
  repositories: Repository[];
  excludedRepoIds: Set<string>;
  onRepositoryChange: (value: string) => void;
  onBranchChange: (value: string) => void;
  onRemove: () => void;
};

function ExtraRepositoryRowItem({
  row,
  repositories,
  excludedRepoIds,
  onRepositoryChange,
  onBranchChange,
  onRemove,
}: ExtraRepositoryRowItemProps) {
  // Filter the repo list to drop already-used repos (prevents duplicates
  // before the user even sees them in the dropdown).
  const filteredRepos = useMemo(
    () => repositories.filter((r) => !excludedRepoIds.has(r.id)),
    [repositories, excludedRepoIds],
  );
  const { headerRepositoryOptions } = useRepositoryOptions(filteredRepos, []);
  const { branches, isLoading: branchesLoading } = useRepositoryBranches(
    row.repositoryId || null,
    Boolean(row.repositoryId),
  );
  const branchOptions = useBranchOptions(branches);

  const branchPlaceholder = computeBranchPlaceholder(
    row.repositoryId,
    branchesLoading,
    branchOptions.length,
  );

  const handleRepoChange = useCallback(
    (value: string) => {
      onRepositoryChange(value);
      // Reset branch when the repo changes — the previous branch may not exist
      // on the new repo, and stale text would mislead the user.
      onBranchChange("");
    },
    [onRepositoryChange, onBranchChange],
  );

  return (
    <div
      className="flex items-center gap-2"
      data-testid="extra-repository-row"
      data-row-key={row.key}
    >
      <div className="flex-1 min-w-0">
        <RepositorySelector
          options={headerRepositoryOptions}
          value={row.repositoryId}
          onValueChange={handleRepoChange}
          disabled={false}
          placeholder="Select repository"
          searchPlaceholder="Search repositories..."
          emptyMessage={
            filteredRepos.length === 0 ? "No more repositories available." : "No repositories found."
          }
          triggerClassName="w-full text-sm"
        />
      </div>
      <div className="flex-1 min-w-0">
        <BranchSelector
          options={branchOptions}
          value={row.branch}
          onValueChange={onBranchChange}
          disabled={!row.repositoryId || branchesLoading || branchOptions.length === 0}
          placeholder={branchPlaceholder}
          searchPlaceholder="Search branches..."
          emptyMessage="No branch found."
        />
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-7 w-7 shrink-0 cursor-pointer text-muted-foreground hover:text-destructive"
        onClick={onRemove}
        aria-label="Remove repository"
        data-testid="remove-extra-repository"
      >
        <IconX className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
