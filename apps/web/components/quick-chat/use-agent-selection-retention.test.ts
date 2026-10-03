import { ApiError } from "@/lib/api/client";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Mocks must be declared before importing the hook so vi.mock hoists correctly.
const mockToast = vi.fn();
const mockStartQuickChat = vi.fn();
const mockDeleteTask = vi.fn();
const recordRecentUseMock = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api/domains/workspace-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/workspace-api")>()),
  startQuickChat: (...args: unknown[]) => mockStartQuickChat(...args),
}));

vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: (...args: unknown[]) => recordRecentUseMock(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: (...args: unknown[]) => mockDeleteTask(...args),
  deleteTaskAfterUserAction: (...args: unknown[]) => mockDeleteTask(...args),
  updateTask: vi.fn(),
}));

import { createAppStore } from "@/lib/state/store";

import { useAgentSelection } from "./use-quick-chat-modal";

const WORKSPACE_ID = "ws-1";

type MockStore = Parameters<typeof useAgentSelection>[1];

function makeStore(overrides: Partial<MockStore> = {}): MockStore {
  return {
    isOpen: true,
    sessions: [],
    terminalTabs: [],
    activeSessionId: "",
    activeKind: "conversation",
    pendingQuickChatOpen: null,
    activeTerminalTabId: null,
    closeQuickChat: vi.fn(),
    closeQuickChatSession: vi.fn(),
    removeQuickChatSession: vi.fn(),
    setActiveQuickChatSession: vi.fn(),
    createQuickTerminal: vi.fn(),
    updateQuickTerminal: vi.fn(),
    activateQuickTerminal: vi.fn(),
    removeQuickTerminal: vi.fn(),
    renameQuickChatSession: vi.fn(),
    openQuickChat: vi.fn(),
    upsertQuickChatSessionFromEvent: vi.fn(),
    applyAgentProfileRecentUse: vi.fn(),
    agentProfiles: [
      { id: "agent-a", label: "Agent A", agent_id: "a", agent_name: "Agent A" },
      { id: "agent-b", label: "Agent B", agent_id: "b", agent_name: "Agent B" },
    ] as MockStore["agentProfiles"],
    agentGeneratedTaskTitles: true,
    taskSessions: {},
    ...overrides,
  };
}

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  vi.clearAllMocks();
  recordRecentUseMock.mockReset();
});

describe("useAgentSelection — supersession", () => {
  it("rapid-pick: a newer pick activates its session while the older pick upserts into tabs without deleting", async () => {
    const store = makeStore();
    let resolveFirst!: (v: { task_id: string; session_id: string }) => void;
    const firstPromise = new Promise<{ task_id: string; session_id: string }>((r) => {
      resolveFirst = r;
    });
    mockStartQuickChat
      .mockImplementationOnce(() => firstPromise)
      .mockResolvedValueOnce({ task_id: "task-b", session_id: "sess-b" });

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    // Click A — request hangs.
    act(() => {
      void result.current.handleSelectAgent("agent-a");
    });
    expect(result.current.pendingAgentId).toBe("agent-a");

    // Click B — supersedes A.
    await act(async () => {
      await result.current.handleSelectAgent("agent-b");
    });
    expect(store.openQuickChat).toHaveBeenCalledWith(
      "sess-b",
      WORKSPACE_ID,
      "agent-b",
      "chat",
      "task-b",
    );

    // Now A resolves — its session is upserted without activating and without deleting.
    await act(async () => {
      resolveFirst({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });
    expect(mockDeleteTask).not.toHaveBeenCalled();
    expect(store.upsertQuickChatSessionFromEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: "sess-a",
        workspaceId: WORKSPACE_ID,
        agentProfileId: "agent-a",
        kind: "chat",
        taskId: "task-a",
      }),
    );
    expect(recordRecentUseMock).toHaveBeenCalledWith("quick_chat", "agent-a", expect.any(Function));
    expect(recordRecentUseMock).toHaveBeenCalledWith("quick_chat", "agent-b", expect.any(Function));
  });

  it("reset() during in-flight request upserts the resolved task without selecting or deleting", async () => {
    const store = makeStore();
    let resolveStart!: (v: { task_id: string; session_id: string }) => void;
    mockStartQuickChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((r) => {
          resolveStart = r;
        }),
    );

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    act(() => {
      void result.current.handleSelectAgent("agent-a");
    });
    expect(result.current.pendingAgentId).toBe("agent-a");

    // User does something that supersedes the in-flight pick (handleNewChat, tab switch, etc.).
    act(() => {
      result.current.reset();
    });
    expect(result.current.pendingAgentId).toBeNull();

    await act(async () => {
      resolveStart({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });
    expect(store.openQuickChat).not.toHaveBeenCalled();
    expect(mockDeleteTask).not.toHaveBeenCalled();
    expect(store.upsertQuickChatSessionFromEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: "sess-a",
        workspaceId: WORKSPACE_ID,
        agentProfileId: "agent-a",
        kind: "chat",
        taskId: "task-a",
      }),
    );
  });
});

describe("late quick chat creation with real store actions", () => {
  it.each([false, true])(
    "preserves newer selection for a late response (failed: %s)",
    async (failed) => {
      const app = createAppStore();
      const pending = Promise.withResolvers<unknown>();
      mockStartQuickChat.mockReturnValueOnce(pending.promise);
      app.getState().openQuickChat("", WORKSPACE_ID);
      const store = makeStore({
        ...app.getState(),
        ...app.getState().quickChat,
        agentProfiles: makeStore().agentProfiles,
        taskSessions: app.getState().taskSessions.items,
      });
      const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));
      let request!: Promise<void>;
      act(() => {
        request = result.current.handleSelectAgent("agent-a");
      });
      act(() => {
        result.current.reset();
      });
      app
        .getState()
        .openQuickChat("selected-session", WORKSPACE_ID, "agent-b", "chat", "selected-task");
      await act(async () => {
        const body = { task_id: "late-task", session_id: "late-session" };
        if (failed) pending.reject(new ApiError("failed to start session", 500, body));
        else pending.resolve(body);
        await request;
      });
      expect(app.getState().quickChat.activeSessionId).toBe("selected-session");
      expect(
        app.getState().quickChat.sessions.some((session) => session.sessionId === "late-session"),
      ).toBe(true);
      expect(mockDeleteTask).not.toHaveBeenCalled();
    },
  );
});
