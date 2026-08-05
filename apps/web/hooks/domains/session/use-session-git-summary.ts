import { useMemo } from "react";
import type { FileInfo } from "@/lib/state/slices/session-runtime/types";
import { useSessionGitStatusByRepo } from "./use-session-git-status";

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
  const repoNamesForControls = useMemo(() => {
    const seen = new Set<string>();
    for (const { repository_name } of statusByRepo) seen.add(repository_name);
    for (const repositoryName of reposInFiles) seen.add(repositoryName);
    const all = Array.from(seen).sort((a, b) => a.localeCompare(b));
    const named = all.filter((repositoryName) => repositoryName !== "");
    const hasRootStatus = statusByRepo.some((entry) => entry.repository_name === "");
    return hasRootStatus || named.length === 0 ? all : named;
  }, [statusByRepo, reposInFiles]);

  const perRepoStatus = useMemo(() => {
    if (statusByRepo.length === 0) return [];
    const stagedByRepo = new Map<string, boolean>();
    const unstagedByRepo = new Map<string, boolean>();
    for (const file of allFiles) {
      const repositoryName = file.repository_name ?? "";
      if (file.staged) stagedByRepo.set(repositoryName, true);
      else unstagedByRepo.set(repositoryName, true);
    }
    const hasRootStatus = statusByRepo.some((status) => status.repository_name === "");
    const hasNamed = statusByRepo.some((status) => status.repository_name !== "");
    let filtered = statusByRepo;
    if (!hasRootStatus && hasNamed) {
      filtered = statusByRepo.filter((status) => status.repository_name !== "");
    }
    return filtered.map(({ repository_name, status }) => ({
      repository_name,
      branch: status?.branch ?? null,
      ahead: status?.ahead ?? 0,
      behind: status?.behind ?? 0,
      hasStaged: stagedByRepo.get(repository_name) ?? false,
      hasUnstaged: unstagedByRepo.get(repository_name) ?? false,
    }));
  }, [statusByRepo, allFiles]);

  return { repoNamesForControls, perRepoStatus };
}
