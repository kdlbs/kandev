import type { Repository } from "@/lib/types/http";

export type TaskPullRequestLinkTarget = {
  id: string;
  title: string;
  repositoryId?: string;
  repositories?: Array<{ repository_id: string }>;
};

export type GitHubPRRepoRef = {
  owner: string;
  repo: string;
  repositoryId: string;
};

export type ParsedPullRequestURL = {
  owner: string;
  repo: string;
  number: number;
  url: string;
};

export function githubReposForTask(
  task: TaskPullRequestLinkTarget,
  repositories: Repository[],
): GitHubPRRepoRef[] {
  const repoIds = new Set((task.repositories ?? []).map((repo) => repo.repository_id));
  if (task.repositoryId) repoIds.add(task.repositoryId);

  return repositories
    .filter((repo) => repoIds.has(repo.id) && repo.provider === "github")
    .map((repo) => ({
      owner: repo.provider_owner,
      repo: repo.provider_name,
      repositoryId: repo.id,
    }))
    .filter((repo) => repo.owner && repo.repo);
}

export function parseGitHubPullRequestURL(input: string): ParsedPullRequestURL | null {
  const match = input
    .trim()
    .match(/^(?:https?:\/\/)?github\.com\/([^/\s]+)\/([^/\s]+)\/pull\/(\d+)(?:[/?#].*)?$/i);
  if (!match) return null;
  const [, owner, repo, number] = match;
  const parsedNumber = Number(number);
  if (!Number.isSafeInteger(parsedNumber) || parsedNumber <= 0) return null;
  return {
    owner,
    repo,
    number: parsedNumber,
    url: `https://github.com/${owner}/${repo}/pull/${parsedNumber}`,
  };
}

function positivePullRequestNumber(input: string): string | null {
  const number = input.replace(/^#/, "");
  if (!/^\d+$/.test(number)) return null;
  const parsedNumber = Number(number);
  if (!Number.isSafeInteger(parsedNumber) || parsedNumber <= 0) return null;
  return String(parsedNumber);
}

export function pullRequestPayload(input: string, githubRepos: GitHubPRRepoRef[]) {
  const trimmed = input.trim();
  const inferredNumber = positivePullRequestNumber(trimmed);
  const inferredRepo = inferredNumber && githubRepos.length === 1 ? githubRepos[0] : null;
  if (inferredRepo && inferredNumber) {
    return {
      pr_url: `https://github.com/${inferredRepo.owner}/${inferredRepo.repo}/pull/${inferredNumber}`,
      repository_id: inferredRepo.repositoryId,
    };
  }

  const parsed = parseGitHubPullRequestURL(trimmed);
  const matchingRepo = parsed
    ? githubRepos.find(
        (repo) =>
          repo.owner.toLowerCase() === parsed.owner.toLowerCase() &&
          repo.repo.toLowerCase() === parsed.repo.toLowerCase(),
      )
    : null;

  return {
    pr_url: parsed?.url ?? trimmed,
    repository_id: matchingRepo?.repositoryId,
  };
}
