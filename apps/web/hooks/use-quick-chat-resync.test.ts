import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ListQuickTerminalTabsResponse } from "@/lib/api/domains/quick-terminal-api";
import type { ListQuickChatSessionsResponse } from "@/lib/api/domains/workspace-api";
import type { TaskSession } from "@/lib/types/http";
const apiMock = vi.hoisted(() => ({
  listQuickChatSessions: vi.fn(),
  listQuickTerminalTabs: vi.fn(),
}));

const syncMock = vi.hoisted(() => ({
  migrateStoredQuickChatNames: vi.fn(),
  toQuickChatSessions: vi.fn((sessions: unknown[]) => sessions),
  toQuickTerminalTab: vi.fn((tab: unknown) => tab),
}));

const storeApiMock = vi.hoisted(() => ({
  getState: () => mockState,
}));

type MockState = {
  connection: { status: string };
  quickChat: { syncRevisionByWorkspace: Record<string, number> };
  taskSessions: { items: Record<string, TaskSession> };
  setTaskSession: (session: TaskSession) => void;
  syncQuickChatSessions: (...args: unknown[]) => void;
  syncQuickTerminalTabs: (...args: unknown[]) => void;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => storeApiMock,
}));
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listQuickChatSessions: apiMock.listQuickChatSessions,
}));
vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  listQuickTerminalTabs: apiMock.listQuickTerminalTabs,
  toQuickTerminalTab: syncMock.toQuickTerminalTab,
}));
vi.mock("@/lib/local-storage", () => ({ getStoredQuickChatNames: vi.fn(() => ({})) }));
vi.mock("@/lib/quick-chat/map-sessions", () => ({
  toQuickChatSessions: syncMock.toQuickChatSessions,
}));
vi.mock("@/lib/quick-chat/rename", () => ({
  migrateStoredQuickChatNames: syncMock.migrateStoredQuickChatNames,
}));

import { useQuickChatResync } from "./use-quick-chat-resync";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

function taskSession(state: TaskSession["state"], updatedAt: string): TaskSession {
  // The resync boundary only reads the stable identity, state, and update timestamp.
  return {
    id: "session-1",
    task_id: "task-1",
    state,
    updated_at: updatedAt,
  } as unknown as TaskSession;
}

const WORKSPACE_ID = "workspace-1";
const EMPTY_SESSIONS_RESPONSE = { sessions: [], task_sessions: [] };
const EMPTY_TABS_RESPONSE = { tabs: [] };

// eslint-disable-next-line max-lines-per-function -- resync scenarios share one connection fixture.
describe("useQuickChatResync", () => {
  beforeEach(() => {
    mockState = {
      connection: { status: "connected" },
      quickChat: { syncRevisionByWorkspace: { [WORKSPACE_ID]: 0 } },
      setTaskSession: vi.fn(),
      taskSessions: { items: {} },
      syncQuickChatSessions: vi.fn(),
      syncQuickTerminalTabs: vi.fn(),
    };
    vi.clearAllMocks();
  });

  afterEach(cleanup);

  it("discards a superseded response and retries the latest workspace state", async () => {
    const firstSessions = deferred<ListQuickChatSessionsResponse>();
    const firstTerminals = deferred<ListQuickTerminalTabsResponse>();
    const latestSessions = deferred<ListQuickChatSessionsResponse>();
    const latestTerminals = deferred<ListQuickTerminalTabsResponse>();
    apiMock.listQuickChatSessions
      .mockReturnValueOnce(firstSessions.promise)
      .mockReturnValueOnce(latestSessions.promise);
    apiMock.listQuickTerminalTabs
      .mockReturnValueOnce(firstTerminals.promise)
      .mockReturnValueOnce(latestTerminals.promise);

    renderHook(() => useQuickChatResync(WORKSPACE_ID));

    await act(async () => {
      mockState.quickChat.syncRevisionByWorkspace[WORKSPACE_ID] = 1;
      firstSessions.resolve(EMPTY_SESSIONS_RESPONSE);
      firstTerminals.resolve(EMPTY_TABS_RESPONSE);
      await Promise.resolve();
    });

    await waitFor(() => expect(apiMock.listQuickChatSessions).toHaveBeenCalledTimes(2));

    await act(async () => {
      latestSessions.resolve(EMPTY_SESSIONS_RESPONSE);
      latestTerminals.resolve(EMPTY_TABS_RESPONSE);
      await Promise.resolve();
    });

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));
    expect(mockState.syncQuickTerminalTabs).toHaveBeenCalledTimes(1);
  });

  it("accepts a snapshot when the workspace has no revision entry yet", async () => {
    mockState.quickChat.syncRevisionByWorkspace = {};
    apiMock.listQuickChatSessions.mockResolvedValueOnce(EMPTY_SESSIONS_RESPONSE);
    apiMock.listQuickTerminalTabs.mockResolvedValueOnce(EMPTY_TABS_RESPONSE);

    renderHook(() => useQuickChatResync(WORKSPACE_ID));

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));

    expect(apiMock.listQuickChatSessions).toHaveBeenCalledTimes(1);
    expect(mockState.syncQuickTerminalTabs).toHaveBeenCalledTimes(1);
  });

  it("does not overwrite a newer live task session with a stale resync row", async () => {
    const stale = taskSession("RUNNING", "2099-01-01T00:00:00Z");
    mockState.taskSessions.items[stale.id] = taskSession("IDLE", "2099-01-01T00:01:00Z");
    apiMock.listQuickChatSessions.mockResolvedValueOnce({
      sessions: [],
      task_sessions: [stale],
    });
    apiMock.listQuickTerminalTabs.mockResolvedValueOnce(EMPTY_TABS_RESPONSE);

    renderHook(() => useQuickChatResync(WORKSPACE_ID));

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));

    expect(mockState.setTaskSession).not.toHaveBeenCalled();
  });

  it("does not let an exact-second resync row regress a fractional-second live row", async () => {
    // "2099-01-01T00:00:00.1Z" is newer than "2099-01-01T00:00:00Z" even though a
    // lexicographic comparison sorts the exact-second string as larger.
    const stale = taskSession("RUNNING", "2099-01-01T00:00:00Z");
    mockState.taskSessions.items[stale.id] = taskSession("IDLE", "2099-01-01T00:00:00.1Z");
    apiMock.listQuickChatSessions.mockResolvedValueOnce({
      sessions: [],
      task_sessions: [stale],
    });
    apiMock.listQuickTerminalTabs.mockResolvedValueOnce(EMPTY_TABS_RESPONSE);

    renderHook(() => useQuickChatResync(WORKSPACE_ID));

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));

    expect(mockState.setTaskSession).not.toHaveBeenCalled();
  });

  it("applies a fractional-second resync row newer than an exact-second live row", async () => {
    const fresh = taskSession("IDLE", "2099-01-01T00:00:00.1Z");
    mockState.taskSessions.items[fresh.id] = taskSession("RUNNING", "2099-01-01T00:00:00Z");
    apiMock.listQuickChatSessions.mockResolvedValueOnce({
      sessions: [],
      task_sessions: [fresh],
    });
    apiMock.listQuickTerminalTabs.mockResolvedValueOnce(EMPTY_TABS_RESPONSE);

    renderHook(() => useQuickChatResync(WORKSPACE_ID));

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));

    expect(mockState.setTaskSession).toHaveBeenCalledWith(fresh);
  });

  it("stops retrying after the cap when every response observes a newer revision", async () => {
    // Every fetch captures revision R, then the store bumps to R+1 before the
    // response is compared, so each attempt is discarded as stale.
    apiMock.listQuickChatSessions.mockImplementation(() => {
      const response = Promise.resolve(EMPTY_SESSIONS_RESPONSE);
      queueMicrotask(() => {
        mockState.quickChat.syncRevisionByWorkspace[WORKSPACE_ID] += 1;
      });
      return response;
    });
    apiMock.listQuickTerminalTabs.mockResolvedValue(EMPTY_TABS_RESPONSE);

    renderHook(() => useQuickChatResync(WORKSPACE_ID));

    await waitFor(() => expect(apiMock.listQuickChatSessions.mock.calls.length).toBe(4));
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(apiMock.listQuickChatSessions.mock.calls.length).toBe(4);
    expect(mockState.syncQuickChatSessions).not.toHaveBeenCalled();
  });

  it("starts a fresh retry budget after reconnecting", async () => {
    let phase: "first" | "second" = "first";
    let requestsInPhase = 0;
    apiMock.listQuickChatSessions.mockImplementation(() => {
      requestsInPhase += 1;
      const response = Promise.resolve(EMPTY_SESSIONS_RESPONSE);
      if (phase === "first" || requestsInPhase === 1) {
        queueMicrotask(() => {
          mockState.quickChat.syncRevisionByWorkspace[WORKSPACE_ID] += 1;
        });
      }
      return response;
    });
    apiMock.listQuickTerminalTabs.mockResolvedValue(EMPTY_TABS_RESPONSE);

    const rendered = renderHook(() => useQuickChatResync(WORKSPACE_ID));
    await waitFor(() => expect(apiMock.listQuickChatSessions).toHaveBeenCalledTimes(4));

    act(() => {
      mockState.connection.status = "disconnected";
      rendered.rerender();
    });
    phase = "second";
    requestsInPhase = 0;
    act(() => {
      mockState.connection.status = "connected";
      rendered.rerender();
    });

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));
    expect(apiMock.listQuickChatSessions).toHaveBeenCalledTimes(6);
  });
});
