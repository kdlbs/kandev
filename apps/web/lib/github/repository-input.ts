import { normalizeGitHubHost, PUBLIC_GITHUB_HOST } from "./host";
import { parseGitHubRepoUrl } from "./parse-url";
import type { RepoFilter } from "@/lib/types/github";

const SLUG_PATTERN = /^([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)$/;

/**
 * Parses typed `owner/name` or a GitHub repository URL into a repository
 * identity. A URL must be on the workspace host (github.com when omitted);
 * the identity carries no host, so a URL from another host is rejected.
 */
export function parseGitHubRepositoryInput(value: string, githubHost?: string): RepoFilter | null {
  const trimmed = value.trim();
  const slug = trimmed.match(SLUG_PATTERN);
  if (slug) return { owner: slug[1]!, name: slug[2]! };
  const url = parseGitHubRepoUrl(trimmed, githubHost);
  if (!url) return null;
  const workspaceHost = normalizeGitHubHost(githubHost ?? PUBLIC_GITHUB_HOST);
  return (url.host ?? PUBLIC_GITHUB_HOST) === workspaceHost
    ? { owner: url.owner, name: url.repo }
    : null;
}

// GitHub logins: 1-39 alphanumerics or hyphens, starting with an alphanumeric.
const LOGIN_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$/;

/** Parses a typed GitHub user or organization login. */
export function parseGitHubLogin(value: string): string | null {
  const trimmed = value.trim();
  return LOGIN_PATTERN.test(trimmed) ? trimmed : null;
}
