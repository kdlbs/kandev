import { describe, it, expect } from "vitest";
import { buildRepositoriesPayload } from "./task-create-dialog-helpers";

describe("buildRepositoriesPayload — unified rows", () => {
  it("maps each row in order, dropping empty ones silently", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [
        { key: "r0", repositoryId: "repo-front", branch: "main" },
        { key: "r1", repositoryId: "repo-back", branch: "develop" },
        { key: "r2", branch: "" }, // no repo picked yet — dropped
        { key: "r3", repositoryId: "repo-shared", branch: "" },
      ],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      { repository_id: "repo-front", base_branch: "main" },
      { repository_id: "repo-back", base_branch: "develop" },
      { repository_id: "repo-shared", base_branch: undefined },
    ]);
  });

  it("emits local_path + default_branch for discovered (on-machine) rows", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [
        { key: "r0", localPath: "/home/me/projects/local-project", branch: "trunk" },
        { key: "r1", repositoryId: "repo-back", branch: "main" },
      ],
      discoveredRepositories: [
        { path: "/home/me/projects/local-project", default_branch: "trunk" },
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ] as any,
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "trunk",
        local_path: "/home/me/projects/local-project",
        default_branch: "trunk",
      },
      { repository_id: "repo-back", base_branch: "main" },
    ]);
  });

  it("URL mode produces a single github_url entry and ignores the rows", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: true,
      githubUrl: "github.com/owner/repo",
      githubBranch: "feature-x",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "ignored", branch: "ignored" }],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "feature-x",
        checkout_branch: undefined,
        github_url: "github.com/owner/repo",
      },
    ]);
  });

  it("single-row workspace repo: payload mirrors the row", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "repo-only", branch: "main" }],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([{ repository_id: "repo-only", base_branch: "main" }]);
  });
});
