import { describe, expect, it } from "vitest";
import {
  addWorkspaceSourceRow,
  buildWorkspaceSourcesPayload,
  getWorkspaceSourceValidation,
  updateWorkspaceSourceRow,
} from "./workspace-source-state";

describe("workspace source state payloads and validation", () => {
  it("builds a deterministic mixed repository, remote, and folder payload", () => {
    const rows = [
      {
        key: "saved",
        kind: "repository" as const,
        repositoryId: "repo-1",
        baseBranch: "main",
        checkoutBranch: "feature/a",
      },
      {
        key: "remote",
        kind: "repository" as const,
        remoteUrl: "https://github.com/acme/api.git",
        provider: "github" as const,
        providerRepoId: "42",
        providerOwner: "acme",
        providerName: "api",
        baseBranch: "trunk",
        checkoutBranch: "release/2026",
      },
      { key: "folder", kind: "folder" as const, localPath: "/src/docs", displayName: "docs" },
    ];

    expect(buildWorkspaceSourcesPayload(rows)).toEqual({
      sources: [
        {
          kind: "repository",
          repository_id: "repo-1",
          base_branch: "main",
          checkout_branch: "feature/a",
        },
        {
          kind: "repository",
          remote_url: "https://github.com/acme/api.git",
          provider: "github",
          provider_repo_id: "42",
          provider_owner: "acme",
          provider_name: "api",
          base_branch: "trunk",
          checkout_branch: "release/2026",
        },
        { kind: "folder", local_path: "/src/docs", display_name: "docs" },
      ],
    });
  });

  it("validates each row independently and retains its input for retry", () => {
    const invalid = { key: "bad", kind: "repository" as const, remoteUrl: "not a url" };
    const valid = { key: "good", kind: "folder" as const, localPath: "/src/docs" };

    expect(getWorkspaceSourceValidation([invalid, valid])).toEqual({
      bad: "Enter a valid remote repository URL and base branch.",
    });
    expect(
      updateWorkspaceSourceRow([invalid, valid], "bad", { remoteUrl: "https://x/y.git" }),
    ).toEqual([{ ...invalid, remoteUrl: "https://x/y.git" }, valid]);
  });

  it("accepts task-create's supported SCP remotes and rejects unsupported hosts", () => {
    const rows = [
      {
        key: "github-ssh",
        kind: "repository" as const,
        remoteUrl: "git@github.com:acme/api.git",
        baseBranch: "main",
      },
      {
        key: "gitlab-ssh",
        kind: "repository" as const,
        remoteUrl: "git@gitlab.com:acme/web.git",
        baseBranch: "main",
      },
      {
        key: "azure-ssh",
        kind: "repository" as const,
        remoteUrl: "git@ssh.dev.azure.com:v3/acme/platform/api",
        baseBranch: "main",
      },
      {
        key: "unsupported",
        kind: "repository" as const,
        remoteUrl: "https://bitbucket.org/acme/api.git",
        baseBranch: "main",
      },
    ];

    expect(getWorkspaceSourceValidation(rows)).toEqual({
      unsupported: "Enter a valid remote repository URL and base branch.",
    });
  });

  it("rejects duplicate repository/branch pairs and canonical folder paths", () => {
    const rows = [
      { key: "one", kind: "repository" as const, repositoryId: "repo-1", baseBranch: "main" },
      { key: "two", kind: "repository" as const, repositoryId: "repo-1", baseBranch: "main" },
      { key: "three", kind: "folder" as const, localPath: "/src/docs/" },
      { key: "four", kind: "folder" as const, localPath: "/src/docs" },
    ];

    expect(getWorkspaceSourceValidation(rows)).toEqual({
      two: "This repository and branch are already selected.",
      four: "This folder is already selected.",
    });
  });
});

describe("workspace source state row creation", () => {
  it("does not add folder rows for an executor without folder support", () => {
    expect(addWorkspaceSourceRow([], "folder", "docker")).toEqual([]);
    expect(addWorkspaceSourceRow([], "folder", "worktree")).toEqual([
      {
        key: expect.any(String),
        kind: "folder",
        sourceType: "folder",
        localPath: "",
        displayName: "",
      },
    ]);
  });

  it("adds explicit saved, local, remote, and folder rows into one mixed batch", () => {
    const saved = addWorkspaceSourceRow([], "saved_repository", "worktree");
    const local = addWorkspaceSourceRow(saved, "local_repository", "worktree");
    const remote = addWorkspaceSourceRow(local, "remote_repository", "worktree");
    const folder = addWorkspaceSourceRow(remote, "folder", "worktree");
    const rows = [
      { ...folder[0], repositoryId: "repo-1", baseBranch: "main" },
      {
        ...folder[1],
        localPath: "/repos/local",
        baseBranch: "main",
        checkoutBranch: "feature/local",
      },
      { ...folder[2], remoteUrl: "https://github.com/acme/remote.git", baseBranch: "trunk" },
      { ...folder[3], localPath: "/repos/docs" },
    ];

    expect(rows.map((row) => row.sourceType)).toEqual([
      "saved_repository",
      "local_repository",
      "remote_repository",
      "folder",
    ]);
    expect(buildWorkspaceSourcesPayload(rows).sources[1]).toMatchObject({
      local_path: "/repos/local",
      checkout_branch: "feature/local",
    });
    expect(getWorkspaceSourceValidation(rows)).toEqual({});
  });
});
