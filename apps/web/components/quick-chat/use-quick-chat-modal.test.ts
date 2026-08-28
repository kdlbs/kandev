import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { QuickTerminalTab } from "@/lib/state/slices/ui/types";

// Mocks must be declared before importing the hook so vi.mock hoists correctly.
const mockToast = vi.fn();
const mockStartQuickChat = vi.fn();
const mockDeleteTask = vi.fn();
const mockUpdateTask = vi.fn();
const mockDeleteQuickTerminalTab = vi.fn();
const mockUpdateQuickTerminalTab = vi.fn();
const recordRecentUseMock = vi.fn();
let mockAppState: ReturnType<typeof makeAppState>;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: ReturnType<typeof makeAppState>) => unknown) =>
    selector(mockAppState),
  useAppStoreApi: () => ({ getState: () => mockAppState }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  startQuickChat: (...args: unknown[]) => mockStartQuickChat(...args),
}));

vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: (...args: unknown[]) => recordRecentUseMock(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: (...args: unknown[]) => mockDeleteTask(...args),
  updateTask: (...args: unknown[]) => mockUpdateTask(...args),
}));

vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  deleteQuickTerminalTab: (...args: unknown[]) => mockDeleteQuickTerminalTab(...args),
  updateQuickTerminalTab: (...args: unknown[]) => mockUpdateQuickTerminalTab(...args),
}));

import { useAgentSelection, useQuickChatModal } from "./use-quick-chat-modal";
import { requestQuickChatClose } from "./quick-chat-focus";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";

const WORKSPACE_ID = "ws-1";
const TERMINAL_ONE_ID = "terminal-1";
const SESSION_ONE_ID = "session-1";
const CHAT_SETUP_ID = getQuickChatSetupSessionId(WORKSPACE_ID, "chat");
const LATE_TERMINAL_ID = "terminal-late";
const LATE_SESSION_ID = "late-session";

type MockStore = Parameters<typeof useAgentSelection>[1];

function makeAppState() {
  return {
    quickChat: {
      isOpen: true,
      sessions: [] as Array<{
        sessionId: string;
        workspaceId: string;
        kind: "chat" | "config";
        taskId?: string;
      }>,
      activeSessionId: "",
      activeKind: "conversation" as const,
      activeTerminalTabId: null,
      terminalTabs: [] as QuickTerminalTab[],
      tabOrderByWorkspace: {} as Record<string, string[]>,
      tabOrderSyncErrorByWorkspace: {} as Record<string, string | null>,
      tabOrderSyncPendingByWorkspace: {} as Record<string, boolean>,
    },
    userSettings: { quickChatTabOrderByWorkspace: {}, agentGeneratedTaskTitles: true },
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
    applyAgentProfileRecentUse: vi.fn(),
    setQuickChatTabOrder: vi.fn(),
    clearQuickChatTabOrder: vi.fn(),
    setQuickChatTabOrderSyncState: vi.fn(),
    agentProfiles: { items: [] },
    taskSessions: { items: {} as Record<string, { task_id: string }> },
  };
}

function makeStore(overrides: Partial<MockStore> = {}): MockStore {
  return {
    isOpen: true,
    sessions: [],
    terminalTabs: [],
    activeSessionId: "",
    activeKind: "conversation",
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
  mockDeleteQuickTerminalTab.mockReset();
  mockDeleteQuickTerminalTab.mockResolvedValue(undefined);
  mockUpdateQuickTerminalTab.mockReset();
  mockUpdateQuickTerminalTab.mockResolvedValue({
    sequence: 1,
    sessionId: null,
    status: "running",
  });
  mockAppState = makeAppState();
  recordRecentUseMock.mockReset();
});

describe("useQuickChatModal — terminal close lifecycle", () => {
  const terminal = (tabId: string, sessionId: string): QuickTerminalTab => ({
    tabId,
    workspaceId: WORKSPACE_ID,
    sessionId,
    sequence: Number(tabId.slice(-1)),
    status: "running",
  });

  it("removes a terminal when the stop endpoint says it is already gone", async () => {
    mockAppState.quickChat.terminalTabs = [terminal(TERMINAL_ONE_ID, SESSION_ONE_ID)];
    mockDeleteQuickTerminalTab.mockRejectedValue(
      new (await import("@/lib/api/client")).ApiError("gone", 404, null),
    );

    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    await act(async () => result.current.handleCloseTerminal(TERMINAL_ONE_ID));

    expect(mockDeleteQuickTerminalTab).toHaveBeenCalledWith(TERMINAL_ONE_ID);
    expect(mockAppState.removeQuickTerminal).toHaveBeenCalledWith(TERMINAL_ONE_ID);
  });

  it("keeps a terminal and records an error when stopping fails", async () => {
    mockAppState.quickChat.terminalTabs = [terminal(TERMINAL_ONE_ID, SESSION_ONE_ID)];
    mockDeleteQuickTerminalTab.mockRejectedValue(new Error("stop failed"));

    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    await act(async () => result.current.handleCloseTerminal(TERMINAL_ONE_ID));

    expect(mockAppState.removeQuickTerminal).not.toHaveBeenCalled();
    expect(mockAppState.updateQuickTerminal).toHaveBeenCalledWith(TERMINAL_ONE_ID, {
      status: "error",
      error: "stop failed",
    });
    expect(mockToast).toHaveBeenCalled();
  });

  it("stops only the explicitly closed sibling terminal", async () => {
    mockAppState.quickChat.terminalTabs = [
      terminal(TERMINAL_ONE_ID, SESSION_ONE_ID),
      terminal("terminal-2", "session-2"),
    ];

    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    await act(async () => result.current.handleCloseTerminal(TERMINAL_ONE_ID));

    expect(mockDeleteQuickTerminalTab).toHaveBeenCalledTimes(1);
    expect(mockDeleteQuickTerminalTab).toHaveBeenCalledWith(TERMINAL_ONE_ID);
    expect(mockAppState.removeQuickTerminal).toHaveBeenCalledWith(TERMINAL_ONE_ID);
    expect(mockAppState.removeQuickTerminal).not.toHaveBeenCalledWith("terminal-2");
  });

  it("stops a detached terminal after its late start reports the session", async () => {
    const tab: QuickTerminalTab = {
      tabId: LATE_TERMINAL_ID,
      workspaceId: WORKSPACE_ID,
      sessionId: null,
      sequence: 1,
      status: "connecting",
    };
    mockAppState.quickChat.terminalTabs = [tab];
    mockAppState.updateQuickTerminal = vi.fn((_tabId, update) => {
      Object.assign(tab, update);
    });
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() =>
      result.current.handleTerminalStateChange(LATE_TERMINAL_ID, {
        status: "running",
        sessionId: LATE_SESSION_ID,
        exitCode: null,
        error: null,
      }),
    );
    await act(async () => result.current.handleCloseTerminal(LATE_TERMINAL_ID));

    expect(mockDeleteQuickTerminalTab).toHaveBeenCalledWith(LATE_TERMINAL_ID);
    expect(mockAppState.removeQuickTerminal).toHaveBeenCalledWith(LATE_TERMINAL_ID);
  });
});

describe("useQuickChatModal — setup lifecycle", () => {
  it("removes a blank placeholder when dismissed from an active session", () => {
    mockAppState.quickChat.sessions = [
      { sessionId: CHAT_SETUP_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
      { sessionId: SESSION_ONE_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    mockAppState.quickChat.activeSessionId = SESSION_ONE_ID;
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

  it("switches an ordinary setup to configuration mode", () => {
    mockAppState.quickChat.sessions = [
      { sessionId: CHAT_SETUP_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    mockAppState.quickChat.activeSessionId = CHAT_SETUP_ID;
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() => result.current.handleSetupKindChange("config"));

    expect(mockAppState.closeQuickChatSession).toHaveBeenCalledWith(CHAT_SETUP_ID);
    expect(mockAppState.openQuickChat).toHaveBeenCalledWith("", WORKSPACE_ID, undefined, "config");
  });

  it("supersedes an in-flight config start when the user changes tabs", () => {
    const resetConfigStart = vi.fn();
    mockAppState.quickChat.sessions = [
      { sessionId: SESSION_ONE_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    mockAppState.quickChat.activeSessionId = SESSION_ONE_ID;
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID, resetConfigStart));

    act(() => result.current.setActiveQuickChatSession(SESSION_ONE_ID));

    expect(resetConfigStart).toHaveBeenCalledTimes(1);
    expect(mockAppState.setActiveQuickChatSession).toHaveBeenCalledWith(
      SESSION_ONE_ID,
      WORKSPACE_ID,
    );
  });

  it("invalidates an in-flight agent start when an external launcher closes the modal", async () => {
    let resolveStart!: (value: { task_id: string; session_id: string }) => void;
    mockStartQuickChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((resolve) => {
          resolveStart = resolve;
        }),
    );
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() => {
      void result.current.handleSelectAgent("agent-a");
    });
    expect(result.current.pendingAgentId).toBe("agent-a");

    expect(requestQuickChatClose()).toBe(true);

    await act(async () => {
      resolveStart({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });

    expect(mockAppState.openQuickChat).not.toHaveBeenCalledWith(
      "sess-a",
      expect.anything(),
      expect.anything(),
      expect.anything(),
      expect.anything(),
    );
    expect(mockDeleteTask).toHaveBeenCalledWith("task-a");
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

    expect(mockAppState.removeQuickChatSession).toHaveBeenCalledWith(configSessionId);
    expect(mockDeleteTask).toHaveBeenCalledWith("config-task");
  });

  it("keeps the session open when backing-task deletion fails", async () => {
    const configSessionId = "config-session";
    mockAppState.quickChat.sessions = [
      { sessionId: configSessionId, workspaceId: WORKSPACE_ID, kind: "config" },
    ];
    mockAppState.quickChat.activeSessionId = configSessionId;
    mockAppState.taskSessions.items = { [configSessionId]: { task_id: "config-task" } };
    mockDeleteTask.mockRejectedValueOnce(new Error("delete failed"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() => result.current.handleCloseTab(configSessionId));
    await act(async () => result.current.handleConfirmClose());

    expect(mockAppState.closeQuickChatSession).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
    consoleError.mockRestore();
  });

  it("activates the adjacent mixed tab after closing the active conversation", async () => {
    mockAppState.quickChat.sessions = [
      {
        sessionId: SESSION_ONE_ID,
        workspaceId: WORKSPACE_ID,
        kind: "chat",
        taskId: "chat-task",
      },
    ];
    mockAppState.quickChat.terminalTabs = [
      {
        tabId: TERMINAL_ONE_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: "terminal-session",
        sequence: 1,
        status: "running",
      },
    ];
    mockAppState.quickChat.activeKind = "conversation";
    mockAppState.quickChat.activeSessionId = SESSION_ONE_ID;
    mockAppState.userSettings.quickChatTabOrderByWorkspace = {
      [WORKSPACE_ID]: [`terminal:${TERMINAL_ONE_ID}`, `conversation:${SESSION_ONE_ID}`],
    };
    mockDeleteTask.mockResolvedValue(undefined);

    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    act(() => result.current.handleCloseTab(SESSION_ONE_ID));
    await act(async () => result.current.handleConfirmClose());

    expect(mockAppState.removeQuickChatSession).toHaveBeenCalledWith(SESSION_ONE_ID);
    expect(mockAppState.activateQuickTerminal).toHaveBeenCalledWith(TERMINAL_ONE_ID, WORKSPACE_ID);
  });

  it("exposes only sessions from the hydrated workspace", () => {
    mockAppState.quickChat.sessions = [
      { sessionId: "session-a", workspaceId: WORKSPACE_ID, kind: "chat" },
      { sessionId: "session-b", workspaceId: "ws-2", kind: "chat" },
    ];

    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    expect(result.current.sessions).toEqual([
      { sessionId: "session-a", workspaceId: WORKSPACE_ID, kind: "chat" },
    ]);
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

    // taskId is threaded through so closing the tab can delete the right task.
    expect(store.openQuickChat).toHaveBeenCalledWith(
      "sess-a",
      WORKSPACE_ID,
      "agent-a",
      "chat",
      "task-a",
    );
    expect(store.renameQuickChatSession).toHaveBeenCalledWith("sess-a", expect.any(String));
    expect(mockDeleteTask).not.toHaveBeenCalled();
    expect(recordRecentUseMock).toHaveBeenCalledWith("quick_chat", "agent-a", expect.any(Function));
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

  it("forwards the enabled agent-title preference to the start request", async () => {
    const store = makeStore({ agentGeneratedTaskTitles: true });
    mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a");
    });

    expect(mockStartQuickChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ auto_title: true }),
    );
  });

  it("omits agent-title generation when the preference is disabled", async () => {
    const store = makeStore({ agentGeneratedTaskTitles: false });
    mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a");
    });

    expect(mockStartQuickChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.not.objectContaining({ auto_title: true }),
    );
  });

  it("numbers chats using only ordinary sessions in the current workspace", async () => {
    const store = makeStore({
      sessions: [
        { sessionId: "local-chat", workspaceId: WORKSPACE_ID, kind: "chat" },
        { sessionId: "local-config", workspaceId: WORKSPACE_ID, kind: "config" },
        { sessionId: "other-chat", workspaceId: "ws-2", kind: "chat" },
        { sessionId: CHAT_SETUP_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
      ],
    });
    mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a");
    });

    expect(mockStartQuickChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ title: "Agent A - Chat 2" }),
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
    expect(store.openQuickChat).toHaveBeenCalledWith(
      "sess-b",
      WORKSPACE_ID,
      "agent-b",
      "chat",
      "task-b",
    );

    // Now A resolves — its orphan task is deleted instead of opening a stale session.
    await act(async () => {
      resolveFirst({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });
    expect(mockDeleteTask).toHaveBeenCalledWith("task-a");
    expect(recordRecentUseMock).toHaveBeenCalledTimes(1);
    expect(recordRecentUseMock).toHaveBeenCalledWith("quick_chat", "agent-b", expect.any(Function));
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

describe("useQuickChatModal — renaming", () => {
  it("saves the new name to the backing task so other devices pick it up", async () => {
    mockUpdateTask.mockResolvedValue(undefined);
    mockAppState.quickChat.sessions = [
      { sessionId: SESSION_ONE_ID, workspaceId: WORKSPACE_ID, kind: "chat", taskId: "task-1" },
    ];
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    await act(async () => {
      result.current.handleRename(SESSION_ONE_ID, "Renamed");
      await flushPromises();
    });

    expect(mockAppState.renameQuickChatSession).toHaveBeenCalledWith(SESSION_ONE_ID, "Renamed");
    expect(mockUpdateTask).toHaveBeenCalledWith("task-1", { title: "Renamed" });
    expect(mockToast).not.toHaveBeenCalled();
  });

  // The close path resolves the task via taskSessions when the tab itself has
  // no taskId; rename must agree, or it silently downgrades to a local-only
  // rename for exactly those sessions — and without a toast, since that branch
  // resolves rather than rejects.
  it("falls back to taskSessions for a tab that carries no taskId", async () => {
    mockUpdateTask.mockResolvedValue(undefined);
    mockAppState.quickChat.sessions = [
      { sessionId: SESSION_ONE_ID, workspaceId: WORKSPACE_ID, kind: "chat" },
    ];
    mockAppState.taskSessions.items = { [SESSION_ONE_ID]: { task_id: "task-legacy" } };
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    await act(async () => {
      result.current.handleRename(SESSION_ONE_ID, "Renamed");
      await flushPromises();
    });

    expect(mockUpdateTask).toHaveBeenCalledWith("task-legacy", { title: "Renamed" });
  });

  it("warns that the rename did not sync when the request fails", async () => {
    mockUpdateTask.mockRejectedValue(new Error("offline"));
    mockAppState.quickChat.sessions = [
      { sessionId: SESSION_ONE_ID, workspaceId: WORKSPACE_ID, kind: "chat", taskId: "task-1" },
    ];
    const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

    await act(async () => {
      result.current.handleRename(SESSION_ONE_ID, "Renamed");
      await flushPromises();
    });

    // The label still changed locally — only the sync failed.
    expect(mockAppState.renameQuickChatSession).toHaveBeenCalledWith(SESSION_ONE_ID, "Renamed");
    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });
});
