import { describe, it, expect } from "vitest";
import { buildRepositoriesPayload } from "./task-create-dialog-helpers";

describe("buildRepositoriesPayload — multi-repo", () => {
  it("appends extra repositories after the primary in order", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      branch: "main",
      githubPrHeadBranch: null,
      repositoryId: "repo-front",
      selectedLocalRepo: null,
      extraRepositories: [
        { repositoryId: "repo-back", branch: "develop" },
        { repositoryId: "repo-shared", branch: "" },
      ],
    });
    expect(payload).toEqual([
      { repository_id: "repo-front", base_branch: "main", checkout_branch: undefined },
      { repository_id: "repo-back", base_branch: "develop" },
      { repository_id: "repo-shared", base_branch: undefined },
    ]);
  });

  it("drops extra rows with empty repository_id", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      branch: "main",
      githubPrHeadBranch: null,
      repositoryId: "repo-front",
      selectedLocalRepo: null,
      extraRepositories: [
        { repositoryId: "", branch: "" },
        { repositoryId: "repo-back", branch: "main" },
      ],
    });
    expect(payload).toHaveLength(2);
    expect(payload[1].repository_id).toBe("repo-back");
  });

  it("returns extras even when no primary is set (recovery shape)", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      branch: "",
      githubPrHeadBranch: null,
      repositoryId: "",
      selectedLocalRepo: null,
      extraRepositories: [{ repositoryId: "repo-x", branch: "main" }],
    });
    expect(payload).toEqual([{ repository_id: "repo-x", base_branch: "main" }]);
  });

  it("single-repo single call: behaves identically to before extraRepositories was added", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      branch: "main",
      githubPrHeadBranch: null,
      repositoryId: "repo-only",
      selectedLocalRepo: null,
    });
    expect(payload).toEqual([
      { repository_id: "repo-only", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});
