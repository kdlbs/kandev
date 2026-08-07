import { useMemo } from "react";
import type { FileInfo } from "@/lib/state/slices/session-runtime/types";
import { useSessionGitStatusByRepo } from "./use-session-git-status";

type RepositoryStatus = ReturnType<typeof useSessionGitStatusByRepo>[number];

export type RepositoryStatusSummary = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

export function deriveMultiRepoSummary(
  statusByRepo: RepositoryStatus[],
  allFiles: FileInfo[],
  reposInFiles: string[],
) {
  const seen = new Set<string>();
  for (const { repository_name } of statusByRepo) seen.add(repository_name);
  for (const repositoryName of reposInFiles) seen.add(repositoryName);
  const all = Array.from(seen).sort((a, b) => a.localeCompare(b));
  const named = all.filter((repositoryName) => repositoryName !== "");
  const hasRootStatus = statusByRepo.some((entry) => entry.repository_name === "");
  const repoNamesForControls = hasRootStatus || named.length === 0 ? all : named;

  if (statusByRepo.length === 0) {
    return { repoNamesForControls, perRepoStatus: [] as RepositoryStatusSummary[] };
  }

  const stagedByRepo = new Map<string, boolean>();
  const unstagedByRepo = new Map<string, boolean>();
  for (const file of allFiles) {
    const repositoryName = file.repository_name ?? "";
    if (file.staged) stagedByRepo.set(repositoryName, true);
    else unstagedByRepo.set(repositoryName, true);
  }
  const hasNamed = statusByRepo.some((status) => status.repository_name !== "");
  const filtered =
    !hasRootStatus && hasNamed
      ? statusByRepo.filter((status) => status.repository_name !== "")
      : statusByRepo;
  const perRepoStatus = filtered.map(({ repository_name, status }) => ({
    repository_name,
    branch: status?.branch ?? null,
    ahead: status?.ahead ?? 0,
    behind: status?.behind ?? 0,
    hasStaged: stagedByRepo.get(repository_name) ?? false,
    hasUnstaged: unstagedByRepo.get(repository_name) ?? false,
  }));
  return { repoNamesForControls, perRepoStatus };
}

/**
 * Derives the repository names and status summaries used by Changes-panel
 * repository controls. A bare multi-repo root has no real empty scope, while
 * a Git root with submodules does and must retain it alongside its children.
 */
export function useMultiRepoSummary(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  allFiles: FileInfo[],
  reposInFiles: string[],
) {
  return useMemo(
    () => deriveMultiRepoSummary(statusByRepo, allFiles, reposInFiles),
    [statusByRepo, allFiles, reposInFiles],
  );
}
