import { IconCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";

import type { LocalRepository, Repository } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { type PillOption } from "@/components/task-create-dialog-pill";
import { formatUserHomePath } from "@/lib/utils";
import { t } from "@/lib/i18n";

export function normalizeRepoPath(path: string): string {
  return path.replace(/\\/g, "/").replace(/\/+$/g, "");
}

export function leafSegment(path: string): string {
  const cleaned = normalizeRepoPath(path);
  const idx = cleaned.lastIndexOf("/");
  return idx >= 0 ? cleaned.slice(idx + 1) : cleaned;
}

export function computeRepoChipDisplay(
  row: TaskRepoRow,
  repositories: Repository[],
  discoveredRepositories: LocalRepository[],
) {
  const workspaceRepo = repositories.find((repository) => repository.id === row.repositoryId);
  const discoveredRepo = discoveredRepositories.find(
    (repository) => repository.path === row.localPath,
  );
  const repoLabel = workspaceRepo?.name ?? (discoveredRepo ? leafSegment(discoveredRepo.path) : "");
  const repoPath = workspaceRepo?.local_path || discoveredRepo?.path || "";
  const repoTooltip = repoPath
    ? t("task:repositoryWithPath", { path: formatUserHomePath(repoPath) })
    : t("task:repository2");
  return { repoLabel, repoTooltip };
}

export function buildRepoOptions(
  filteredRepos: Repository[],
  filteredDiscovered: LocalRepository[],
  selectedElsewhere: Set<string>,
): PillOption[] {
  return [
    ...filteredRepos.map((repository) => ({
      value: repository.id,
      label: repository.name,
      keywords: [
        repository.name,
        repository.local_path,
        formatUserHomePath(repository.local_path),
      ].filter((value): value is string => !!value),
      renderLabel: () =>
        renderWorkspaceRepoOption(
          repository,
          selectedElsewhere.has(repoIdIdentity(repository.id)) ||
            (!!repository.local_path &&
              selectedElsewhere.has(repoPathIdentity(repository.local_path))),
        ),
    })),
    ...filteredDiscovered.map((repository) => ({
      value: repository.path,
      label: leafSegment(repository.path),
      keywords: [repository.path, formatUserHomePath(repository.path)],
      renderLabel: () =>
        renderDiscoveredRepoOption(
          repository.path,
          selectedElsewhere.has(repoPathIdentity(repository.path)),
        ),
    })),
  ];
}

function repoIdIdentity(id: string): string {
  return `id:${id}`;
}

function repoPathIdentity(path: string): string {
  return `path:${normalizeRepoPath(path)}`;
}

function renderWorkspaceRepoOption(repo: Repository, alreadyAdded: boolean) {
  const display = repo.local_path ? formatUserHomePath(repo.local_path) : "";
  return (
    <span
      className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden"
      title={display || repo.name}
    >
      <span className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <span className="truncate">{repo.name}</span>
        {display ? (
          <span className="truncate text-[11px] text-muted-foreground">{display}</span>
        ) : null}
      </span>
      {alreadyAdded ? <AlreadyAddedMarker /> : null}
    </span>
  );
}

function renderDiscoveredRepoOption(path: string, alreadyAdded: boolean) {
  const display = formatUserHomePath(path);
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden" title={display}>
      <span className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <span className="truncate">{leafSegment(path)}</span>
        <span className="truncate text-[11px] text-muted-foreground">{display}</span>
      </span>
      <Badge variant="outline" className="text-[10px] text-muted-foreground shrink-0">
        {t("task:onDisk")}
      </Badge>
      {alreadyAdded ? <AlreadyAddedMarker /> : null}
    </span>
  );
}

function AlreadyAddedMarker() {
  const { t } = useTranslation();
  return (
    <span
      role="img"
      aria-label={t("task:alreadyAdded")}
      data-testid="already-added-repository-marker"
      className="shrink-0 text-primary"
    >
      <IconCheck aria-hidden="true" className="h-4 w-4" />
    </span>
  );
}
