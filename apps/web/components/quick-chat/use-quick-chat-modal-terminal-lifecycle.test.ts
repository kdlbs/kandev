import { ApiError } from "@/lib/api/client";
import { createElement } from "react";
import { act, render, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { QuickTerminalTab } from "@/lib/state/slices/ui/types";

const mockToast = vi.fn();
const mockStartQuickChat = vi.fn();
const mockDeleteTask = vi.fn();
const mockUpdateTask = vi.fn();
const mockDeleteQuickTerminalTab = vi.fn();
const mockUpdateQuickTerminalTab = vi.fn();
const mockListQuickTerminalTabs = vi.fn();
const mockCreateQuickTerminalTab = vi.fn();
const mockStartHostShell = vi.fn();
const mockPtyMount = vi.fn();
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
  updateTask: (...args: unknown[]) => mockUpdateTask(...args),
}));

vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  deleteQuickTerminalTab: (...args: unknown[]) => mockDeleteQuickTerminalTab(...args),
  updateQuickTerminalTab: (...args: unknown[]) => mockUpdateQuickTerminalTab(...args),
  listQuickTerminalTabs: (...args: unknown[]) => mockListQuickTerminalTabs(...args),
  toQuickTerminalTab: (tab: unknown) => tab,
}));

vi.mock("@/lib/api", () => ({
  createQuickTerminalTab: (...args: unknown[]) => mockCreateQuickTerminalTab(...args),
  startHostShell: (...args: unknown[]) => mockStartHostShell(...args),
  toQuickTerminalTab: (tab: unknown) => tab,
}));

vi.mock("@/components/settings/pty-terminal-view", () => ({
  PtyTerminalView: () => {
    mockPtyMount();
    mockStartHostShell();
    return null;
  },
}));

import { useQuickChatModal } from "./use-quick-chat-modal";
import { QuickTerminalTabView } from "./quick-terminal-tab-view";

const WORKSPACE_ID = "ws-1";
const TERMINAL_ONE_ID = "terminal-1";
const SESSION_ONE_ID = "session-1";
const LATE_TERMINAL_ID = "terminal-late";
const LATE_SESSION_ID = "late-session";

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
      pendingQuickChatOpen: null,
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
    setQuickChatInitialPrompt: vi.fn(),
    upsertQuickChatSessionFromEvent: vi.fn(),
    applyAgentProfileRecentUse: vi.fn(),
    setQuickChatTabOrder: vi.fn(),
    clearQuickChatTabOrder: vi.fn(),
    setQuickChatTabOrderSyncState: vi.fn(),
    agentProfiles: { items: [] },
    taskSessions: { items: {} as Record<string, { task_id: string }> },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
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
  mockListQuickTerminalTabs.mockReset();
  mockListQuickTerminalTabs.mockResolvedValue({ tabs: [] });
  mockCreateQuickTerminalTab.mockReset();
  mockCreateQuickTerminalTab.mockResolvedValue({
    tabId: TERMINAL_ONE_ID,
    workspaceId: WORKSPACE_ID,
    sessionId: null,
    sequence: 1,
    status: "connecting",
  });
  mockStartHostShell.mockReset();
  mockPtyMount.mockReset();
  mockAppState = makeAppState();
  recordRecentUseMock.mockReset();
});

function terminal(tabId: string, sessionId: string): QuickTerminalTab {
  return {
    tabId,
    workspaceId: WORKSPACE_ID,
    sessionId,
    sequence: Number(tabId.slice(-1)),
    status: "running",
  };
}

it("removes a terminal when the stop endpoint says it is already gone", async () => {
  mockAppState.quickChat.terminalTabs = [terminal(TERMINAL_ONE_ID, SESSION_ONE_ID)];
  mockDeleteQuickTerminalTab.mockRejectedValue(new ApiError("gone", 404, null));

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

it("keeps the published generation after DELETE fails so later shell exits persist", async () => {
  const tab = {
    tabId: TERMINAL_ONE_ID,
    workspaceId: WORKSPACE_ID,
    sessionId: SESSION_ONE_ID,
    sequence: 1,
    status: "running" as const,
  };
  mockAppState.quickChat.terminalTabs = [tab];
  mockDeleteQuickTerminalTab.mockRejectedValue(new Error("stop failed"));
  const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

  await act(async () => result.current.handleCloseTerminal(TERMINAL_ONE_ID));
  expect(result.current.terminalRecoveryByTab[TERMINAL_ONE_ID]).toEqual({ generation: 0 });

  act(() =>
    result.current.handleTerminalStateChange(
      TERMINAL_ONE_ID,
      { status: "exited", sessionId: SESSION_ONE_ID, exitCode: 0, error: null },
      0,
    ),
  );
  await waitFor(() =>
    expect(mockUpdateQuickTerminalTab).toHaveBeenCalledWith(TERMINAL_ONE_ID, {
      sessionId: null,
      status: "exited",
      exitCode: 0,
      error: null,
    }),
  );
});

it("suppresses descriptor creation and PTY startup while DELETE holds a tab in recovery preparation", async () => {
  const pendingList = deferred<{ tabs: QuickTerminalTab[] }>();
  const pendingDelete = deferred<void>();
  mockAppState.quickChat.terminalTabs = [
    {
      tabId: TERMINAL_ONE_ID,
      workspaceId: WORKSPACE_ID,
      sessionId: null,
      sequence: 1,
      status: "connecting",
    },
  ];
  mockListQuickTerminalTabs.mockReturnValueOnce(pendingList.promise);
  mockDeleteQuickTerminalTab.mockReturnValueOnce(pendingDelete.promise);

  const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));
  let recovery!: Promise<void>;
  act(() => {
    recovery = result.current.handleRecoverTerminal(TERMINAL_ONE_ID);
  });
  await waitFor(() => expect(mockListQuickTerminalTabs).toHaveBeenCalledTimes(1));

  let close!: Promise<void>;
  act(() => {
    close = result.current.handleCloseTerminal(TERMINAL_ONE_ID);
  });
  const recoveryState = result.current.terminalRecoveryByTab[TERMINAL_ONE_ID];
  const view = render(
    createElement(QuickTerminalTabView, {
      tab: mockAppState.quickChat.terminalTabs[0],
      onStateChange: vi.fn(),
      recoveryGeneration: recoveryState?.generation ?? 0,
      recoveryPhase: recoveryState?.phase,
      recoveryClosing: recoveryState?.closing,
    }),
  );
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });

  expect(recoveryState).toEqual({ generation: 1, closing: true });
  expect(mockCreateQuickTerminalTab).not.toHaveBeenCalled();
  expect(mockPtyMount).not.toHaveBeenCalled();
  expect(mockStartHostShell).not.toHaveBeenCalled();

  pendingList.resolve({ tabs: [] });
  await act(async () => recovery);
  pendingDelete.resolve();
  await act(async () => close);
  view.unmount();
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
    result.current.handleTerminalStateChange(
      LATE_TERMINAL_ID,
      {
        status: "running",
        sessionId: LATE_SESSION_ID,
        exitCode: null,
        error: null,
      },
      0,
    ),
  );
  await act(async () => result.current.handleCloseTerminal(LATE_TERMINAL_ID));

  expect(mockDeleteQuickTerminalTab).toHaveBeenCalledWith(LATE_TERMINAL_ID);
  expect(mockAppState.removeQuickTerminal).toHaveBeenCalledWith(LATE_TERMINAL_ID);
});

it("prepares recovery under the existing descriptor after confirming it still exists", async () => {
  const sibling: QuickTerminalTab = {
    tabId: "terminal-sibling",
    workspaceId: WORKSPACE_ID,
    sessionId: "sibling-session",
    sequence: 2,
    status: "running",
  };
  mockAppState.quickChat.terminalTabs = [
    {
      tabId: TERMINAL_ONE_ID,
      workspaceId: WORKSPACE_ID,
      sessionId: SESSION_ONE_ID,
      sequence: 4,
      status: "exited",
      exitCode: 0,
    },
    sibling,
  ];
  Object.assign(mockAppState.quickChat, {
    activeKind: "terminal",
    activeTerminalTabId: TERMINAL_ONE_ID,
  });
  mockAppState.quickChat.tabOrderByWorkspace[WORKSPACE_ID] = [sibling.tabId, TERMINAL_ONE_ID];
  mockListQuickTerminalTabs.mockResolvedValueOnce({
    tabs: [
      {
        tabId: TERMINAL_ONE_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: null,
        sequence: 4,
        status: "exited",
        exitCode: 0,
      },
    ],
  });
  const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

  await act(async () => result.current.handleRecoverTerminal(TERMINAL_ONE_ID));

  expect(mockListQuickTerminalTabs).toHaveBeenCalledWith(WORKSPACE_ID);
  expect(mockAppState.updateQuickTerminal).toHaveBeenCalledWith(TERMINAL_ONE_ID, {
    status: "connecting",
    sessionId: null,
    exitCode: null,
    error: null,
  });
  expect(result.current.terminalRecoveryByTab[TERMINAL_ONE_ID]).toEqual({
    generation: 1,
    phase: "starting",
  });
  expect(mockAppState.activateQuickTerminal).not.toHaveBeenCalled();
  expect(mockAppState.quickChat.activeTerminalTabId).toBe(TERMINAL_ONE_ID);
  expect(mockAppState.quickChat.tabOrderByWorkspace[WORKSPACE_ID]).toEqual([
    sibling.tabId,
    TERMINAL_ONE_ID,
  ]);
  expect(mockAppState.updateQuickTerminal.mock.calls.map(([tabId]) => tabId)).toEqual([
    TERMINAL_ONE_ID,
    TERMINAL_ONE_ID,
  ]);
});

it("cancels descriptor preparation when the tab is explicitly closed", async () => {
  const tab: QuickTerminalTab = {
    tabId: TERMINAL_ONE_ID,
    workspaceId: WORKSPACE_ID,
    sessionId: SESSION_ONE_ID,
    sequence: 4,
    status: "exited",
  };
  mockAppState.quickChat.terminalTabs = [tab];
  const pendingList = deferred<{
    tabs: Array<{
      tabId: string;
      workspaceId: string;
      sessionId: string | null;
      sequence: number;
      status: "exited";
    }>;
  }>();
  mockListQuickTerminalTabs.mockImplementationOnce(() => pendingList.promise);
  const { result } = renderHook(() => useQuickChatModal(WORKSPACE_ID));

  let recovery!: Promise<void>;
  act(() => {
    recovery = result.current.handleRecoverTerminal(TERMINAL_ONE_ID);
  });
  expect(result.current.terminalRecoveryByTab[TERMINAL_ONE_ID]).toEqual({
    generation: 1,
    phase: "preparing",
  });

  await act(async () => result.current.handleCloseTerminal(TERMINAL_ONE_ID));
  pendingList.resolve({
    tabs: [
      {
        tabId: TERMINAL_ONE_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: null,
        sequence: 4,
        status: "exited",
      },
    ],
  });
  await act(async () => recovery);

  expect(mockDeleteQuickTerminalTab).toHaveBeenCalledWith(TERMINAL_ONE_ID);
  expect(mockAppState.removeQuickTerminal).toHaveBeenCalledWith(TERMINAL_ONE_ID);
  expect(result.current.terminalRecoveryByTab[TERMINAL_ONE_ID]).toEqual({
    generation: 2,
    closing: true,
  });
});
