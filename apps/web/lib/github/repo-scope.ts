import type { GitHubWorkspaceSettings, RepoFilter } from "@/lib/types/github";

function same(a: string, b: string): boolean {
  return a.trim().toLowerCase() === b.trim().toLowerCase();
}

/** True when the workspace repository scope allows `owner/repo`. No settings means no restriction. */
export function repoAllowedByScope(
  settings: GitHubWorkspaceSettings | null | undefined,
  owner: string,
  repo: string,
): boolean {
  if (!settings || settings.repo_scope_mode === "all") return true;
  if (settings.repo_scope_mode === "orgs") {
    return (settings.repo_scope_orgs ?? []).some((allowed) => same(allowed, owner));
  }
  return (settings.repo_scope_repos ?? []).some(
    (allowed) => same(allowed.owner, owner) && same(allowed.name, repo),
  );
}

/** True when the workspace repository scope allows selecting every repository of `org`. */
export function orgAllowedByScope(
  settings: GitHubWorkspaceSettings | null | undefined,
  org: string,
): boolean {
  if (!settings || settings.repo_scope_mode === "all") return true;
  if (settings.repo_scope_mode === "orgs") {
    return (settings.repo_scope_orgs ?? []).some((allowed) => same(allowed, org));
  }
  return false;
}

/** Case-insensitive identity match for repository filters. */
export function sameRepoFilter(a: RepoFilter, b: RepoFilter): boolean {
  return same(a.owner, b.owner) && same(a.name, b.name);
}
