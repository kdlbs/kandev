import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchAccessibleRepos: vi.fn(),
  listUserProjects: vi.fn(),
  useGitHubStatus: vi.fn(),
  useGitLabStatus: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchAccessibleRepos: mocks.fetchAccessibleRepos,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({ listUserProjects: mocks.listUserProjects }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: mocks.useGitHubStatus,
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-status", () => ({
  useGitLabStatus: mocks.useGitLabStatus,
}));

import { useRemoteRepositories } from "./use-remote-repositories";
import { pluginRegistry } from "@/lib/plugins/registry";

const WORKSPACE_ID = "workspace-1";
const NEXT_WORKSPACE_ID = "workspace-2";
const PLUGIN_ID = "test-bitbucket-provider";
const GITLAB_UNAVAILABLE = "GitLab unavailable";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  vi.resetAllMocks();
});

function setBuiltInAvailability({
  github = true,
  gitlab = true,
}: {
  github?: boolean;
  gitlab?: boolean;
} = {}) {
  mocks.useGitHubStatus.mockReturnValue({
    status: github ? { authenticated: true, token_configured: true } : null,
    loaded: true,
    loading: false,
    refresh: vi.fn(),
  });
  mocks.useGitLabStatus.mockReturnValue({
    status: gitlab ? { authenticated: true, token_configured: true } : null,
    loading: false,
    refresh: vi.fn(),
  });
}

beforeEach(() => setBuiltInAvailability());

function rejectUnavailableProviders() {
  mocks.listUserProjects.mockRejectedValue(new Error("GitLab not configured"));
}

function expectWorkspaceScopedGitHubRequest() {
  expect(mocks.fetchAccessibleRepos).toHaveBeenCalledWith({
    workspaceId: WORKSPACE_ID,
    limit: 100,
  });
}

const githubRepo = (name: string) => ({
  owner: "acme",
  name,
  full_name: `acme/${name}`,
  default_branch: "main",
  private: false,
});

describe("useRemoteRepositories registered providers", () => {
  it("isolates a provider URL matcher failure", async () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      matchesURL: () => {
        throw new Error("matcher crashed");
      },
      listBranches: async () => [],
      inspectURL: async () => null,
      listRepositories: async () => [],
    });
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    mocks.listUserProjects.mockResolvedValue({ projects: [] });
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(() => result.current.matchesURL?.("not-a-repository-url")).not.toThrow();
    expect(result.current.matchesURL?.("not-a-repository-url")).toBe(false);
  });
});

describe("useRemoteRepositories registered provider listings", () => {
  it("lists registered provider repositories with their exact clone URL", async () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
      listRepositories: async () => [
        {
          providerId: "bitbucket",
          providerHost: "https://bitbucket.example.test/bitbucket",
          ownerOrProject: "PLATFORM",
          repositoryId: "web-42",
          repositoryName: "web",
          cloneUrl: "https://bitbucket.example.test/bitbucket/scm/PLATFORM/web.git",
          defaultBranch: "main",
        },
      ],
    });
    mocks.fetchAccessibleRepos.mockRejectedValue(new Error("GitHub not configured"));
    rejectUnavailableProviders();
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos).toEqual([
      expect.objectContaining({
        provider: "bitbucket",
        id: "web-42",
        fullName: "PLATFORM/web",
        url: "https://bitbucket.example.test/bitbucket/scm/PLATFORM/web.git",
        defaultBranch: "main",
      }),
    ]);
    expect(result.current.availableProviders).toEqual(["bitbucket"]);
  });

  it("follows provider cursors and forwards server-side search", async () => {
    const listRepositories = vi.fn(
      async ({ cursor, query: _query }: { cursor?: string; query?: string }) =>
        cursor === "page-2"
          ? {
              repositories: [
                {
                  providerId: "bitbucket",
                  providerHost: "https://bitbucket.org",
                  ownerOrProject: "acme",
                  repositoryId: "two",
                  repositoryName: "two",
                  cloneUrl: "https://bitbucket.org/acme/two.git",
                },
              ],
            }
          : {
              repositories: [
                {
                  providerId: "bitbucket",
                  providerHost: "https://bitbucket.org",
                  ownerOrProject: "acme",
                  repositoryId: "one",
                  repositoryName: "one",
                  cloneUrl: "https://bitbucket.org/acme/one.git",
                },
              ],
              nextCursor: "page-2",
            },
    );
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
      listRepositories,
    });
    mocks.fetchAccessibleRepos.mockRejectedValue(new Error("GitHub not configured"));
    rejectUnavailableProviders();
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.repos).toHaveLength(2));
    act(() => {
      result.current.search("t");
      result.current.search("tw");
      result.current.search("two");
    });
    await waitFor(() =>
      expect(listRepositories).toHaveBeenCalledWith(
        expect.objectContaining({ query: "two", limit: 100 }),
      ),
    );
    expect(listRepositories).toHaveBeenCalledWith(
      expect.objectContaining({ cursor: "page-2", query: "two" }),
    );
    expect(listRepositories).not.toHaveBeenCalledWith(expect.objectContaining({ query: "t" }));
    expect(listRepositories).not.toHaveBeenCalledWith(expect.objectContaining({ query: "tw" }));
    expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(1);
    expect(mocks.listUserProjects).toHaveBeenCalledTimes(1);
  });
});

describe("useRemoteRepositories provider eligibility", () => {
  it("waits for provider eligibility before listing repositories", async () => {
    setBuiltInAvailability({ github: false, gitlab: false });
    mocks.useGitHubStatus.mockReturnValue({
      status: null,
      loaded: false,
      loading: true,
      refresh: vi.fn(),
    });
    mocks.fetchAccessibleRepos.mockResolvedValue([]);

    const { result, rerender } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    expect(result.current.loading).toBe(true);
    expect(mocks.fetchAccessibleRepos).not.toHaveBeenCalled();
    expect(mocks.listUserProjects).not.toHaveBeenCalled();

    setBuiltInAvailability({ gitlab: false });
    rerender();

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(1);
    expect(mocks.listUserProjects).not.toHaveBeenCalled();
  });

  it("does not request or report unconfigured built-in providers", async () => {
    setBuiltInAvailability({ gitlab: false });
    mocks.fetchAccessibleRepos.mockResolvedValue([
      {
        owner: "acme",
        name: "web",
        full_name: "acme/web",
        default_branch: "main",
        private: false,
      },
    ]);
    mocks.listUserProjects.mockRejectedValue(new Error("GitLab not configured"));

    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos.map((repo) => repo.fullName)).toEqual(["acme/web"]);
    expect(result.current.availableProviders).toEqual(["github"]);
    expect(result.current.sourceErrors).toEqual([]);
    expect(mocks.listUserProjects).not.toHaveBeenCalled();
  });

  it("keeps an eligible provider failure visible beside successful results", async () => {
    setBuiltInAvailability();
    mocks.fetchAccessibleRepos.mockResolvedValue([
      {
        owner: "acme",
        name: "web",
        full_name: "acme/web",
        default_branch: "main",
        private: false,
      },
    ]);
    mocks.listUserProjects.mockRejectedValue(new Error(GITLAB_UNAVAILABLE));

    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos.map((repo) => repo.fullName)).toEqual(["acme/web"]);
    expect(result.current.availableProviders).toEqual(["github"]);
    expect(result.current.sourceErrors).toEqual([
      { provider: "gitlab", error: new Error(GITLAB_UNAVAILABLE) },
    ]);
  });
});

describe("useRemoteRepositories provider results", () => {
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
    mocks.listUserProjects.mockRejectedValue(new Error(GITLAB_UNAVAILABLE));
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expectWorkspaceScopedGitHubRequest();
    expect(result.current.repos.map((repo) => `${repo.provider}:${repo.fullName}`)).toEqual([
      "github:acme/web",
    ]);
    expect(result.current.availableProviders).toEqual(["github"]);
    expect(result.current.error).toEqual(new Error(GITLAB_UNAVAILABLE));
    expect(result.current.sourceErrors).toEqual([
      { provider: "gitlab", error: new Error(GITLAB_UNAVAILABLE) },
    ]);
    expect(result.current.unavailable).toBe(false);
  });

  it("does not call an empty connected provider unavailable", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    rejectUnavailableProviders();
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos).toEqual([]);
    expect(result.current.availableProviders).toEqual(["github"]);
    expect(result.current.unavailable).toBe(false);
  });

  it("reports every provider whose repository request succeeds", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    mocks.listUserProjects.mockResolvedValue({ projects: [] });

    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.availableProviders).toEqual(["github", "gitlab"]);
  });
});

describe("useRemoteRepositories provider eligibility changes", () => {
  it("uses current provider eligibility after a workspace change", async () => {
    mocks.useGitHubStatus.mockImplementation((workspaceId: string) => ({
      status: workspaceId === WORKSPACE_ID ? { authenticated: true, token_configured: true } : null,
      loaded: true,
      loading: false,
      refresh: vi.fn(),
    }));
    mocks.useGitLabStatus.mockImplementation((workspaceId: string) => ({
      status: workspaceId === WORKSPACE_ID ? null : { authenticated: true, token_configured: true },
      loading: false,
      refresh: vi.fn(),
    }));
    mocks.fetchAccessibleRepos.mockResolvedValue([
      {
        owner: "acme",
        name: "web",
        full_name: "acme/web",
        default_branch: "main",
        private: false,
      },
    ]);
    mocks.listUserProjects.mockResolvedValue({
      projects: [
        {
          id: 42,
          namespace: "acme",
          path: "worker",
          path_with_namespace: "acme/worker",
          web_url: "https://gitlab.com/acme/worker",
          default_branch: "main",
          visibility: "private",
        },
      ],
    });
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useRemoteRepositories(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    await waitFor(() => expect(result.current.repos).toHaveLength(1));
    expect(result.current.repos[0]?.provider).toBe("github");
    expect(mocks.listUserProjects).not.toHaveBeenCalled();

    rerender({ workspaceId: NEXT_WORKSPACE_ID });

    await waitFor(() => expect(result.current.repos[0]?.provider).toBe("gitlab"));
    expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(1);
    expect(mocks.listUserProjects).toHaveBeenCalledTimes(1);
  });
});

describe("useRemoteRepositories provider refreshes", () => {
  it("re-evaluates provider eligibility before a manual refresh", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    mocks.listUserProjects.mockResolvedValue({ projects: [] });
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(1);
    expect(mocks.listUserProjects).toHaveBeenCalledTimes(1);

    setBuiltInAvailability({ gitlab: false });
    act(() => result.current.refresh?.());

    await waitFor(() => {
      expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2);
      expect(result.current.availableProviders).toEqual(["github"]);
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.listUserProjects).toHaveBeenCalledTimes(1);
  });

  it("keeps currently eligible providers available while a refresh is loading", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    mocks.listUserProjects.mockResolvedValue({ projects: [] });
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    let resolveRefresh: ((repos: never[]) => void) | undefined;
    mocks.fetchAccessibleRepos.mockImplementationOnce(
      () => new Promise((resolve) => (resolveRefresh = resolve)),
    );
    setBuiltInAvailability({ gitlab: false });

    act(() => result.current.refresh?.());

    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2));
    expect(result.current.availableProviders).toEqual(["github"]);
    expect(result.current.repos).toEqual([]);

    act(() => resolveRefresh?.([]));
    await waitFor(() => expect(result.current.loading).toBe(false));
  });

  it("clears a provider error while its refresh retry is loading", async () => {
    setBuiltInAvailability({ gitlab: false });
    mocks.fetchAccessibleRepos.mockRejectedValueOnce(new Error("GitHub unavailable"));
    const { result } = renderHook(() => useRemoteRepositories(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.sourceErrors).toEqual([
      { provider: "github", error: new Error("GitHub unavailable") },
    ]);

    let resolveRetry: ((repos: never[]) => void) | undefined;
    mocks.fetchAccessibleRepos.mockImplementationOnce(
      () => new Promise((resolve) => (resolveRetry = resolve)),
    );

    act(() => result.current.refresh?.());

    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2));
    expect(result.current.loading).toBe(true);
    expect(result.current.sourceErrors).toEqual([]);

    act(() => resolveRetry?.([]));
    await waitFor(() => expect(result.current.loading).toBe(false));
  });
});

describe("useRemoteRepositories workspace scope", () => {
  it("does not invoke plugin providers without an active workspace", async () => {
    const listRepositories = vi.fn().mockResolvedValue([]);
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
      listRepositories,
    });

    const { result } = renderHook(() => useRemoteRepositories(""));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(listRepositories).not.toHaveBeenCalled();
  });

  it("clears repositories immediately when the workspace changes", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    mocks.listUserProjects.mockResolvedValueOnce({
      projects: [
        {
          id: 1,
          namespace: "acme",
          path: "api",
          path_with_namespace: "acme/api",
          web_url: "https://gitlab.com/acme/api",
          default_branch: "main",
          visibility: "private",
        },
      ],
    });
    let resolveNext: ((value: { projects: never[] }) => void) | undefined;
    mocks.listUserProjects.mockImplementationOnce(
      () => new Promise((resolve) => (resolveNext = resolve)),
    );
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useRemoteRepositories(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );
    await waitFor(() => expect(result.current.repos).toHaveLength(1));

    rerender({ workspaceId: NEXT_WORKSPACE_ID });
    await waitFor(() => expect(result.current.loading).toBe(true));
    expect(result.current.repos).toEqual([]);

    act(() => resolveNext?.({ projects: [] }));
    await waitFor(() => expect(result.current.loading).toBe(false));
  });

  it("does not publish a stale repository result after a workspace change", async () => {
    setBuiltInAvailability({ gitlab: false });
    let resolveFirst: ((value: unknown) => void) | undefined;
    let resolveSecond: ((value: unknown) => void) | undefined;
    mocks.fetchAccessibleRepos
      .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
      .mockImplementationOnce(() => new Promise((resolve) => (resolveSecond = resolve)));
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useRemoteRepositories(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(1));
    rerender({ workspaceId: NEXT_WORKSPACE_ID });
    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2));

    await act(async () => resolveFirst?.([githubRepo("stale")]));
    expect(result.current.repos).toEqual([]);

    await act(async () => resolveSecond?.([githubRepo("current")]));
    await waitFor(() =>
      expect(result.current.repos.map((repo) => repo.fullName)).toEqual(["acme/current"]),
    );
  });

  it("ignores a result resolved before the next workspace request starts", async () => {
    setBuiltInAvailability({ gitlab: false });
    let resolveFirst: ((value: unknown) => void) | undefined;
    mocks.fetchAccessibleRepos
      .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
      .mockImplementationOnce(() => new Promise(() => {}));
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useRemoteRepositories(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(1));
    await act(async () => {
      resolveFirst?.([githubRepo("stale")]);
      rerender({ workspaceId: NEXT_WORKSPACE_ID });
    });
    expect(result.current.repos).toEqual([]);
    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2));
  });
});
