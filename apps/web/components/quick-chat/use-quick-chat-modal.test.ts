import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Mocks must be declared before importing the hook so vi.mock hoists correctly.
const mockToast = vi.fn();
const mockStartQuickChat = vi.fn();
const mockDeleteTask = vi.fn();
let mockAppState: ReturnType<typeof makeAppState>;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: ReturnType<typeof makeAppState>) => unknown) =>
    selector(mockAppState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  startQuickChat: (...args: unknown[]) => mockStartQuickChat(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: (...args: unknown[]) => mockDeleteTask(...args),
}));

import { useAgentSelection, useQuickChatModal } from "./use-quick-chat-modal";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";

const WORKSPACE_ID = "ws-1";
const CHAT_SETUP_ID = getQuickChatSetupSessionId(WORKSPACE_ID, "chat");

type MockStore = Parameters<typeof useAgentSelection>[1];

function makeAppState() {
  return {
    quickChat: {
      isOpen: true,
      sessions: [] as Array<{
        sessionId: string;
        workspaceId: string;
        kind: "chat" | "config";
      }>,
      activeSessionId: "",
    },
    closeQuickChat: vi.fn(),
    closeQuickChatSession: vi.fn(),
    setActiveQuickChatSession: vi.fn(),
    renameQuickChatSession: vi.fn(),
    openQuickChat: vi.fn(),
    agentProfiles: { items: [] },
    taskSessions: { items: {} as Record<string, { task_id: string }> },
  };
}

function makeStore(overrides: Partial<MockStore> = {}): MockStore {
  return {
    isOpen: true,
    sessions: [],
    activeSessionId: "",
    closeQuickChat: vi.fn(),
    closeQuickChatSession: vi.fn(),
    setActiveQuickChatSession: vi.fn(),
    renameQuickChatSession: vi.fn(),
    openQuickChat: vi.fn(),
    agentProfiles: [
      { id: "agent-a", label: "Agent A", agent_id: "a", agent_name: "Agent A" },
      { id: "agent-b", label: "Agent B", agent_id: "b", agent_name: "Agent B" },
    ] as MockStore["agentProfiles"],
    taskSessions: {},
    ...overrides,
  };
}

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  vi.clearAllMocks();
  mockAppState = makeAppState();
});

describe("useQuickChatModal — setup lifecycle", () => {
  it("removes a blank placeholder when dismissed from an active session", () => {
    mockAppState.quickChat.sessions = [
      { sessionId: CHAT_SETUP_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
      { sessionId: "session-1", workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    mockAppState.quickChat.activeSessionId = "session-1";
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() => result.current.handleOpenChange(false));

    expect(mockAppState.closeQuickChatSession).toHaveBeenCalledWith(CHAT_SETUP_ID);
    expect(mockAppState.closeQuickChat).toHaveBeenCalledTimes(1);
  });

  it("changes the setup key when a fresh blank chat is requested", () => {
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    expect(result.current.setupKey).toBe(0);
    act(() => result.current.handleNewChat());

    expect(result.current.setupKey).toBe(1);
    expect(mockAppState.openQuickChat).toHaveBeenCalledWith("", WORKSPACE_ID, undefined, "chat");
  });

  it("supersedes an in-flight config start when the user changes tabs", () => {
    const resetConfigStart = vi.fn();
    mockAppState.quickChat.sessions = [
      { sessionId: "session-1", workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    mockAppState.quickChat.activeSessionId = "session-1";
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID, resetConfigStart));

    act(() => result.current.setActiveQuickChatSession("session-1"));

    expect(resetConfigStart).toHaveBeenCalledTimes(1);
    expect(mockAppState.setActiveQuickChatSession).toHaveBeenCalledWith("session-1", WORKSPACE_ID);
  });
});

describe("useQuickChatModal — persisted config lifecycle", () => {
  it("deletes the backing task only after config-tab close is confirmed", async () => {
    const configSessionId = "config-session";
    mockAppState.quickChat.sessions = [
      { sessionId: configSessionId, workspaceId: WORKSPACE_ID, kind: "config" },
    ];
    mockAppState.quickChat.activeSessionId = configSessionId;
    mockAppState.taskSessions.items = { [configSessionId]: { task_id: "config-task" } };
    mockDeleteTask.mockResolvedValue(undefined);
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() => result.current.handleCloseTab(configSessionId));
    expect(mockDeleteTask).not.toHaveBeenCalled();

    await act(async () => result.current.handleConfirmClose());

    expect(mockAppState.closeQuickChatSession).toHaveBeenCalledWith(configSessionId);
    expect(mockDeleteTask).toHaveBeenCalledWith("config-task");
  });
});

describe("useAgentSelection — happy path", () => {
  it("opens the chat and clears pending state when the request resolves", async () => {
    const store = makeStore();
    mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a");
    });

    expect(store.openQuickChat).toHaveBeenCalledWith("sess-a", WORKSPACE_ID, "agent-a");
    expect(store.renameQuickChatSession).toHaveBeenCalledWith("sess-a", expect.any(String));
    expect(mockDeleteTask).not.toHaveBeenCalled();
    expect(result.current.pendingAgentId).toBeNull();
  });

  it("forwards ordered repository context to the start request", async () => {
    const store = makeStore();
    mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });
    const repositories = [
      { repository_id: "repo-front", base_branch: "main" },
      { repository_id: "repo-back", base_branch: "develop" },
    ];

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a", repositories);
    });

    expect(mockStartQuickChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ repositories }),
    );
  });
});

describe("useAgentSelection — supersession", () => {
  it("rapid-pick: a newer pick deletes the older orphan task", async () => {
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
    expect(store.openQuickChat).toHaveBeenCalledWith("sess-b", WORKSPACE_ID, "agent-b");

    // Now A resolves — its orphan task is deleted instead of opening a stale session.
    await act(async () => {
      resolveFirst({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });
    expect(mockDeleteTask).toHaveBeenCalledWith("task-a");
    expect(store.openQuickChat).not.toHaveBeenCalledWith(
      "sess-a",
      expect.anything(),
      expect.anything(),
    );
  });

  it("reset() during in-flight request deletes the resolved task", async () => {
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
    expect(mockDeleteTask).toHaveBeenCalledWith("task-a");
  });
});

describe("useAgentSelection — error handling", () => {
  it("does not toast when a superseded request rejects (avoid noise from races)", async () => {
    const store = makeStore();
    let rejectStart!: (e: Error) => void;
    mockStartQuickChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((_resolve, reject) => {
          rejectStart = reject;
        }),
    );

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    act(() => {
      void result.current.handleSelectAgent("agent-a");
    });
    act(() => {
      result.current.reset();
    });

    await act(async () => {
      rejectStart(new Error("network blew up"));
      await flushPromises();
    });

    expect(mockToast).not.toHaveBeenCalled();
  });

  it("toasts when the current (non-superseded) request rejects", async () => {
    const store = makeStore();
    mockStartQuickChat.mockRejectedValueOnce(new Error("server exploded"));
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a");
    });

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Failed to start quick chat",
        description: "server exploded",
        variant: "error",
      }),
    );
    expect(result.current.pendingAgentId).toBeNull();
  });
});
