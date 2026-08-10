import { describe, expect, it, vi } from "vitest";
import {
  createChangeRequestWithProvider,
  resolveChangeRequestProviderTarget,
} from "./change-request-creation";
import type { RepositoryProviderRegistration } from "./types";

const WORKSPACE_ID = "workspace-a";

function provider(
  overrides: Partial<RepositoryProviderRegistration> = {},
): RepositoryProviderRegistration & { pluginId: string } {
  return {
    pluginId: "bitbucket-plugin",
    id: "bitbucket",
    label: "Bitbucket",
    listRepositories: async () => [],
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
    createChangeRequest: async () => ({ url: "https://bitbucket.test/pr/1" }),
    ...overrides,
  };
}

const repositories = [
  { id: "repo-a", workspace_id: WORKSPACE_ID, name: "api", provider: "bitbucket" },
  { id: "repo-b", workspace_id: WORKSPACE_ID, name: "web", provider: "github" },
];
const task = {
  id: "task-a",
  workspaceId: WORKSPACE_ID,
  repositories: [
    { repository_id: "repo-a", position: 0 },
    { repository_id: "repo-b", position: 1 },
  ],
};

describe("resolveChangeRequestProviderTarget", () => {
  it("selects the persisted provider repository matching the Git repo scope", () => {
    const registration = provider();
    const target = resolveChangeRequestProviderTarget({
      task,
      repositories,
      repositoryScope: "api",
      getProvider: (id) => (id === "bitbucket" ? registration : undefined),
    });

    expect(target).toMatchObject({
      provider: registration,
      workspaceId: WORKSPACE_ID,
      taskId: "task-a",
      repositoryId: "repo-a",
      repository: repositories[0],
    });
  });

  it("uses the primary repository only when no multi-repo scope was supplied", () => {
    const registration = provider();
    const target = resolveChangeRequestProviderTarget({
      task,
      repositories,
      getProvider: () => registration,
    });

    expect(target?.repositoryId).toBe("repo-a");
  });

  it("falls back to the built-in flow when provider create is absent or scope is ambiguous", () => {
    expect(
      resolveChangeRequestProviderTarget({
        task,
        repositories,
        repositoryScope: "web",
        getProvider: () => provider({ createChangeRequest: undefined }),
      }),
    ).toBeNull();
    expect(
      resolveChangeRequestProviderTarget({
        task: {
          ...task,
          repositories: [...task.repositories, { repository_id: "repo-c", position: 2 }],
        },
        repositories: [...repositories, { ...repositories[0], id: "repo-c" }],
        repositoryScope: "api",
        getProvider: () => provider(),
      }),
    ).toBeNull();
  });
});

describe("createChangeRequestWithProvider", () => {
  it("pushes before invoking provider create and returns native PR result", async () => {
    const order: string[] = [];
    const push = vi.fn(async () => {
      order.push("push");
      return { success: true, operation: "push", output: "pushed" };
    });
    const createChangeRequest = vi.fn(async () => {
      order.push("create");
      return { url: "https://bitbucket.test/pr/42", provider: "bitbucket" };
    });

    const result = await createChangeRequestWithProvider({
      target: {
        provider: provider({ createChangeRequest }),
        workspaceId: WORKSPACE_ID,
        taskId: "task-a",
        repositoryId: "repo-a",
        repository: repositories[0],
      },
      push,
      repositoryScope: "api",
      title: "Create plugin PR",
      body: "Body",
      baseBranch: "main",
      draft: false,
      branchAlreadyPushed: false,
    });

    expect(order).toEqual(["push", "create"]);
    expect(push).toHaveBeenCalledWith({ setUpstream: true }, "api");
    expect(createChangeRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        workspaceId: WORKSPACE_ID,
        taskId: "task-a",
        repositoryId: "repo-a",
        title: "Create plugin PR",
        body: "Body",
        baseBranch: "main",
        draft: false,
        signal: expect.any(AbortSignal),
      }),
    );
    expect(result).toEqual({
      success: true,
      branch_pushed: true,
      pr_url: "https://bitbucket.test/pr/42",
      provider: "bitbucket",
      output: "pushed",
    });
  });

  it("does not invoke provider create when push fails", async () => {
    const createChangeRequest = vi.fn();
    const result = await createChangeRequestWithProvider({
      target: {
        provider: provider({ createChangeRequest }),
        workspaceId: WORKSPACE_ID,
        taskId: "task-a",
        repositoryId: "repo-a",
        repository: repositories[0],
      },
      push: async () => ({ success: false, operation: "push", output: "", error: "denied" }),
      title: "Title",
      body: "",
      draft: false,
      branchAlreadyPushed: false,
    });

    expect(createChangeRequest).not.toHaveBeenCalled();
    expect(result).toMatchObject({ success: false, branch_pushed: false, error: "denied" });
  });

  it("retries only provider creation after a successful push", async () => {
    const push = vi.fn();
    const createChangeRequest = vi.fn().mockRejectedValueOnce(new Error("provider unavailable"));
    const target = {
      provider: provider({ createChangeRequest }),
      workspaceId: WORKSPACE_ID,
      taskId: "task-a",
      repositoryId: "repo-a",
      repository: repositories[0],
    };

    const failed = await createChangeRequestWithProvider({
      target,
      push,
      title: "Title",
      body: "",
      draft: false,
      branchAlreadyPushed: true,
    });

    expect(push).not.toHaveBeenCalled();
    expect(failed).toMatchObject({
      success: false,
      branch_pushed: true,
      error: "provider unavailable",
    });
  });
});
