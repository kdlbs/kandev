import { describe, expect, it } from "vitest";
import {
  githubReposForTask,
  parseGitHubPullRequestURL,
  pullRequestPayload,
  type GitHubPRRepoRef,
} from "./task-github-pr-url";
import { repositoryId, workspaceId, type Repository } from "@/lib/types/http";

type RepoOverrides = Partial<Omit<Repository, "id" | "workspace_id">> & { id: string };

function repo(overrides: RepoOverrides): Repository {
  const { id, ...rest } = overrides;
  return {
    ...rest,
    id: repositoryId(id),
    workspace_id: workspaceId("workspace-1"),
    name: overrides.name ?? "Repo",
    source_type: "remote",
    local_path: "",
    provider: overrides.provider ?? "github",
    provider_repo_id: "",
    provider_owner: overrides.provider_owner ?? "kdlbs",
    provider_name: overrides.provider_name ?? "kandev",
    default_branch: "main",
    worktree_branch_prefix: "",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    worktree_files: [],
    created_at: "2026-06-23T00:00:00Z",
    updated_at: "2026-06-23T00:00:00Z",
  };
}

const KANDEV_PR_URL = "https://github.com/kdlbs/kandev/pull/1471";

describe("githubReposForTask", () => {
  it("maps task repositories to GitHub repository ids, not task-repository row ids", () => {
    const refs = githubReposForTask(
      {
        id: "task-1",
        title: "Task",
        repositories: [{ repository_id: repositoryId("repo-1") }],
      },
      [
        repo({ id: "repo-1", provider_owner: "kdlbs", provider_name: "kandev" }),
        repo({ id: "repo-2", provider_owner: "acme", provider_name: "site" }),
      ],
    );

    expect(refs).toEqual([{ owner: "kdlbs", repo: "kandev", repositoryId: "repo-1" }]);
  });

  it("uses the task repositoryId fallback and ignores non-GitHub repositories", () => {
    const refs = githubReposForTask(
      { id: "task-1", title: "Task", repositoryId: repositoryId("repo-2") },
      [
        repo({ id: "repo-1", provider_owner: "kdlbs", provider_name: "kandev" }),
        repo({ id: "repo-2", provider: "gitlab", provider_owner: "acme", provider_name: "site" }),
      ],
    );

    expect(refs).toEqual([]);
  });
});

describe("parseGitHubPullRequestURL", () => {
  it("normalizes full GitHub pull request URLs", () => {
    expect(parseGitHubPullRequestURL(KANDEV_PR_URL)).toEqual({
      owner: "kdlbs",
      repo: "kandev",
      number: 1471,
      url: KANDEV_PR_URL,
    });
    expect(parseGitHubPullRequestURL("github.com/kdlbs/kandev/pull/1471/")).toEqual({
      owner: "kdlbs",
      repo: "kandev",
      number: 1471,
      url: KANDEV_PR_URL,
    });
    expect(
      parseGitHubPullRequestURL("https://github.com/kdlbs/kandev/pull/1471?tab=files#diff"),
    ).toEqual({
      owner: "kdlbs",
      repo: "kandev",
      number: 1471,
      url: KANDEV_PR_URL,
    });
  });

  it("rejects malformed or non-GitHub pull request inputs", () => {
    expect(parseGitHubPullRequestURL("https://gitlab.com/kdlbs/kandev/pull/1471")).toBeNull();
    expect(parseGitHubPullRequestURL("https://github.com/kdlbs/kandev/issues/1471")).toBeNull();
    expect(parseGitHubPullRequestURL("https://github.com/kdlbs/kandev/pull/0")).toBeNull();
    expect(parseGitHubPullRequestURL("https://github.com/kdlbs/kandev/pull/00")).toBeNull();
    expect(parseGitHubPullRequestURL("not a url")).toBeNull();
  });
});

describe("pullRequestPayload", () => {
  const githubRepos: GitHubPRRepoRef[] = [
    { owner: "kdlbs", repo: "kandev", repositoryId: "repo-1" },
    { owner: "acme", repo: "site", repositoryId: "repo-2" },
  ];

  it("infers a bare PR number when exactly one GitHub repo is attached", () => {
    expect(pullRequestPayload("#1471", [githubRepos[0]])).toEqual({
      pr_url: KANDEV_PR_URL,
      repository_id: "repo-1",
    });
  });

  it("does not infer bare numbers for multi-repo tasks", () => {
    expect(pullRequestPayload("1471", githubRepos)).toEqual({
      pr_url: "1471",
      repository_id: undefined,
    });
  });

  it("does not infer non-positive PR numbers", () => {
    expect(pullRequestPayload("#0", [githubRepos[0]])).toEqual({
      pr_url: "#0",
      repository_id: undefined,
    });
    expect(pullRequestPayload("00", [githubRepos[0]])).toEqual({
      pr_url: "00",
      repository_id: undefined,
    });
  });

  it("attaches the repository id when a URL matches a task GitHub repo", () => {
    expect(pullRequestPayload("https://github.com/ACME/site/pull/5", githubRepos)).toEqual({
      pr_url: "https://github.com/ACME/site/pull/5",
      repository_id: "repo-2",
    });
  });

  it("keeps unmatched and malformed inputs without a repository id", () => {
    expect(pullRequestPayload("https://github.com/other/repo/pull/5", githubRepos)).toEqual({
      pr_url: "https://github.com/other/repo/pull/5",
      repository_id: undefined,
    });
    expect(pullRequestPayload("  not a url  ", [githubRepos[0]])).toEqual({
      pr_url: "not a url",
      repository_id: undefined,
    });
  });
});
