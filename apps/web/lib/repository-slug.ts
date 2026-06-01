import type { Repository } from "@/lib/types/http";

/**
 * Canonical identity string for a repository, used as both the task board's
 * per-task `repositoryPath` and the sidebar filter's selectable option value
 * (and the repository grouping key). Both sides MUST derive it the same way —
 * otherwise a saved repository filter compares a clause value against a task
 * field that was built differently and silently matches nothing (#1213).
 *
 * Provider-backed repos use the `owner/name` slug; local repos fall back to the
 * repo name, then the last path segment, then the raw local path.
 */
export function repositorySlug(repo: Repository): string {
  if (repo.provider_owner && repo.provider_name) {
    return `${repo.provider_owner}/${repo.provider_name}`;
  }
  return repo.name || repo.local_path?.split("/").filter(Boolean).pop() || repo.local_path;
}
