import { Badge } from "@kandev/ui/badge";

import type { Branch, LocalRepository, Repository } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { branchToOption, sortBranches } from "@/components/task-create-dialog-branch-options";
import { type PillOption } from "@/components/task-create-dialog-pill";
import { t } from "@/lib/i18n";

export function buildRepoBaseBranchData({
  branches,
  branchesLoaded,
  savedBaseBranch,
  row,
  repositories,
  discoveredRepositories,
}: {
  branches: Branch[];
  branchesLoaded: boolean;
  savedBaseBranch?: string;
  row: TaskRepoRow;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
}): { baseBranchOptions: PillOption[]; defaultBranch: string } {
  const workspaceRepo = repositories.find((repository) => repository.id === row.repositoryId);
  const discoveredRepo = discoveredRepositories.find(
    (repository) => repository.path === row.localPath,
  );
  const defaultBranch = workspaceRepo?.default_branch ?? discoveredRepo?.default_branch ?? "";
  const available = sortBranches(branches).map(branchToOption);
  const savedBaseOption = savedBaseBranch
    ? savedBaseBranchOption(savedBaseBranch, branchesLoaded, available)
    : null;
  return {
    defaultBranch,
    baseBranchOptions: [
      ...(savedBaseOption ? [savedBaseOption] : []),
      taskDefaultBranchOption(defaultBranch),
      ...available,
    ],
  };
}

function savedBaseBranchOption(
  branch: string,
  branchesLoaded: boolean,
  available: PillOption[],
): PillOption | null {
  if (available.some((option) => option.value === branch)) return null;
  return branchesLoaded ? unavailableBranchToOption(branch) : savedBranchToOption(branch);
}

export function unavailableBranchToOption(branch: string): PillOption {
  return {
    value: branch,
    label: branch,
    keywords: [branch],
    group: "branches",
    groupLabel: t("task:branchesGroup"),
    disabled: true,
    disabledReason: t("task:branchUnavailable"),
    renderLabel: () => (
      <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
        <span className="truncate" title={branch}>
          {branch}
        </span>
        <Badge variant="outline" className="shrink-0 text-xs">
          {t("task:branchUnavailableShort")}
        </Badge>
      </span>
    ),
  };
}

function savedBranchToOption(branch: string): PillOption {
  return {
    value: branch,
    label: branch,
    keywords: [branch],
    group: "branches",
    groupLabel: t("task:branchesGroup"),
  };
}

function taskDefaultBranchOption(defaultBranch: string): PillOption {
  const branchLabel = defaultBranch || t("common:repositoryDefaultBranchOption");
  const label = t("workspaces:repositorySetsTaskDefault", { branch: branchLabel });
  return {
    value: "",
    label,
    keywords: [label, branchLabel],
    group: "branches",
    groupLabel: t("task:branchesGroup"),
  };
}
