import { parseGitHubRepoUrl } from "./parse-url";
import type { RepoFilter } from "@/lib/types/github";

const SLUG_PATTERN = /^([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)$/;

/** Parses typed `owner/name` or a GitHub repository URL into a repository identity. */
export function parseGitHubRepositoryInput(value: string, githubHost?: string): RepoFilter | null {
  const trimmed = value.trim();
  const slug = trimmed.match(SLUG_PATTERN);
  if (slug) return { owner: slug[1]!, name: slug[2]! };
  const url = parseGitHubRepoUrl(trimmed, githubHost);
  return url ? { owner: url.owner, name: url.repo } : null;
}
