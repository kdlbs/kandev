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
      { repository_id: "repo-front", base_branch: "main", checkout_branch: undefined },
      { repository_id: "repo-back", base_branch: "develop", checkout_branch: undefined },
      { repository_id: "repo-shared", base_branch: undefined, checkout_branch: undefined },
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
        checkout_branch: undefined,
        local_path: "/home/me/projects/local-project",
        default_branch: "trunk",
      },
      { repository_id: "repo-back", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});

// Regression for the "new branch on local executor" bug: the chip's branch
// is the working branch on disk (e.g. "feature/x"), not the integration
// branch. We must send it as `checkout_branch`, with `base_branch` anchored
// to the repo's `default_branch`. Without this, agentctl recomputes
// merge-base(HEAD, origin/feature/x) which collapses to HEAD and the
// changes panel is empty after refresh.
describe("buildRepositoriesPayload — local executor branch split (core)", () => {
  it("rowBranch != default_branch → swap into checkout_branch", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "feature/x" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "main", checkout_branch: "feature/x" },
    ]);
  });

  it("rowBranch matches default_branch → no checkout_branch", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "main" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "main", checkout_branch: undefined },
    ]);
  });

  it("localPath row uses discoveredRepositories.default_branch", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", localPath: "/p/r", branch: "feature/y" }],
      discoveredRepositories: [
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        { path: "/p/r", default_branch: "main" } as any,
      ],
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "main",
        checkout_branch: "feature/y",
        local_path: "/p/r",
        default_branch: "main",
      },
    ]);
  });
});

describe("buildRepositoriesPayload — local executor branch split (edge cases)", () => {
  it("fresh-branch flow: skips the split so the picked base is preserved as base_branch", () => {
    // When the user enables "Fork a new branch", the chip's branch is the
    // BASE TO FORK FROM (e.g. "develop"), not a working branch. The backend
    // creates a new branch from that base and rewrites base_branch to the
    // new branch name. If we split here, base_branch would land on the
    // repo's default ("main") and the fork would happen from main instead
    // of develop — silently wrong.
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "develop" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
      freshBranch: { confirmDiscard: false, consentedDirtyFiles: [] },
    });
    expect(payload).toEqual([
      {
        repository_id: "repo-1",
        base_branch: "develop",
        checkout_branch: undefined,
        fresh_branch: true,
        confirm_discard: false,
        consented_dirty_files: [],
      },
    ]);
  });

  it("falls through when default_branch is unknown (legacy repos)", () => {
    // Repos created before the backend probe fix may have an unset
    // default_branch in the workspace store. If we synthesize base_branch=
    // rowBranch here (as the original draft did), we reproduce the very bug
    // this PR fixes: agentctl recomputes merge-base(HEAD, origin/<rowBranch>)
    // → collapses to HEAD → empty changes panel. Better to leave the legacy
    // shape alone — the next backend createRepository call will populate
    // default_branch via the gitref probe.
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "feature/x" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "" }] as any,
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "feature/x", checkout_branch: undefined },
    ]);
  });

  it("non-local executor leaves rowBranch as base_branch (worktree flow unchanged)", () => {
    const payload = buildRepositoriesPayload({
      useGitHubUrl: false,
      githubUrl: "",
      githubBranch: "",
      githubPrHeadBranch: null,
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "main" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: false,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});

describe("buildRepositoriesPayload — single-row and URL mode", () => {
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
    expect(payload).toEqual([
      { repository_id: "repo-only", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});
