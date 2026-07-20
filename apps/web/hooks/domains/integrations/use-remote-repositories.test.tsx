import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchAccessibleRepos: vi.fn(),
  listUserProjects: vi.fn(),
  listAzureDevOpsProjects: vi.fn(),
  listAzureDevOpsRepositories: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchAccessibleRepos: mocks.fetchAccessibleRepos,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({ listUserProjects: mocks.listUserProjects }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listAzureDevOpsProjects: mocks.listAzureDevOpsProjects,
  listAzureDevOpsRepositories: mocks.listAzureDevOpsRepositories,
}));

import { useRemoteRepositories } from "./use-remote-repositories";

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function rejectUnavailableProviders() {
  mocks.listUserProjects.mockRejectedValue(new Error("GitLab not configured"));
  mocks.listAzureDevOpsProjects.mockRejectedValue(new Error("Azure DevOps not configured"));
}

describe("useRemoteRepositories", () => {
  it("combines successful providers while tolerating a provider failure", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([
      {
        owner: "acme",
        name: "web",
        full_name: "acme/web",
        default_branch: "main",
        private: false,
      },
    ]);
    mocks.listUserProjects.mockRejectedValue(new Error("GitLab unavailable"));
    mocks.listAzureDevOpsProjects.mockResolvedValue({
      projects: [{ id: "project-1", name: "Platform" }],
    });
    mocks.listAzureDevOpsRepositories.mockResolvedValue({
      repositories: [
        {
          id: "repo-1",
          name: "api",
          projectId: "project-1",
          projectName: "Platform",
          webUrl: "https://dev.azure.com/acme/Platform/_git/api",
          defaultBranch: "refs/heads/trunk",
        },
      ],
    });

    const { result } = renderHook(() => useRemoteRepositories("workspace-1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos.map((repo) => `${repo.provider}:${repo.fullName}`)).toEqual([
      "github:acme/web",
      "azure_devops:Platform/api",
    ]);
    expect(result.current.repos[1].defaultBranch).toBe("trunk");
    expect(result.current.unavailable).toBe(false);
  });

  it("does not call an empty connected provider unavailable", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    rejectUnavailableProviders();
    const { result } = renderHook(() => useRemoteRepositories("workspace-1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos).toEqual([]);
    expect(result.current.unavailable).toBe(false);
  });

  it("clears repositories immediately when the workspace changes", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    mocks.listUserProjects.mockRejectedValue(new Error("GitLab not configured"));
    mocks.listAzureDevOpsProjects.mockResolvedValueOnce({
      projects: [{ id: "p1", name: "One" }],
    });
    mocks.listAzureDevOpsRepositories.mockResolvedValueOnce({
      repositories: [
        {
          id: "r1",
          name: "api",
          projectId: "p1",
          projectName: "One",
          webUrl: "https://dev.azure.com/acme/One/_git/api",
          defaultBranch: "refs/heads/main",
        },
      ],
    });
    let resolveNext: ((value: { projects: never[] }) => void) | undefined;
    mocks.listAzureDevOpsProjects.mockImplementationOnce(
      () => new Promise((resolve) => (resolveNext = resolve)),
    );
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useRemoteRepositories(workspaceId),
      { initialProps: { workspaceId: "workspace-1" } },
    );
    await waitFor(() => expect(result.current.repos).toHaveLength(1));

    rerender({ workspaceId: "workspace-2" });
    await waitFor(() => expect(result.current.loading).toBe(true));
    expect(result.current.repos).toEqual([]);

    act(() => resolveNext?.({ projects: [] }));
    await waitFor(() => expect(result.current.loading).toBe(false));
  });
});
