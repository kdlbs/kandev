import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockListQuickChatSessions = vi.hoisted(() => vi.fn());
const mockUpdateTask = vi.hoisted(() => vi.fn());
const mockState = vi.hoisted(() => ({
  value: {} as {
    connection: { status: string };
    syncQuickChatSessions: unknown;
    setTaskSession: unknown;
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState.value) => unknown) => selector(mockState.value),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listQuickChatSessions: (...args: unknown[]) => mockListQuickChatSessions(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  updateTask: (...args: unknown[]) => mockUpdateTask(...args),
}));

import { useQuickChatResync } from "./use-quick-chat-resync";
import { getStoredQuickChatNames, setStoredQuickChatName } from "@/lib/local-storage";

const syncQuickChatSessions = vi.fn();
const setTaskSession = vi.fn();

function setConnection(status: string) {
  mockState.value = { connection: { status }, syncQuickChatSessions, setTaskSession };
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  mockUpdateTask.mockResolvedValue(undefined);
  setConnection("connected");
  mockListQuickChatSessions.mockResolvedValue({
    sessions: [
      {
        session_id: "session-1",
        task_id: "task-1",
        workspace_id: "ws-1",
        kind: "chat",
        name: "Chat 1",
        agent_profile_id: "agent-1",
      },
    ],
    task_sessions: [{ id: "session-1", task_id: "task-1" }],
  });
});

describe("useQuickChatResync", () => {
  it("replaces the workspace's tabs with the server's list once connected", async () => {
    renderHook(() => useQuickChatResync("ws-1"));

    await waitFor(() => expect(syncQuickChatSessions).toHaveBeenCalledTimes(1));
    expect(syncQuickChatSessions).toHaveBeenCalledWith("ws-1", [
      {
        kind: "chat",
        sessionId: "session-1",
        taskId: "task-1",
        workspaceId: "ws-1",
        name: "Chat 1",
        agentProfileId: "agent-1",
      },
    ]);
  });

  // A tab without its session row renders but cannot subscribe or accept
  // input, so the rows in the response must land in the store too.
  it("stores the session rows that came with the list", async () => {
    renderHook(() => useQuickChatResync("ws-1"));

    await waitFor(() =>
      expect(setTaskSession).toHaveBeenCalledWith({ id: "session-1", task_id: "task-1" }),
    );
  });

  it("waits for the socket before trusting any list", () => {
    setConnection("connecting");

    renderHook(() => useQuickChatResync("ws-1"));

    expect(mockListQuickChatSessions).not.toHaveBeenCalled();
  });

  it("does nothing without an active workspace", () => {
    renderHook(() => useQuickChatResync(null));

    expect(mockListQuickChatSessions).not.toHaveBeenCalled();
  });

  it("resyncs again after a reconnect, since events were missed while away", async () => {
    const { rerender } = renderHook(() => useQuickChatResync("ws-1"));
    await waitFor(() => expect(mockListQuickChatSessions).toHaveBeenCalledTimes(1));

    setConnection("reconnecting");
    rerender();
    setConnection("connected");
    rerender();

    await waitFor(() => expect(mockListQuickChatSessions).toHaveBeenCalledTimes(2));
  });

  it("does not refetch on unrelated re-renders of the same connection", async () => {
    const { rerender } = renderHook(() => useQuickChatResync("ws-1"));
    await waitFor(() => expect(mockListQuickChatSessions).toHaveBeenCalledTimes(1));

    rerender();
    rerender();

    expect(mockListQuickChatSessions).toHaveBeenCalledTimes(1);
  });

  it("leaves existing tabs alone when the resync fails", async () => {
    mockListQuickChatSessions.mockRejectedValueOnce(new Error("offline"));

    renderHook(() => useQuickChatResync("ws-1"));

    await waitFor(() => expect(mockListQuickChatSessions).toHaveBeenCalled());
    expect(syncQuickChatSessions).not.toHaveBeenCalled();
  });
});

describe("useQuickChatResync — legacy rename migration", () => {
  it("uploads a browser-only rename so it survives and reaches other devices", async () => {
    setStoredQuickChatName("session-1", "My old local name");

    renderHook(() => useQuickChatResync("ws-1"));

    await waitFor(() =>
      expect(mockUpdateTask).toHaveBeenCalledWith("task-1", { title: "My old local name" }),
    );
    await waitFor(() => expect(getStoredQuickChatNames()).toEqual({}));
  });

  it("does not re-upload a name the server already has", async () => {
    setStoredQuickChatName("session-1", "Chat 1");

    renderHook(() => useQuickChatResync("ws-1"));

    await waitFor(() => expect(syncQuickChatSessions).toHaveBeenCalled());
    expect(mockUpdateTask).not.toHaveBeenCalled();
  });
});
