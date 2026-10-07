import type { Repository, TaskSessionWorktree } from "@/lib/types/http";

type RepositoryLabelMetadata = Pick<Repository, "id" | "name" | "provider_owner" | "provider_name">;
type RepositoryLabelWorktree = Pick<
  TaskSessionWorktree,
  "repository_id" | "worktree_id" | "worktree_path" | "position"
>;

/** Builds labels keyed by workspace-relative paths without changing file identities. */
export function buildFileBrowserRepositoryLabels(
  workspacePath: string | null | undefined,
  worktrees: readonly RepositoryLabelWorktree[] | null | undefined,
  repositories: readonly RepositoryLabelMetadata[],
): Record<string, string> {
  if (!workspacePath || !worktrees?.length) return {};
  const repositoriesById = new Map(repositories.map((repository) => [repository.id, repository]));
  const candidates = worktrees.flatMap((worktree) => {
    if (!worktree.worktree_path || !worktree.repository_id) return [];
    const relativePath = relativeWorkspacePath(workspacePath, worktree.worktree_path);
    const repository = repositoriesById.get(worktree.repository_id);
    if (relativePath === null || !repository?.name) return [];
    return [{ relativePath, repository, worktree }];
  });
  const repositoryIdsByName = new Map<string, Set<string>>();
  for (const candidate of candidates) {
    const repositoryIds = repositoryIdsByName.get(candidate.repository.name) ?? new Set<string>();
    repositoryIds.add(candidate.repository.id);
    repositoryIdsByName.set(candidate.repository.name, repositoryIds);
  }

  return Object.fromEntries(
    candidates.map(({ relativePath, repository }) => {
      const duplicates = repositoryIdsByName.get(repository.name);
      if (!duplicates || duplicates.size < 2) return [relativePath, repository.name];
      const identity =
        repository.provider_owner || repository.provider_name || repository.id.slice(-6);
      return [relativePath, `${repository.name} (${identity} · ${repository.id.slice(-6)})`];
    }),
  );
}

/** Formats a workspace-relative path for display while retaining its real path elsewhere. */
export function labelFileBrowserPath(path: string, labels: Record<string, string>): string {
  const match = Object.keys(labels)
    .filter((root) => path === root || path.startsWith(`${root}/`))
    .sort((left, right) => right.length - left.length)[0];
  if (!match) return path;
  const remainder = path.slice(match.length).replace(/^\//, "");
  return remainder ? `${labels[match]}/${remainder}` : labels[match];
}

/** Changes when an active repository moves or is replaced, even if count is unchanged. */
export function workspaceInventoryRevision(
  worktrees: readonly RepositoryLabelWorktree[] = [],
): string {
  return JSON.stringify(
    [...worktrees]
      .sort(
        (left, right) =>
          left.position - right.position ||
          (left.worktree_id ?? "").localeCompare(right.worktree_id ?? ""),
      )
      .map(({ position, repository_id, worktree_id, worktree_path }) => [
        position,
        repository_id ?? "",
        worktree_id ?? "",
        worktree_path ?? "",
      ]),
  );
}

function relativeWorkspacePath(workspacePath: string, worktreePath: string): string | null {
  const root = normalizePath(workspacePath).replace(/\/$/, "");
  const checkout = normalizePath(worktreePath);
  if (checkout === root) return "";
  const prefix = `${root}/`;
  if (!checkout.startsWith(prefix)) return null;
  return checkout.slice(prefix.length);
}

function normalizePath(path: string): string {
  return path.replaceAll("\\", "/").replace(/\/{2,}/g, "/");
}
