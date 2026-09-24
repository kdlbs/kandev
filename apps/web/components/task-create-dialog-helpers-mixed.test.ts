import { describe, expect, it } from "vitest";
import {
  buildRepositoriesPayload,
  buildWorkspaceSourcesPayload,
} from "./task-create-dialog-helpers";
import type { TaskRepositorySelection } from "@/components/task-create-dialog-types";

const FRONT_REPOSITORY_ID = "repo-front";

// eslint-disable-next-line max-lines-per-function -- mixed payload cases share one serializer fixture.
describe("buildRepositoriesPayload — unified rows", () => {
  it("maps mixed selections in their original order", () => {
    const selections: TaskRepositorySelection[] = [
      { kind: "local", key: "local-1", repositoryId: "repo-local", branch: "main" },
      {
        kind: "remote",
        key: "remote-1",
        url: "https://git.example/acme/remote",
        remoteUrl: "https://git.example/acme/remote.git",
        provider: "bitbucket",
        providerHost: "git.example",
        providerScope: "acme",
        providerRepoId: "remote-1",
        providerOwner: "acme",
        providerName: "remote",
        branch: "develop",
        source: "picker",
      },
      { kind: "local", key: "local-2", localPath: "/tmp/second", branch: "trunk" },
    ];

    expect(
      buildRepositoriesPayload({
        selections,
        useRemote: false,
        remoteRepos: [],
        repositories: [],
        discoveredRepositories: [{ path: "/tmp/second", name: "second", default_branch: "trunk" }],
      }),
    ).toEqual([
      {
        repository_id: "repo-local",
        base_branch: "main",
        checkout_branch: undefined,
      },
      {
        repository_id: "",
        base_branch: "develop",
        checkout_branch: undefined,
        remote_url: "https://git.example/acme/remote.git",
        provider: "bitbucket",
        provider_host: "git.example",
        provider_scope: "acme",
        provider_repo_id: "remote-1",
        provider_owner: "acme",
        provider_name: "remote",
      },
      {
        repository_id: "",
        base_branch: "trunk",
        checkout_branch: undefined,
        local_path: "/tmp/second",
        default_branch: "trunk",
      },
    ]);
  });

  it.each([
    { name: "Local", isLocalExecutor: true },
    { name: "Worktree", isLocalExecutor: false },
  ])("drops remote-origin intent when switching back to $name execution", ({ isLocalExecutor }) => {
    const selections: TaskRepositorySelection[] = [
      {
        kind: "local",
        key: "local-1",
        repositoryId: "repo-host",
        branch: "feature/work",
        baseBranch: "main",
        checkoutSource: "remote_origin",
        expectedOrigin: "https://github.com/acme/host.git",
      },
    ];
    const options = {
      selections,
      useRemote: false,
      remoteRepos: [],
      repositories: [],
      discoveredRepositories: [],
      isLocalExecutor,
      remoteOriginMode: false,
    };

    const repositoriesPayload = buildRepositoriesPayload(options);
    expect(repositoriesPayload[0]).toMatchObject({ repository_id: "repo-host" });
    expect(repositoriesPayload[0]).not.toHaveProperty("checkout_source");
    expect(repositoriesPayload[0]).not.toHaveProperty("expected_origin");

    const workspaceSourcesPayload = buildWorkspaceSourcesPayload(options);
    expect(workspaceSourcesPayload[0]).toMatchObject({
      kind: "repository",
      repository_id: "repo-host",
    });
    expect(workspaceSourcesPayload[0]).not.toHaveProperty("checkout_source");
    expect(workspaceSourcesPayload[0]).not.toHaveProperty("expected_origin");
  });

  it("carries remote checkout options through ordered workspace sources", () => {
    const checkoutOptions = {
      version: 1 as const,
      download_mode: "on_demand" as const,
      sparse_directories: ["extensions/shared"],
    };
    const workspaceSourcesPayload = buildWorkspaceSourcesPayload({
      selections: [
        {
          kind: "remote",
          key: "remote-1",
          url: "https://github.com/acme/remote",
          branch: "main",
          source: "paste",
          checkoutOptions,
        },
      ],
      useRemote: false,
      remoteRepos: [],
      repositories: [],
      discoveredRepositories: [],
    });

    expect(workspaceSourcesPayload).toEqual([
      expect.objectContaining({
        kind: "repository",
        github_url: "https://github.com/acme/remote",
        checkout_options: checkoutOptions,
      }),
    ]);
  });

  it("maps each row in order, dropping empty ones silently", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [
        { key: "r0", repositoryId: FRONT_REPOSITORY_ID, branch: "main" },
        { key: "r1", repositoryId: "repo-back", branch: "develop" },
        { key: "r2", branch: "" }, // no repo picked yet — dropped
        { key: "r3", repositoryId: "repo-shared", branch: "" },
      ],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: FRONT_REPOSITORY_ID,
        base_branch: "main",
        checkout_branch: undefined,
      },
      { repository_id: "repo-back", base_branch: "develop", checkout_branch: undefined },
      { repository_id: "repo-shared", base_branch: undefined, checkout_branch: undefined },
    ]);
  });
});

describe("buildRepositoriesPayload — branch and discovered rows", () => {
  it("submits a selected branch policy id without deriving identity from its label", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [
        {
          key: "r0",
          repositoryId: FRONT_REPOSITORY_ID,
          branch: "develop",
          branchPolicyId: "policy-hotfix",
        },
      ],
      discoveredRepositories: [],
    });

    expect(payload).toEqual([
      {
        repository_id: FRONT_REPOSITORY_ID,
        base_branch: "develop",
        checkout_branch: undefined,
        branch_policy_id: "policy-hotfix",
      },
    ]);
  });

  it("emits local_path + default_branch for discovered (on-machine) rows", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
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
