import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import type { WorkspaceRootFilesResponse } from "@/lib/types/workspace-files";
import { useWorkspaceRootFiles } from "./use-workspace-root-files";

const mocks = vi.hoisted(() => ({
  state: {} as AppState,
  list: vi.fn(),
  listeners: new Map<string, Set<(message: unknown) => void>>(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => selector(mocks.state),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    on: (event: string, callback: (message: unknown) => void) => {
      const listeners = mocks.listeners.get(event) ?? new Set();
      listeners.add(callback);
      mocks.listeners.set(event, listeners);
      return () => listeners.delete(callback);
    },
  }),
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  listWorkspaceRootFiles: mocks.list,
}));

const SESSION_ID = "session-root-files";
const ENVIRONMENT_ID = "environment-root-files";
const ROOT_FILE = "dashboard.json";

function listing(path: string): WorkspaceRootFilesResponse {
  return {
    supported: true,
    files: [{ path, size: 1, modified_at: "2026-11-19T10:00:00Z" }],
    truncated: false,
    destinations: [],
  };
}

function emitFileChanges(repositoryName: string | undefined) {
  const message = {
    payload: {
      session_id: SESSION_ID,
      changes: [{ repository_name: repositoryName }],
    },
  };
  mocks.listeners.get("session.workspace.file.changes")?.forEach((listener) => listener(message));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.useFakeTimers();
  mocks.state = {
    connection: { status: "connected" },
    environmentIdBySessionId: { [SESSION_ID]: ENVIRONMENT_ID },
    gitStatus: {
      inventoryByEnvironment: {
        [ENVIRONMENT_ID]: { generation: 1, scopes: ["web"] },
      },
    },
    workspaceRootFiles: { byEnvironmentId: {} },
    setWorkspaceRootFiles: vi.fn(),
  } as unknown as AppState;
  mocks.listeners.clear();
  mocks.list.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useWorkspaceRootFiles fetch lifecycle", () => {
  it("fetches root files for the active environment on mount", async () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));

    renderHook(() => useWorkspaceRootFiles(SESSION_ID));

    expect(mocks.list).toHaveBeenCalledWith(expect.anything(), SESSION_ID);
    await act(async () => Promise.resolve());
    expect(mocks.state.setWorkspaceRootFiles).toHaveBeenCalledWith(
      ENVIRONMENT_ID,
      listing(ROOT_FILE),
    );
  });

  it("fetches again when the session environment changes", async () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const hook = renderHook(({ sessionId }) => useWorkspaceRootFiles(sessionId), {
      initialProps: { sessionId: SESSION_ID as string | null },
    });
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => Promise.resolve());

    mocks.state.environmentIdBySessionId[SESSION_ID] = "environment-root-files-next";
    hook.rerender({ sessionId: SESSION_ID });

    expect(mocks.list).toHaveBeenCalledTimes(2);
    await act(async () => Promise.resolve());
    expect(mocks.state.setWorkspaceRootFiles).toHaveBeenLastCalledWith(
      "environment-root-files-next",
      listing(ROOT_FILE),
    );
  });

  it("refreshes the listing when repository inventory changes", async () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const hook = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => Promise.resolve());

    mocks.state.gitStatus.inventoryByEnvironment![ENVIRONMENT_ID] = {
      generation: 2,
      scopes: ["api", "web"],
    };
    hook.rerender();
    act(() => vi.advanceTimersByTime(300));

    expect(mocks.list).toHaveBeenCalledTimes(2);
    await act(async () => Promise.resolve());
  });

  it("fetches after the WebSocket reconnects", async () => {
    mocks.state.connection.status = "disconnected";
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const hook = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).not.toHaveBeenCalled();

    mocks.state.connection.status = "connected";
    hook.rerender();
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => Promise.resolve());
    mocks.state.connection.status = "disconnected";
    hook.rerender();
    mocks.state.connection.status = "connected";
    hook.rerender();
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it("deduplicates simultaneous reads for one environment", async () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const first = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    const second = renderHook(() => useWorkspaceRootFiles(SESSION_ID));

    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => Promise.resolve());
    first.unmount();
    second.unmount();
  });
});

describe("useWorkspaceRootFiles notification refresh", () => {
  it("debounces root-scoped file notifications", () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => {
      emitFileChanges(undefined);
      emitFileChanges("");
      vi.advanceTimersByTime(299);
    });
    expect(mocks.list).toHaveBeenCalledTimes(1);
    act(() => vi.advanceTimersByTime(1));
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it("does not refetch for repository-scoped file notifications", () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => {
      emitFileChanges("web");
      vi.advanceTimersByTime(300);
    });

    expect(mocks.list).toHaveBeenCalledTimes(1);
  });

  it("clears a pending refetch when the last hook unmounts", () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const hook = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => emitFileChanges(undefined));
    hook.unmount();
    act(() => vi.advanceTimersByTime(300));

    expect(mocks.list).toHaveBeenCalledTimes(1);
  });

  it("clears the old environment refetch when the session environment changes", () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const hook = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => emitFileChanges(undefined));
    mocks.state.environmentIdBySessionId[SESSION_ID] = "environment-root-files-next";
    hook.rerender();
    expect(mocks.list).toHaveBeenCalledTimes(2);
    act(() => vi.advanceTimersByTime(300));

    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it("keeps a shared environment refetch while another hook remains mounted", () => {
    mocks.list.mockResolvedValue(listing(ROOT_FILE));
    const first = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    const second = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => emitFileChanges(undefined));
    first.unmount();
    act(() => vi.advanceTimersByTime(300));

    expect(mocks.list).toHaveBeenCalledTimes(2);
    second.unmount();
  });

  it("keeps a newer result when an older refresh resolves later", async () => {
    const older = deferred<WorkspaceRootFilesResponse>();
    const newer = deferred<WorkspaceRootFilesResponse>();
    mocks.list.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);

    act(() => {
      emitFileChanges(undefined);
      vi.advanceTimersByTime(300);
    });
    expect(mocks.list).toHaveBeenCalledTimes(2);

    await act(async () => newer.resolve(listing("newer.json")));
    expect(mocks.state.setWorkspaceRootFiles).toHaveBeenLastCalledWith(
      ENVIRONMENT_ID,
      listing("newer.json"),
    );
    await act(async () => older.resolve(listing("older.json")));

    expect(mocks.state.setWorkspaceRootFiles).toHaveBeenLastCalledWith(
      ENVIRONMENT_ID,
      listing("newer.json"),
    );
  });

  it("starts a fresh read after the last observer unmounts with a request pending", async () => {
    const oldRequest = deferred<WorkspaceRootFilesResponse>();
    mocks.list.mockReturnValueOnce(oldRequest.promise).mockResolvedValueOnce(listing("fresh.json"));

    const first = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(1);
    first.unmount();

    const second = renderHook(() => useWorkspaceRootFiles(SESSION_ID));
    expect(mocks.list).toHaveBeenCalledTimes(2);
    await act(async () => Promise.resolve());
    expect(mocks.state.setWorkspaceRootFiles).toHaveBeenLastCalledWith(
      ENVIRONMENT_ID,
      listing("fresh.json"),
    );

    await act(async () => oldRequest.resolve(listing("stale.json")));
    expect(mocks.state.setWorkspaceRootFiles).toHaveBeenLastCalledWith(
      ENVIRONMENT_ID,
      listing("fresh.json"),
    );
    second.unmount();
  });
});
