import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { useBranchesByURL } from "./use-branches-by-url";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { RepositoryInspection, RepositoryProviderRegistration } from "@/lib/plugins/types";

const fetchBranches = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/github-api", () => ({ fetchRepoBranches: fetchBranches }));
vi.mock("@/lib/api/domains/gitlab-api", () => ({ listProjectBranches: vi.fn() }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({ listAzureDevOpsBranches: vi.fn() }));

const PLUGIN = "branch-cache-refresh-test";
const HOST = "https://code.example.test";
const ALPHA = `${HOST}/repos/alpha`;
const BETA = `${HOST}/repos/beta`;
const GAMMA = `${HOST}/repos/gamma`;
const WORKSPACE = "registry-workspace";
const GITHUB_ALPHA = "https://github.com/acme/alpha";
const GITHUB_BETA = "https://github.com/acme/beta";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN);
  fetchBranches.mockReset();
});

function descriptor(url: string): RepositoryInspection {
  const name = url.split("/").at(-1)!;
  return {
    providerId: "cache-test",
    providerHost: HOST,
    ownerOrProject: "acme",
    repositoryId: name,
    repositoryName: name,
    cloneUrl: `${HOST}/acme/${name}.git`,
  };
}

function register(overrides: Partial<RepositoryProviderRegistration> = {}) {
  const listBranches = vi.fn(async ({ repository }: { repository: RepositoryInspection }) => [
    { name: `${repository.repositoryName}-main` },
  ]);
  pluginRegistry.forPlugin(PLUGIN).registerRepositoryProvider({
    id: "cache-test",
    label: "Cache test",
    listRepositories: async () => [],
    matchesURL: (url) => url.startsWith(HOST),
    inspectURL: async ({ url }) => descriptor(url),
    listBranches,
    ...overrides,
  });
  return listBranches;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.10
describe("branch cache provider refresh", () => {
  it.each([
    [ALPHA, BETA],
    [BETA, ALPHA],
  ])("recovers every cached unsupported URL, ensuring %s before %s", async (first, second) => {
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE));
    act(() => {
      result.current.ensure(ALPHA);
      result.current.ensure(BETA);
    });
    expect(result.current.branches(ALPHA)).toEqual([]);
    expect(result.current.branches(BETA)).toEqual([]);
    let listBranches!: ReturnType<typeof register>;
    act(() => {
      listBranches = register();
    });
    act(() => {
      result.current.ensure(` ${first} `);
      result.current.ensure(second);
      result.current.ensure(first);
    });
    await waitFor(() => {
      expect(result.current.branches(ALPHA)).toEqual([{ name: "alpha-main", type: "remote" }]);
      expect(result.current.branches(BETA)).toEqual([{ name: "beta-main", type: "remote" }]);
    });
    act(() => {
      result.current.ensure(ALPHA);
      result.current.ensure(BETA);
    });
    expect(listBranches).toHaveBeenCalledTimes(2);
    act(() => {
      result.current.clear(` ${BETA} `);
      result.current.ensure(BETA);
    });
    await waitFor(() => expect(listBranches).toHaveBeenCalledTimes(3));
    await waitFor(() => expect(result.current.loading(BETA)).toBe(false));
    expect(result.current.branches(BETA)[0]?.name).toBe("beta-main");
  });

  it("rechecks mixed success, empty and error entries on replacement and removal", async () => {
    register({
      listBranches: async ({ repository }) => {
        if (repository.repositoryName === "gamma") throw new Error("Old branch failure");
        return repository.repositoryName === "beta" ? [] : [{ name: "old-alpha" }];
      },
    });
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE));
    act(() => [ALPHA, BETA, GAMMA].forEach(result.current.ensure));
    await waitFor(() => expect(result.current.error(GAMMA)?.message).toBe("Old branch failure"));
    expect(result.current.branches(ALPHA)[0]?.name).toBe("old-alpha");
    expect(result.current.branches(BETA)).toEqual([]);
    act(() => {
      pluginRegistry.unregisterPlugin(PLUGIN);
      register();
    });
    act(() => [BETA, GAMMA, ALPHA].forEach(result.current.ensure));
    await waitFor(() => {
      expect(result.current.branches(ALPHA)[0]?.name).toBe("alpha-main");
      expect(result.current.branches(BETA)[0]?.name).toBe("beta-main");
      expect(result.current.branches(GAMMA)[0]?.name).toBe("gamma-main");
    });
    expect(result.current.error(GAMMA)).toBeUndefined();
    act(() => pluginRegistry.unregisterPlugin(PLUGIN));
    act(() => [ALPHA, BETA, GAMMA].forEach(result.current.ensure));
    for (const url of [ALPHA, BETA, GAMMA]) {
      expect(result.current.branches(url)).toEqual([]);
      expect(result.current.error(url)).toBeUndefined();
      expect(result.current.loading(url)).toBe(false);
    }
  });

  it("falls back to built-in branches for both URLs after their provider is removed", async () => {
    register({ matchesURL: () => true });
    fetchBranches.mockImplementation(async (_workspace: string, _owner: string, repo: string) => ({
      branches: [{ name: `${repo}-builtin` }],
    }));
    const { result } = renderHook(() => useBranchesByURL(WORKSPACE));
    act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
    await waitFor(() => expect(result.current.branches(GITHUB_BETA)[0]?.name).toBe("beta-main"));
    act(() => pluginRegistry.unregisterPlugin(PLUGIN));
    act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
    await waitFor(() => {
      expect(result.current.branches(GITHUB_ALPHA)[0]?.name).toBe("alpha-builtin");
      expect(result.current.branches(GITHUB_BETA)[0]?.name).toBe("beta-builtin");
    });
  });
});

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.11
it("fences old success, error and finalization while both replacement requests are pending", async () => {
  type Response = { branches: Array<{ name: string }> };
  const oldAlpha = deferred<Response>();
  const oldBeta = deferred<Response>();
  const newAlpha = deferred<Response>();
  const newBeta = deferred<Response>();
  fetchBranches
    .mockReturnValueOnce(oldAlpha.promise)
    .mockReturnValueOnce(oldBeta.promise)
    .mockReturnValueOnce(newAlpha.promise)
    .mockReturnValueOnce(newBeta.promise);
  const { result } = renderHook(() => useBranchesByURL(WORKSPACE));
  act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
  const oldSignals = fetchBranches.mock.calls.map((call) => call[3].init.signal as AbortSignal);
  act(() => register({ matchesURL: () => false }));
  act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
  expect(fetchBranches).toHaveBeenCalledTimes(4);
  expect(oldSignals.every((signal) => signal.aborted)).toBe(true);
  await act(async () => {
    oldAlpha.resolve({ branches: [{ name: "obsolete-alpha" }] });
    oldBeta.reject(new Error("Obsolete beta failure"));
    await Promise.allSettled([oldAlpha.promise, oldBeta.promise]);
  });
  for (const url of [GITHUB_ALPHA, GITHUB_BETA]) {
    expect(result.current.loading(url)).toBe(true);
    expect(result.current.branches(url)).toEqual([]);
    expect(result.current.error(url)).toBeUndefined();
  }
  act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
  expect(fetchBranches).toHaveBeenCalledTimes(4);
  await act(async () => {
    newAlpha.resolve({ branches: [{ name: "current-alpha" }] });
    newBeta.resolve({ branches: [{ name: "current-beta" }] });
  });
  expect(result.current.branches(GITHUB_ALPHA)[0]?.name).toBe("current-alpha");
  expect(result.current.branches(GITHUB_BETA)[0]?.name).toBe("current-beta");
});

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.12
it("keeps registered-provider caches isolated between hook instances and workspaces", async () => {
  register({ listBranches: async ({ workspaceId }) => [{ name: workspaceId }] });
  const first = renderHook(() => useBranchesByURL("workspace-a"));
  const second = renderHook(() => useBranchesByURL("workspace-b"));
  act(() => {
    first.result.current.ensure(ALPHA);
    second.result.current.ensure(ALPHA);
  });
  await waitFor(() => {
    expect(first.result.current.branches(ALPHA)[0]?.name).toBe("workspace-a");
    expect(second.result.current.branches(ALPHA)[0]?.name).toBe("workspace-b");
  });
  act(() => first.result.current.clear(ALPHA));
  expect(first.result.current.branches(ALPHA)).toEqual([]);
  expect(second.result.current.branches(ALPHA)[0]?.name).toBe("workspace-b");
});
