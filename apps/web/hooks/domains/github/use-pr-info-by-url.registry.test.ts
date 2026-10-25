import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { usePRInfoByURL } from "./use-pr-info-by-url";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { RepositoryInspection, RepositoryProviderRegistration } from "@/lib/plugins/types";

const fetchPR = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/github-api", () => ({ fetchPRInfo: fetchPR, fetchIssueInfo: vi.fn() }));

const PLUGIN = "inspection-cache-refresh-test";
const HOST = "https://code.example.test";
const ALPHA = `${HOST}/repos/alpha`;
const BETA = `${HOST}/repos/beta`;
const GAMMA = `${HOST}/repos/gamma`;
const WORKSPACE = "registry-workspace";
const GITHUB_ALPHA = "https://github.com/acme/alpha/pull/11";
const GITHUB_BETA = "https://github.com/acme/beta/pull/22";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN);
  fetchPR.mockReset();
});

function descriptor(url: string): RepositoryInspection {
  const name = url.includes("github.com") ? url.split("/")[4] : url.split("/").at(-1)!;
  return {
    providerId: "cache-test",
    providerHost: HOST,
    ownerOrProject: "acme",
    repositoryId: name,
    repositoryName: name,
    cloneUrl: `${HOST}/acme/${name}.git`,
    headBranch: `${name}-head`,
    baseBranch: `${name}-base`,
    pullRequest: { number: name === "alpha" ? 11 : 22, title: `${name} change` },
  };
}

function register(overrides: Partial<RepositoryProviderRegistration> = {}) {
  const inspectURL = vi.fn(async ({ url }: { url: string }) => descriptor(url));
  pluginRegistry.forPlugin(PLUGIN).registerRepositoryProvider({
    id: "cache-test",
    label: "Cache test",
    listRepositories: async () => [],
    matchesURL: (url) => url.startsWith(HOST),
    inspectURL,
    listBranches: async () => [],
    ...overrides,
  });
  return inspectURL;
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

function pullRequest(number: number, head: string) {
  return {
    number,
    title: `${head} change`,
    head_branch: head,
    base_branch: "main",
    body: "",
    url: "",
    html_url: "",
    state: "open" as const,
    author_login: "acme",
    repo_owner: "acme",
    repo_name: head,
    draft: false,
  };
}

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.10
describe("inspection cache provider refresh", () => {
  it.each([
    [ALPHA, BETA],
    [BETA, ALPHA],
  ])("recovers every cached unsupported URL, ensuring %s before %s", async (first, second) => {
    const { result } = renderHook(() => usePRInfoByURL(WORKSPACE));
    act(() => [ALPHA, BETA].forEach(result.current.ensure));
    expect(result.current.inspection?.(ALPHA)).toBeUndefined();
    expect(result.current.inspection?.(BETA)).toBeUndefined();
    let inspectURL!: ReturnType<typeof register>;
    act(() => {
      inspectURL = register();
    });
    act(() => {
      result.current.ensure(` ${first} `);
      result.current.ensure(second);
      result.current.ensure(first);
    });
    await waitFor(() => {
      expect(result.current.inspection?.(ALPHA)?.repositoryName).toBe("alpha");
      expect(result.current.inspection?.(BETA)?.repositoryName).toBe("beta");
    });
    expect(result.current.info(ALPHA)).toEqual({
      prHeadBranch: "alpha-head",
      prBaseBranch: "alpha-base",
      prNumber: 11,
      suggestedTitle: "PR #11: alpha change",
    });
    expect(result.current.info(BETA)?.prHeadBranch).toBe("beta-head");
    act(() => [ALPHA, BETA].forEach(result.current.ensure));
    expect(inspectURL).toHaveBeenCalledTimes(2);
    act(() => {
      result.current.clear(` ${BETA} `);
      result.current.ensure(BETA);
    });
    await waitFor(() => expect(inspectURL).toHaveBeenCalledTimes(3));
    await waitFor(() => expect(result.current.settled(BETA)).toBe(true));
    expect(result.current.inspection?.(BETA)?.repositoryName).toBe("beta");
  });

  it("rechecks mixed success, no-match and error entries on replacement and removal", async () => {
    register({
      inspectURL: async ({ url }) => {
        if (url === GAMMA) throw new Error("Old inspection failure");
        return url === BETA ? null : { ...descriptor(url), headBranch: "old-alpha" };
      },
    });
    const { result } = renderHook(() => usePRInfoByURL(WORKSPACE));
    act(() => [ALPHA, BETA, GAMMA].forEach(result.current.ensure));
    await waitFor(() =>
      expect(result.current.error(GAMMA)?.message).toBe("Old inspection failure"),
    );
    expect(result.current.info(ALPHA)?.prHeadBranch).toBe("old-alpha");
    expect(result.current.settled(BETA)).toBe(true);
    expect(result.current.inspection?.(BETA)).toBeUndefined();
    act(() => {
      pluginRegistry.unregisterPlugin(PLUGIN);
      register();
    });
    act(() => [BETA, GAMMA, ALPHA].forEach(result.current.ensure));
    await waitFor(() => {
      expect(result.current.info(ALPHA)?.prHeadBranch).toBe("alpha-head");
      expect(result.current.info(BETA)?.prHeadBranch).toBe("beta-head");
      expect(result.current.info(GAMMA)?.prHeadBranch).toBe("gamma-head");
    });
    expect(result.current.error(GAMMA)).toBeUndefined();
    act(() => pluginRegistry.unregisterPlugin(PLUGIN));
    act(() => [ALPHA, BETA, GAMMA].forEach(result.current.ensure));
    for (const url of [ALPHA, BETA, GAMMA]) {
      expect(result.current.inspection?.(url)).toBeUndefined();
      expect(result.current.info(url)).toBeUndefined();
      expect(result.current.error(url)).toBeUndefined();
      expect(result.current.loading(url)).toBe(false);
    }
  });

  it("falls back to built-in PR metadata for both URLs after their provider is removed", async () => {
    register({ matchesURL: () => true });
    fetchPR.mockImplementation(
      async (_workspace: string, _owner: string, repo: string, number: number) =>
        pullRequest(number, `${repo}-builtin`),
    );
    const { result } = renderHook(() => usePRInfoByURL(WORKSPACE));
    act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
    await waitFor(() => expect(result.current.info(GITHUB_BETA)?.prHeadBranch).toBe("beta-head"));
    act(() => pluginRegistry.unregisterPlugin(PLUGIN));
    act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
    await waitFor(() => {
      expect(result.current.info(GITHUB_ALPHA)?.prHeadBranch).toBe("alpha-builtin");
      expect(result.current.info(GITHUB_BETA)?.prHeadBranch).toBe("beta-builtin");
    });
    expect(result.current.inspection?.(GITHUB_ALPHA)).toBeUndefined();
    expect(result.current.inspection?.(GITHUB_BETA)).toBeUndefined();
  });
});

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.11
it("fences old success, error and finalization while both replacement metadata requests are pending", async () => {
  const oldAlpha = deferred<ReturnType<typeof pullRequest>>();
  const oldBeta = deferred<ReturnType<typeof pullRequest>>();
  const newAlpha = deferred<ReturnType<typeof pullRequest>>();
  const newBeta = deferred<ReturnType<typeof pullRequest>>();
  fetchPR
    .mockReturnValueOnce(oldAlpha.promise)
    .mockReturnValueOnce(oldBeta.promise)
    .mockReturnValueOnce(newAlpha.promise)
    .mockReturnValueOnce(newBeta.promise);
  const { result } = renderHook(() => usePRInfoByURL(WORKSPACE));
  act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
  const oldSignals = fetchPR.mock.calls.map((call) => call[4].init.signal as AbortSignal);
  act(() => register({ matchesURL: () => false }));
  act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
  expect(fetchPR).toHaveBeenCalledTimes(4);
  expect(oldSignals.every((signal) => signal.aborted)).toBe(true);
  await act(async () => {
    oldAlpha.resolve(pullRequest(11, "obsolete-alpha"));
    oldBeta.reject(new Error("Obsolete beta failure"));
    await Promise.allSettled([oldAlpha.promise, oldBeta.promise]);
  });
  for (const url of [GITHUB_ALPHA, GITHUB_BETA]) {
    expect(result.current.loading(url)).toBe(true);
    expect(result.current.info(url)).toBeUndefined();
    expect(result.current.error(url)).toBeUndefined();
  }
  act(() => [GITHUB_ALPHA, GITHUB_BETA].forEach(result.current.ensure));
  expect(fetchPR).toHaveBeenCalledTimes(4);
  await act(async () => {
    newAlpha.resolve(pullRequest(11, "current-alpha"));
    newBeta.resolve(pullRequest(22, "current-beta"));
  });
  expect(result.current.info(GITHUB_ALPHA)?.prHeadBranch).toBe("current-alpha");
  expect(result.current.info(GITHUB_BETA)?.prHeadBranch).toBe("current-beta");
});

// @covers AC-PLUGINS-REPOSITORY-TASK-CREATION-001.12
it("keeps structured inspection isolated between hook instances and workspaces", async () => {
  register({
    inspectURL: async ({ url, workspaceId }) => ({ ...descriptor(url), headBranch: workspaceId }),
  });
  const first = renderHook(() => usePRInfoByURL("workspace-a"));
  const second = renderHook(() => usePRInfoByURL("workspace-b"));
  act(() => {
    first.result.current.ensure(ALPHA);
    second.result.current.ensure(ALPHA);
  });
  await waitFor(() => {
    expect(first.result.current.info(ALPHA)?.prHeadBranch).toBe("workspace-a");
    expect(second.result.current.info(ALPHA)?.prHeadBranch).toBe("workspace-b");
  });
  act(() => first.result.current.clear(ALPHA));
  expect(first.result.current.inspection?.(ALPHA)).toBeUndefined();
  expect(second.result.current.inspection?.(ALPHA)?.repositoryName).toBe("alpha");
});
