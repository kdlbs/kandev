import type { Repository } from "@/lib/types/http";

/**
 * Canonical identity for a repository — the task board's per-task
 * `repositoryPath`, the sidebar filter's option value, and the grouping key.
 * All sites MUST derive it identically, or a saved repo filter compares
 * mismatched strings and silently matches nothing (#1213).
 */
export function repositorySlug(repo: Repository): string {
  if (repo.provider_owner && repo.provider_name) {
    return `${repo.provider_owner}/${repo.provider_name}`;
  }
  return repo.name || repo.local_path.split("/").filter(Boolean).pop() || repo.local_path;
}
