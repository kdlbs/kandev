import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

import { loadNodeChildren, useFileBrowserTree } from "./file-browser-hooks";

const ROOT_PATH = "";
const CODEX_PATH = ".codex";
const AGENTS_PATH = `${CODEX_PATH}/agents`;
const CONFIG_PATH = `${AGENTS_PATH}/config.toml`;
const EXPANDED_PATHS = [CODEX_PATH, AGENTS_PATH];
const ROOT = { name: "", path: ROOT_PATH, is_dir: true, size: 0 };
const CODEX = { name: CODEX_PATH, path: CODEX_PATH, is_dir: true, size: 0 };
const AGENTS = { name: "agents", path: AGENTS_PATH, is_dir: true, size: 0 };
const CONFIG = { name: "config.toml", path: CONFIG_PATH, is_dir: false, size: 1 };
const SESSION = "session-1";
const ENVIRONMENT = "environment-1";
const STORAGE_KEY = `kandev.filesPanel.expanded.${ENVIRONMENT}`;

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(EXPANDED_PATHS));
  requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
    if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
    if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
    if (path === AGENTS_PATH) {
      return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
    }
    throw new Error(`Unexpected path: ${path}`);
  });
});

afterEach(() => vi.useRealTimers());

describe("useFileBrowserTree persisted expansion", () => {
  it("hydrates every persisted expanded ancestor before marking the tree loaded", async () => {
    const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

    await waitFor(() => expect(result.current.loadState).toBe("loaded"));

    expect(requestFileTreeMock.mock.calls.map((call) => call[2])).toEqual([
      ROOT_PATH,
      ...EXPANDED_PATHS,
    ]);
    expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
    expect(result.current.visibleRows.map((row) => row.path)).toEqual([
      CODEX_PATH,
      AGENTS_PATH,
      CONFIG_PATH,
    ]);
  });

  it.each([
    ["deep-only", [AGENTS_PATH]],
    ["deep-first", [AGENTS_PATH, CODEX_PATH]],
  ])("synthesizes ancestor expansion for %s saved paths", async (_label, savedPaths) => {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(savedPaths));
    const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

    await waitFor(() => expect(result.current.loadState).toBe("loaded"));

    expect(requestFileTreeMock.mock.calls.map((call) => call[2])).toEqual([
      ROOT_PATH,
      ...EXPANDED_PATHS,
    ]);
    expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
  });

  it("discards invalid persisted paths while restoring valid siblings", async () => {
    sessionStorage.setItem(
      STORAGE_KEY,
      JSON.stringify([CODEX_PATH, AGENTS_PATH, "/home/jcfs/project/public/assets"]),
    );
    const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));

    await waitFor(() => expect(result.current.loadState).toBe("loaded"));

    expect(requestFileTreeMock.mock.calls.map((call) => call[2])).toEqual([
      ROOT_PATH,
      ...EXPANDED_PATHS,
    ]);
    expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
  });

  it("retries a transient restored-folder failure without pruning persisted expansion", async () => {
    vi.useFakeTimers();
    let codexAttempts = 0;
    requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
      if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
      if (path === CODEX_PATH && codexAttempts++ === 0) {
        return Promise.reject(new Error("folder unavailable"));
      }
      if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
      if (path === AGENTS_PATH) return Promise.resolve({ root: { ...AGENTS, children: [CONFIG] } });
      throw new Error(`Unexpected path: ${path}`);
    });

    const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));
    await act(async () => Promise.resolve());

    expect(result.current.loadState).toBe("waiting");
    expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
    expect(sessionStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify(EXPANDED_PATHS));

    await act(async () => vi.advanceTimersByTimeAsync(1000));

    expect(result.current.loadState).toBe("loaded");
    expect(result.current.expandedPaths).toEqual(new Set(EXPANDED_PATHS));
    expect(result.current.visibleRows.map((row) => row.path)).toEqual([
      CODEX_PATH,
      AGENTS_PATH,
      CONFIG_PATH,
    ]);
  });

  it("prunes an expanded folder and descendants when its restore response is null", async () => {
    requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
      if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
      if (path === CODEX_PATH) return Promise.resolve({ root: null });
      throw new Error(`Unexpected path: ${path}`);
    });

    const { result } = renderHook(() => useFileBrowserTree(SESSION, ENVIRONMENT));
    await waitFor(() => expect(result.current.loadState).toBe("loaded"));

    expect(result.current.expandedPaths).toEqual(new Set());
    expect(result.current.visibleRows.map((row) => row.path)).toEqual([CODEX_PATH]);
    expect(sessionStorage.getItem(STORAGE_KEY)).toBe("[]");
  });
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
it.each(["empty", "missing"])(
  "discards retained descendants after an authoritative %s folder response",
  async (responseKind) => {
    const { result, rerender } = renderHook(
      ({ sessionId }) => useFileBrowserTree(sessionId, ENVIRONMENT),
      { initialProps: { sessionId: SESSION } },
    );
    await waitFor(() => expect(result.current.loadState).toBe("loaded"));
    expect(result.current.visibleRows.map((row) => row.path)).toContain(CONFIG_PATH);
    requestFileTreeMock.mockImplementation((_client: unknown, _sessionId: string, path: string) => {
      if (path === ROOT_PATH) return Promise.resolve({ root: { ...ROOT, children: [CODEX] } });
      if (path === CODEX_PATH) return Promise.resolve({ root: { ...CODEX, children: [AGENTS] } });
      // Empty directories omit children on the backend wire response.
      return Promise.resolve({ root: responseKind === "empty" ? AGENTS : null });
    });
    rerender({ sessionId: "session-2" });
    await waitFor(() => expect(result.current.loadState).toBe("loaded"));
    const folder = result.current.visibleRows.find((row) => row.path === AGENTS_PATH)!.node;
    expect(folder.children ?? []).toEqual([]);
    expect(result.current.visibleRows.map((row) => row.path)).not.toContain(CONFIG_PATH);

    const fresh = { ...CONFIG, name: "fresh.toml", path: `${AGENTS_PATH}/fresh.toml` };
    requestFileTreeMock.mockResolvedValue({ root: { ...AGENTS, children: [fresh] } });
    await act(async () => {
      result.current.setExpandedPaths((previous) => new Set([...previous, AGENTS_PATH]));
      await loadNodeChildren(folder, "session-2", result.current);
    });
    expect(result.current.visibleRows.map((row) => row.path)).toContain(fresh.path);
    expect(result.current.visibleRows.map((row) => row.path)).not.toContain(CONFIG_PATH);
  },
);
