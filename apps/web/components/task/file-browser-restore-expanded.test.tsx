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

import { useFileBrowserTree } from "./file-browser-hooks";

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

describe("useFileBrowserTree persisted expansion", () => {
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
