import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const requestFileTreeMock = vi.fn();
const getWebSocketClientMock = vi.fn(() => ({ on: vi.fn(() => () => {}) }));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClientMock(),
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileTree: (...args: unknown[]) => requestFileTreeMock(...args),
  requestFileContent: vi.fn(),
  searchWorkspaceFiles: vi.fn(),
}));
vi.mock("@/hooks/domains/session/use-session-agentctl", () => ({
  useSessionAgentctl: () => ({ isReady: true }),
}));

import { useFileBrowserTree, loadNodeChildren } from "./file-browser-hooks";
import { FileBrowserTreeCache, type FileTreeCacheBinding } from "./file-browser-tree-cache";

const SESSION = "session-1";
const SUCCESSOR_SESSION = "session-2";
const ENVIRONMENT = "environment-1";
const SUCCESSOR_ENVIRONMENT = "environment-2";
const ROOT_PATH = "";
const CODEX_PATH = ".codex";
const AGENTS_PATH = ".codex/agents";
const CONFIG_PATH = ".codex/agents/config.toml";
const EXPANDED_PATHS = [CODEX_PATH, AGENTS_PATH];
const STORAGE_KEY = `kandev.filesPanel.expanded.${ENVIRONMENT}`;
const SUCCESSOR_STORAGE_KEY = `kandev.filesPanel.expanded.${SUCCESSOR_ENVIRONMENT}`;
const RETRY_DELAYS_MS = [1000, 2000, 5000, 10000];
const ROOT = { name: "", path: ROOT_PATH, is_dir: true, size: 0 };
const CODEX = { name: ".codex", path: CODEX_PATH, is_dir: true, size: 0 };
const AGENTS = { name: "agents", path: AGENTS_PATH, is_dir: true, size: 0 };
const CONFIG = { name: "config.toml", path: CONFIG_PATH, is_dir: false, size: 1 };
const NEW_CONFIG = { name: "config.toml", path: ".new/config.toml", is_dir: false, size: 1 };

function deferred<T>() {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((done) => (resolve = done)), resolve };
}

function mockHydratedTree() {
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
    if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
    if (path === AGENTS_PATH) return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
    throw new Error(`Unexpected path: ${path}`);
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(EXPANDED_PATHS));
  mockHydratedTree();
});

afterEach(() => vi.useRealTimers());

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
it("shows a retained tree before a return navigation refresh responds", async () => {
  const cache = new FileBrowserTreeCache();
  const bindings: Record<string, FileTreeCacheBinding> = {
    [ENVIRONMENT]: { cache, key: ENVIRONMENT, isCurrent: () => true },
    [SUCCESSOR_ENVIRONMENT]: { cache, key: SUCCESSOR_ENVIRONMENT, isCurrent: () => true },
  };
  const { result, rerender } = renderHook(
    ({ key }) => useFileBrowserTree(SESSION, key, bindings[key]),
    {
      initialProps: { key: ENVIRONMENT },
    },
  );
  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  rerender({ key: SUCCESSOR_ENVIRONMENT });
  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  const response = deferred<{ root: typeof ROOT & { children: (typeof CODEX)[] } }>();
  requestFileTreeMock.mockReturnValueOnce(response.promise);
  rerender({ key: ENVIRONMENT });
  expect(result.current.visibleRows.map((row) => row.path)).toContain(CONFIG_PATH);
  expect(result.current.isLoadingTree).toBe(true);
  await act(async () => response.resolve({ root: { ...ROOT, children: [CODEX] } }));
});

it("shares a pending restored folder read with manual expansion", async () => {
  const response = deferred<{ root: typeof CODEX & { children: (typeof AGENTS)[] } }>();
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === "") return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
    if (path === CODEX_PATH) return response.promise;
    return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
  });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));
  await waitFor(() => expect(requestFileTreeMock).toHaveBeenCalledTimes(2));
  let load: Promise<boolean>;
  act(() => {
    load = loadNodeChildren(CODEX, SESSION, result.current);
  });
  expect(requestFileTreeMock).toHaveBeenCalledTimes(2);
  await act(async () => {
    response.resolve({ root: { ...CODEX, children: [AGENTS] } });
    await load;
  });
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4
it("publishes the root and successful sibling before a held folder completes", async () => {
  const held = deferred<{ root: typeof CODEX & { children: (typeof AGENTS)[] } }>();
  const sibling = { ...CODEX, name: ".new", path: ".new" };
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify([CODEX_PATH, ".new"]));
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === "") return Promise.resolve({ root: { ...ROOT, children: [CODEX, sibling] } });
    if (path === CODEX_PATH) return held.promise;
    return Promise.resolve({ root: { ...sibling, children: [NEW_CONFIG] } });
  });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));
  await waitFor(() =>
    expect(requestFileTreeMock.mock.calls.some((call) => call[2] === CODEX_PATH)).toBe(true),
  );
  expect(result.current.tree?.children?.map((node) => node.path)).toEqual([CODEX_PATH, ".new"]);
  await waitFor(() =>
    expect(result.current.visibleRows.some((row) => row.path === NEW_CONFIG.path)).toBe(true),
  );
  expect(result.current.isLoadingTree).toBe(true);
  await act(async () => held.resolve({ root: { ...CODEX, children: [AGENTS] } }));
  expect(result.current.loadState).toBe("loaded");
});

it("restores at most four independent folders concurrently and preserves every merge", async () => {
  const folders = Array.from({ length: 6 }, (_, i) => ({
    ...CODEX,
    name: `dir-${i}`,
    path: `dir-${i}`,
  }));
  const responses = folders.map(() =>
    deferred<{ root: typeof CODEX & { children: (typeof CONFIG)[] } }>(),
  );
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(folders.map((folder) => folder.path)));
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === "") return Promise.resolve({ root: { ...ROOT, children: folders } });
    return responses[folders.findIndex((folder) => folder.path === path)].promise;
  });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));
  await waitFor(() => expect(requestFileTreeMock).toHaveBeenCalledTimes(5));
  for (const index of [2, 0, 1, 3, 4, 5]) {
    await act(async () =>
      responses[index].resolve({
        root: {
          ...folders[index],
          children: [{ ...CONFIG, path: `${folders[index].path}/config.toml` }],
        },
      }),
    );
  }
  expect(result.current.loadState).toBe("loaded");
  expect(result.current.visibleRows.filter((row) => !row.isDir)).toHaveLength(6);
});

it("preserves successful branches and expansion intent after a transient folder failure", async () => {
  vi.useFakeTimers();
  const sibling = { ...CODEX, name: ".new", path: ".new" };
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify([CODEX_PATH, ".new"]));
  let fail = true;
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === "") return Promise.resolve({ root: { ...ROOT, children: [CODEX, sibling] } });
    if (path === CODEX_PATH && fail) return Promise.reject(new Error("temporarily unavailable"));
    return Promise.resolve({
      root: { ...(path === CODEX_PATH ? CODEX : sibling), children: [NEW_CONFIG] },
    });
  });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.visibleRows.some((row) => row.path === NEW_CONFIG.path)).toBe(true);
  expect(result.current.expandedPaths).toEqual(new Set([CODEX_PATH, ".new"]));
  fail = false;
  await act(async () => result.current.loadTree({ resetRetry: true }));
  expect(result.current.loadState).toBe("loaded");
});

it("starts a successor load after a session switch and ignores the stale predecessor", async () => {
  const firstRoot = deferred<{ root: typeof ROOT & { children: (typeof CODEX)[] } }>();
  requestFileTreeMock.mockImplementation((_client: unknown, sessionId: string, path: string) => {
    if (sessionId === SESSION && path === ROOT_PATH) return firstRoot.promise;
    if (sessionId === SUCCESSOR_SESSION && path === ROOT_PATH) {
      return Promise.resolve({
        root: { ...ROOT, children: [{ ...CODEX, name: ".new", path: ".new" }] },
      });
    }
    return Promise.resolve({ root: { ...CODEX, children: [] } });
  });
  const { result, rerender } = renderHook(
    ({ sessionId }) => useFileBrowserTree(sessionId, ENVIRONMENT),
    { initialProps: { sessionId: SESSION } },
  );
  rerender({ sessionId: SUCCESSOR_SESSION });
  firstRoot.resolve({ root: { ...ROOT, children: [CODEX] } });

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));

  expect(requestFileTreeMock.mock.calls.map((call) => [call[1], call[2]])).toContainEqual([
    SUCCESSOR_SESSION,
    ROOT_PATH,
  ]);
  expect(result.current.visibleRows.map((row) => row.path)).toEqual([".new"]);
});

it("replaces a completed session tree with the successor's root and restored expansion", async () => {
  requestFileTreeMock.mockImplementation((_client: unknown, sessionId: string, path: string) => {
    if (sessionId === SESSION && path === ROOT_PATH) {
      return Promise.resolve({
        root: { ...ROOT, children: [{ ...CODEX, name: ".old", path: ".old" }, CODEX] },
      });
    }
    if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
    if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
    if (path === AGENTS_PATH) return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
    throw new Error(`Unexpected successor path: ${path}`);
  });
  const { result, rerender } = renderHook(
    ({ sessionId }) => useFileBrowserTree(sessionId, ENVIRONMENT),
    { initialProps: { sessionId: SESSION } },
  );

  await waitFor(() => expect(result.current.visibleRows.map((row) => row.path)).toContain(".old"));
  rerender({ sessionId: SUCCESSOR_SESSION });

  await waitFor(() =>
    expect(result.current.visibleRows.map((row) => row.path)).toEqual([
      CODEX_PATH,
      AGENTS_PATH,
      CONFIG_PATH,
    ]),
  );

  expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
  expect(sessionStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify(EXPANDED_PATHS));
  expect(result.current.visibleRows.map((row) => row.path)).toEqual([
    CODEX_PATH,
    AGENTS_PATH,
    CONFIG_PATH,
  ]);
  expect(requestFileTreeMock.mock.calls.map((call) => [call[1], call[2]])).toContainEqual([
    SUCCESSOR_SESSION,
    ROOT_PATH,
  ]);
  expect(requestFileTreeMock.mock.calls.map((call) => [call[1], call[2]])).toContainEqual([
    SUCCESSOR_SESSION,
    AGENTS_PATH,
  ]);
});

it("hydrates folders expanded after restore when a same-key session is replaced", async () => {
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === ROOT_PATH)
      return Promise.resolve({
        root: { ...ROOT, children: [CODEX, { ...CODEX, name: ".new", path: ".new" }] },
      });
    if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
    if (path === AGENTS_PATH) return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
    if (path === ".new")
      return Promise.resolve({
        root: { ...CODEX, name: ".new", path: ".new", children: [NEW_CONFIG] },
      });
    throw new Error(`Unexpected path: ${path}`);
  });
  const { result, rerender } = renderHook(
    ({ sessionId }) => useFileBrowserTree(sessionId, ENVIRONMENT),
    { initialProps: { sessionId: SESSION } },
  );

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  act(() => result.current.setExpandedPaths((paths) => new Set(paths).add(".new")));
  rerender({ sessionId: SUCCESSOR_SESSION });

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));

  expect(requestFileTreeMock.mock.calls.map((call) => [call[1], call[2]])).toContainEqual([
    SUCCESSOR_SESSION,
    ".new",
  ]);
  expect(result.current.visibleRows.map((row) => row.path)).toContain(NEW_CONFIG.path);
});

it("invalidates an in-flight request when the reset key changes", async () => {
  const firstRoot = deferred<{ root: typeof ROOT & { children: (typeof CODEX)[] } }>();
  requestFileTreeMock.mockImplementationOnce(() => firstRoot.promise);
  mockHydratedTree();
  sessionStorage.setItem(`kandev.filesPanel.expanded.${SUCCESSOR_ENVIRONMENT}`, "[]");
  const { result, rerender } = renderHook(({ resetKey }) => useFileBrowserTree(SESSION, resetKey), {
    initialProps: { resetKey: ENVIRONMENT },
  });
  rerender({ resetKey: SUCCESSOR_ENVIRONMENT });
  firstRoot.resolve({ root: { ...ROOT, children: [CODEX] } });

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  expect(result.current.visibleRows.map((row) => row.path)).toEqual([CODEX_PATH]);
});

it("replaces a completed reset-key tree with the new key's root and saved expansion", async () => {
  sessionStorage.setItem(SUCCESSOR_STORAGE_KEY, JSON.stringify([".new"]));
  let rootRequests = 0;
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === ROOT_PATH) {
      rootRequests += 1;
      return Promise.resolve(
        rootRequests === 1
          ? { root: { ...ROOT, children: [CODEX] } }
          : { root: { ...ROOT, children: [{ ...CODEX, name: ".new", path: ".new" }] } },
      );
    }
    if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
    if (path === AGENTS_PATH) return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
    if (path === ".new")
      return Promise.resolve({
        root: { ...CODEX, name: ".new", path: ".new", children: [NEW_CONFIG] },
      });
    throw new Error(`Unexpected path: ${path}`);
  });
  const { result, rerender } = renderHook(({ resetKey }) => useFileBrowserTree(SESSION, resetKey), {
    initialProps: { resetKey: ENVIRONMENT },
  });

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  rerender({ resetKey: SUCCESSOR_ENVIRONMENT });

  await waitFor(() => expect(rootRequests).toBe(2));
  await waitFor(() =>
    expect(requestFileTreeMock.mock.calls.map((call) => call[2])).toEqual([
      ROOT_PATH,
      CODEX_PATH,
      AGENTS_PATH,
      ROOT_PATH,
      ".new",
    ]),
  );
  await waitFor(() => expect(result.current.expandedPaths).toEqual(new Set([".new"])));

  expect(sessionStorage.getItem(SUCCESSOR_STORAGE_KEY)).toBe(JSON.stringify([".new"]));
  expect(result.current.visibleRows.map((row) => row.path)).toEqual([".new", NEW_CONFIG.path]);
});

it("does not persist the previous reset key's expansion under a new key", async () => {
  const successorStorageKey = `kandev.filesPanel.expanded.${SUCCESSOR_ENVIRONMENT}`;
  const setItemSpy = vi.spyOn(Storage.prototype, "setItem");
  sessionStorage.setItem(successorStorageKey, JSON.stringify([".new"]));
  const { result, rerender } = renderHook(({ resetKey }) => useFileBrowserTree(SESSION, resetKey), {
    initialProps: { resetKey: ENVIRONMENT },
  });
  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  setItemSpy.mockClear();

  rerender({ resetKey: SUCCESSOR_ENVIRONMENT });
  await waitFor(() => expect(result.current.loadState).toBe("loaded"));

  expect(setItemSpy).not.toHaveBeenCalledWith(successorStorageKey, JSON.stringify(EXPANDED_PATHS));
  setItemSpy.mockRestore();
});

it("recovers from manual retry after all automatic root retries preserve saved expansion", async () => {
  vi.useFakeTimers();
  let rootAttempts = 0;
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === ROOT_PATH && rootAttempts++ <= RETRY_DELAYS_MS.length) {
      return Promise.reject(new Error("root unavailable"));
    }
    if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
    if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
    if (path === AGENTS_PATH) return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
    throw new Error(`Unexpected path: ${path}`);
  });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

  for (const delay of RETRY_DELAYS_MS) {
    await act(async () => vi.advanceTimersByTimeAsync(delay));
  }
  expect(result.current.loadState).toBe("manual");
  expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));

  await act(async () => result.current.loadTree({ resetRetry: true }));

  expect(result.current.loadState).toBe("loaded");
  expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
  expect(result.current.visibleRows.map((row) => row.path)).toEqual([
    CODEX_PATH,
    AGENTS_PATH,
    CONFIG_PATH,
  ]);
  expect(rootAttempts).toBe(RETRY_DELAYS_MS.length + 2);
});

it("deduplicates concurrent load requests for one generation", async () => {
  const root = deferred<{ root: typeof ROOT & { children: (typeof CODEX)[] } }>();
  mockHydratedTree();
  requestFileTreeMock.mockImplementationOnce(() => root.promise);
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

  const duplicate = result.current.loadTree();
  expect(requestFileTreeMock).toHaveBeenCalledTimes(1);
  root.resolve({ root: { ...ROOT, children: [CODEX] } });
  await duplicate;
  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
});

it("retains restored paths while an automatic root retry recovers", async () => {
  vi.useFakeTimers();
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(EXPANDED_PATHS));
  requestFileTreeMock.mockRejectedValueOnce(new Error("root unavailable"));
  mockHydratedTree();
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

  await act(async () => Promise.resolve());
  expect(result.current.loadState).toBe("waiting");
  expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));

  await act(async () => vi.advanceTimersByTimeAsync(1000));
  expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
  expect(result.current.loadState).toBe("loaded");
});

it("does not let a scheduled predecessor retry request for a replacement owner", async () => {
  vi.useFakeTimers();
  const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout").mockImplementation(() => undefined);
  requestFileTreeMock.mockRejectedValueOnce(new Error("root unavailable"));
  mockHydratedTree();
  const { result, rerender } = renderHook(
    ({ sessionId }) => useFileBrowserTree(sessionId, ENVIRONMENT),
    { initialProps: { sessionId: SESSION } },
  );

  await act(async () => Promise.resolve());
  rerender({ sessionId: SUCCESSOR_SESSION });
  await act(async () => vi.advanceTimersByTimeAsync(1000));

  expect(
    requestFileTreeMock.mock.calls.filter(
      (call) => call[1] === SUCCESSOR_SESSION && call[2] === ROOT_PATH,
    ),
  ).toHaveLength(1);
  expect(result.current.loadState).toBe("loaded");
  clearTimeoutSpy.mockRestore();
});

it("prunes persisted folders when the restored root is null", async () => {
  requestFileTreeMock.mockResolvedValue({ root: null });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  expect(result.current.expandedPaths).toEqual(new Set());
  await waitFor(() => expect(sessionStorage.getItem(STORAGE_KEY)).toBe("[]"));
});

it("does not request children for empty persisted paths", async () => {
  sessionStorage.setItem(STORAGE_KEY, "[]");
  requestFileTreeMock.mockResolvedValue({ root: { ...ROOT, children: [CODEX] } });
  const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

  await waitFor(() => expect(result.current.loadState).toBe("loaded"));
  expect(requestFileTreeMock.mock.calls.map((call) => call[2])).toEqual([ROOT_PATH]);
  expect(result.current.expandedPaths).toEqual(new Set());
});
