import { describe, expect, it } from "vitest";
import { buildGitHubRepoUrl, parseGitHubRepoUrl } from "./github-repo-url";

describe("parseGitHubRepoUrl", () => {
  it("parses a plain repository URL", () => {
    expect(parseGitHubRepoUrl("https://github.com/jcfs/kandev-workflows-test")).toEqual({
      owner: "jcfs",
      repo: "kandev-workflows-test",
    });
  });

  it("tolerates trailing slash, .git suffix, www, and missing scheme", () => {
    expect(parseGitHubRepoUrl("https://github.com/jcfs/repo/")).toEqual({
      owner: "jcfs",
      repo: "repo",
    });
    expect(parseGitHubRepoUrl("https://www.github.com/jcfs/repo.git")).toEqual({
      owner: "jcfs",
      repo: "repo",
    });
    expect(parseGitHubRepoUrl("github.com/jcfs/repo")).toEqual({ owner: "jcfs", repo: "repo" });
  });

  it("parses an SSH remote", () => {
    expect(parseGitHubRepoUrl("git@github.com:jcfs/repo.git")).toEqual({
      owner: "jcfs",
      repo: "repo",
    });
  });

  it("extracts branch and directory from a /tree/ link", () => {
    expect(
      parseGitHubRepoUrl(
        "https://github.com/jcfs/kandev-workflows-test/tree/main/.kandev/workflows",
      ),
    ).toEqual({
      owner: "jcfs",
      repo: "kandev-workflows-test",
      branch: "main",
      path: ".kandev/workflows",
    });
  });

  it("extracts branch without path from a branch-root /tree/ link", () => {
    expect(parseGitHubRepoUrl("https://github.com/jcfs/repo/tree/develop")).toEqual({
      owner: "jcfs",
      repo: "repo",
      branch: "develop",
    });
  });

  it("resolves a /blob/ file link to the file's directory", () => {
    expect(
      parseGitHubRepoUrl("https://github.com/jcfs/repo/blob/main/.kandev/workflows/dev.yml"),
    ).toEqual({
      owner: "jcfs",
      repo: "repo",
      branch: "main",
      path: ".kandev/workflows",
    });
  });

  it("ignores unknown path markers beyond owner/repo", () => {
    expect(parseGitHubRepoUrl("https://github.com/jcfs/repo/pulls")).toEqual({
      owner: "jcfs",
      repo: "repo",
    });
  });

  it("rejects non-GitHub and malformed input", () => {
    expect(parseGitHubRepoUrl("https://gitlab.com/jcfs/repo")).toBeNull();
    expect(parseGitHubRepoUrl("https://github.com/only-owner")).toBeNull();
    expect(parseGitHubRepoUrl("not a url at all :::")).toBeNull();
    expect(parseGitHubRepoUrl("")).toBeNull();
  });

  it("decodes percent-encoded path segments", () => {
    expect(parseGitHubRepoUrl("https://github.com/jcfs/repo/tree/main/my%20flows")).toEqual({
      owner: "jcfs",
      repo: "repo",
      branch: "main",
      path: "my flows",
    });
  });
});

describe("buildGitHubRepoUrl", () => {
  it("renders owner/repo/branch/path back into a tree link", () => {
    const url = buildGitHubRepoUrl({
      owner: "jcfs",
      repo: "kandev-workflows-test",
      branch: "main",
      path: ".kandev/workflows",
    });
    expect(url).toBe("https://github.com/jcfs/kandev-workflows-test/tree/main/.kandev/workflows");
  });

  it("round-trips through parseGitHubRepoUrl", () => {
    const parts = { owner: "jcfs", repo: "repo", branch: "dev", path: "my flows/sub" };
    expect(parseGitHubRepoUrl(buildGitHubRepoUrl(parts))).toEqual(parts);
  });

  it("omits the tree suffix without a branch and the path when empty", () => {
    expect(buildGitHubRepoUrl({ owner: "jcfs", repo: "repo" })).toBe(
      "https://github.com/jcfs/repo",
    );
    expect(buildGitHubRepoUrl({ owner: "jcfs", repo: "repo", branch: "main" })).toBe(
      "https://github.com/jcfs/repo/tree/main",
    );
  });
});
