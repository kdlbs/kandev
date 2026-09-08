import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  reconcileQuickTerminalTabs,
  reconcileQuickChatSessions,
  mergeHydratedQuickChatSessions,
  removeQuickChatSession,
  removeQuickChatSessionsForTask,
  upsertQuickChatSession,
} from "./quick-chat-sync";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickChatState, QuickTerminalTab } from "./types";

const mockStoredNames = vi.hoisted(() => ({ value: {} as Record<string, string> }));

vi.mock("@/lib/local-storage", () => ({
  getStoredQuickChatNames: () => mockStoredNames.value,
}));

const WS = "ws-1";
const OTHER_WS = "ws-2";
const SETUP_ID = getQuickChatSetupSessionId(WS, "chat");
const STALE_TERMINAL_ID = "stale-terminal";

function chat(sessionId: string, overrides: Partial<QuickChatSession> = {}): QuickChatSession {
  return {
    kind: "chat",
    sessionId,
    workspaceId: WS,
    taskId: `task-${sessionId}`,
    ...overrides,
  };
}

function state(sessions: QuickChatSession[], overrides: Partial<QuickChatState> = {}) {
  return {
    isOpen: true,
    sessions,
    activeSessionId: null,
    terminalTabs: [],
    activeKind: "conversation" as const,
    activeTerminalTabId: null,
    lastTerminalTabIdByWorkspace: {},
    unseenIdleByWorkspace: {},
    lastSettledAtBySession: {},
    sessionOwnership: {},
    syncRevisionByWorkspace: {},
    tombstonedSessions: {},
    tabOrderByWorkspace: {},
    tabOrderSyncErrorByWorkspace: {},
    tabOrderSyncPendingByWorkspace: {},
    ...overrides,
  };
}

beforeEach(() => {
  mockStoredNames.value = {};
});

describe("reconcileQuickChatSessions", () => {
  it("adopts tabs the server knows about and drops tabs it does not", () => {
    // The reported bug: this client only ever saw "a" and "local", another
    // device created "b" and closed "local".
    const before = state([chat("a"), chat("local")]);

    const after = reconcileQuickChatSessions(before, WS, [chat("a"), chat("b")]);

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["a", "b"]);
  });

  it("takes the server's order for the initial restore", () => {
    // Nothing on screen yet, so activity order decides where tabs land.
    const after = reconcileQuickChatSessions(state([]), WS, [chat("b"), chat("a")]);

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["b", "a"]);
  });

  it("does not reshuffle tabs already on screen when activity order changes", () => {
    // The server sorts by latest activity, which moves while sessions run. If a
    // reconnect adopted that wholesale, the strip would jump under the cursor.
    const before = state([chat("b"), chat("a")], { activeSessionId: "b" });

    const after = reconcileQuickChatSessions(before, WS, [chat("a"), chat("b")]);

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["b", "a"]);
  });

  it("appends chats it has not seen after the tabs already on screen", () => {
    const before = state([chat("b"), chat("a")]);

    const after = reconcileQuickChatSessions(before, WS, [chat("new"), chat("a"), chat("b")]);

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["b", "a", "new"]);
  });

  it("leaves other workspaces' tabs untouched", () => {
    const foreign = chat("foreign", { workspaceId: OTHER_WS });
    const before = state([foreign, chat("a")]);

    const after = reconcileQuickChatSessions(before, WS, []);

    expect(after.sessions).toEqual([foreign]);
  });

  it("keeps an unstarted setup tab, which has no backing task to sync", () => {
    const setup: QuickChatSession = { kind: "chat", sessionId: SETUP_ID, workspaceId: WS };
    const before = state([setup, chat("a")], { activeSessionId: SETUP_ID });

    const after = reconcileQuickChatSessions(before, WS, [chat("a")]);

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["a", SETUP_ID]);
    expect(after.activeSessionId).toBe(SETUP_ID);
  });

  it("keeps client-only draft state on a tab that survives", () => {
    const before = state([chat("a", { initialPrompt: "unsent draft" })]);

    const after = reconcileQuickChatSessions(before, WS, [chat("a")]);

    expect(after.sessions[0].initialPrompt).toBe("unsent draft");
  });

  it("lets a local rename win over the server's task title", () => {
    mockStoredNames.value = { a: "My name for it" };
    const before = state([chat("a")]);

    const after = reconcileQuickChatSessions(before, WS, [chat("a", { name: "Server Title" })]);

    expect(after.sessions[0].name).toBe("My name for it");
  });

  it("re-points the active tab when the server dropped the one in view", () => {
    const before = state([chat("a"), chat("b")], { activeSessionId: "a" });

    const after = reconcileQuickChatSessions(before, WS, [chat("b")]);

    expect(after.activeSessionId).toBe("b");
    expect(after.isOpen).toBe(true);
  });

  it("closes the modal when the workspace has no tabs left", () => {
    const before = state([chat("a")], { activeSessionId: "a" });

    const after = reconcileQuickChatSessions(before, WS, []);

    expect(after.sessions).toEqual([]);
    expect(after.activeSessionId).toBeNull();
    expect(after.isOpen).toBe(false);
  });
});

describe("reconcileQuickTerminalTabs", () => {
  function terminal(tabId: string, overrides: Partial<QuickTerminalTab> = {}): QuickTerminalTab {
    return {
      tabId,
      workspaceId: WS,
      sessionId: `pty-${tabId}`,
      sequence: 1,
      status: "running",
      ...overrides,
    };
  }

  it("restores server descriptors and drops established tabs the server removed", () => {
    const pending = terminal("pending", { sessionId: null, status: "connecting", sequence: 3 });
    const removed = terminal("removed", { sequence: 1 });
    const foreign = terminal("foreign", { workspaceId: OTHER_WS, sequence: 4 });
    const before = state([], {
      terminalTabs: [foreign, removed, pending],
      activeKind: "terminal",
      activeTerminalTabId: removed.tabId,
      lastTerminalTabIdByWorkspace: { [WS]: removed.tabId, [OTHER_WS]: foreign.tabId },
    });
    const restored = terminal("restored", { sequence: 2, sessionId: "pty-restored" });

    const after = reconcileQuickTerminalTabs(before, WS, [restored]);

    expect(after.terminalTabs).toEqual([foreign, restored, pending]);
    expect(after.terminalTabs).not.toContainEqual(removed);
    expect(after.activeKind).toBe("terminal");
    expect(after.activeTerminalTabId).toBe(restored.tabId);
    expect(after.lastTerminalTabIdByWorkspace[WS]).toBe(restored.tabId);
    expect(after.lastTerminalTabIdByWorkspace[OTHER_WS]).toBe(foreign.tabId);
  });

  it("uses a conversation or closes when the active workspace has no terminal left", () => {
    const before = state([chat("chat-a")], {
      activeKind: "terminal",
      activeSessionId: "chat-a",
      activeTerminalTabId: "removed",
      terminalTabs: [terminal("removed")],
      lastTerminalTabIdByWorkspace: { [WS]: "removed" },
    });

    const conversation = reconcileQuickTerminalTabs(before, WS, []);
    expect(conversation.activeKind).toBe("conversation");
    expect(conversation.activeSessionId).toBe("chat-a");
    expect(conversation.isOpen).toBe(true);

    const closed = reconcileQuickTerminalTabs({ ...before, sessions: [] }, WS, []);
    expect(closed.isOpen).toBe(false);
    expect(closed.activeSessionId).toBeNull();
  });
});

describe("reconcileQuickChatSessions — selection preservation", () => {
  it("leaves a never-selected active tab unset instead of promoting one", () => {
    // A background resync must not decide which tab is "current" for a user who
    // has not opened quick chat at all.
    const before = state([], { isOpen: false, activeSessionId: null });

    const after = reconcileQuickChatSessions(before, WS, [chat("a"), chat("b")]);

    expect(after.sessions).toHaveLength(2);
    expect(after.activeSessionId).toBeNull();
    expect(after.isOpen).toBe(false);
  });

  it("does not disturb an active tab belonging to another workspace", () => {
    const foreign = chat("foreign", { workspaceId: OTHER_WS });
    const before = state([foreign, chat("a")], { activeSessionId: "foreign" });

    const after = reconcileQuickChatSessions(before, WS, []);

    expect(after.activeSessionId).toBe("foreign");
    expect(after.isOpen).toBe(true);
  });

  it("preserves local terminal tabs and active terminal selection during resync", () => {
    const terminal: QuickTerminalTab = {
      tabId: "terminal-a",
      workspaceId: WS,
      sessionId: "pty-a",
      sequence: 1,
      status: "running",
    };
    const before = state([], {
      activeKind: "terminal",
      activeTerminalTabId: terminal.tabId,
      terminalTabs: [terminal],
      lastTerminalTabIdByWorkspace: { [WS]: terminal.tabId },
    });

    const after = reconcileQuickChatSessions(before, WS, []);

    expect(after.terminalTabs).toEqual([terminal]);
    expect(after.activeKind).toBe("terminal");
    expect(after.activeTerminalTabId).toBe(terminal.tabId);
    expect(after.isOpen).toBe(true);
  });

  it("falls back to a local terminal when the active conversation disappears", () => {
    const terminal: QuickTerminalTab = {
      tabId: "terminal-a",
      workspaceId: WS,
      sessionId: "pty-a",
      sequence: 1,
      status: "running",
    };
    const before = state([chat("a")], {
      activeSessionId: "a",
      terminalTabs: [terminal],
    });

    const after = reconcileQuickChatSessions(before, WS, []);

    expect(after.activeKind).toBe("terminal");
    expect(after.activeTerminalTabId).toBe(terminal.tabId);
    expect(after.isOpen).toBe(true);
  });
});

describe("reconcileQuickChatSessions — stale terminal recovery", () => {
  it("recovers a stale terminal selection within its previous workspace", () => {
    const workspaceTerminal: QuickTerminalTab = {
      tabId: "terminal-a",
      workspaceId: WS,
      sessionId: "pty-a",
      sequence: 1,
      status: "running",
    };
    const otherWorkspaceTerminal: QuickTerminalTab = {
      ...workspaceTerminal,
      tabId: "terminal-b",
      workspaceId: OTHER_WS,
    };
    const before = state([chat("chat-a")], {
      activeKind: "terminal",
      activeSessionId: "chat-a",
      activeTerminalTabId: STALE_TERMINAL_ID,
      terminalTabs: [otherWorkspaceTerminal, workspaceTerminal],
      lastTerminalTabIdByWorkspace: {
        [WS]: STALE_TERMINAL_ID,
        [OTHER_WS]: otherWorkspaceTerminal.tabId,
      },
    });

    const after = reconcileQuickChatSessions(before, WS, [chat("chat-a")]);

    expect(after.activeKind).toBe("terminal");
    expect(after.activeTerminalTabId).toBe(workspaceTerminal.tabId);
  });

  it("switches back to conversation mode when a stale terminal has no replacement", () => {
    const before = state([chat("chat-a")], {
      activeKind: "terminal",
      activeSessionId: "chat-a",
      activeTerminalTabId: STALE_TERMINAL_ID,
      lastTerminalTabIdByWorkspace: { [WS]: STALE_TERMINAL_ID },
    });

    const after = reconcileQuickChatSessions(before, WS, [chat("chat-a")]);

    expect(after.activeKind).toBe("conversation");
    expect(after.activeSessionId).toBe("chat-a");
  });

  it("closes when stale terminal recovery has no conversation or terminal left", () => {
    const before = state([], {
      isOpen: true,
      activeKind: "terminal",
      activeTerminalTabId: STALE_TERMINAL_ID,
      lastTerminalTabIdByWorkspace: { [WS]: STALE_TERMINAL_ID },
    });

    const after = reconcileQuickChatSessions(before, WS, []);

    expect(after.activeKind).toBe("conversation");
    expect(after.activeSessionId).toBeNull();
    expect(after.isOpen).toBe(false);
  });
});

describe("upsertQuickChatSession", () => {
  it("appends a chat started on another device", () => {
    const after = upsertQuickChatSession(state([chat("a")]), chat("b"));

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["a", "b"]);
  });

  it("never steals focus or opens the modal", () => {
    const before = state([chat("a")], { isOpen: false, activeSessionId: "a" });

    const after = upsertQuickChatSession(before, chat("b"));

    expect(after.isOpen).toBe(false);
    expect(after.activeSessionId).toBe("a");
  });

  it("updates a known tab in place rather than duplicating it", () => {
    const before = state([chat("a", { name: "Old" })]);

    const after = upsertQuickChatSession(before, chat("a", { name: "Renamed elsewhere" }));

    expect(after.sessions).toHaveLength(1);
    expect(after.sessions[0].name).toBe("Renamed elsewhere");
  });

  it("keeps this device's rename when a remote update carries the old title", () => {
    mockStoredNames.value = { a: "Mine" };
    const before = state([chat("a", { name: "Mine" })]);

    const after = upsertQuickChatSession(before, chat("a", { name: "Server Title" }));

    expect(after.sessions[0].name).toBe("Mine");
  });

  it("accepts a session once its tombstone has expired", () => {
    const expiredAt = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    const before = state([], {
      tombstonedSessions: { a: { workspaceId: WS, tombstonedAt: expiredAt } },
    });

    const after = upsertQuickChatSession(before, chat("a"));

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["a"]);
    expect(after.tombstonedSessions).toEqual({});
  });
});

describe("mergeHydratedQuickChatSessions", () => {
  it("accepts a session when its tombstone has expired", () => {
    const expiredAt = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    const before = state([], {
      tombstonedSessions: { a: { workspaceId: WS, tombstonedAt: expiredAt } },
    });

    const after = mergeHydratedQuickChatSessions(before, [chat("a")]);

    expect(after.sessions.map((session) => session.sessionId)).toEqual(["a"]);
    expect(after.tombstonedSessions).toEqual({});
  });
});

describe("removeQuickChatSessionsForTask", () => {
  it("drops the tab whose task was deleted elsewhere", () => {
    const before = state([chat("a"), chat("b")], { activeSessionId: "b" });

    const after = removeQuickChatSessionsForTask(before, "task-b");

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["a"]);
    expect(after.activeSessionId).toBe("a");
  });

  it("keeps a null active tab null while removing others", () => {
    const before = state([chat("a"), chat("b")], { activeSessionId: null });

    const after = removeQuickChatSessionsForTask(before, "task-b");

    expect(after.sessions.map((s) => s.sessionId)).toEqual(["a"]);
    expect(after.activeSessionId).toBeNull();
  });

  it("removes lifecycle state using ownership after its tab has disappeared", () => {
    const before = state([], {
      sessionOwnership: { removed: { workspaceId: WS, taskId: "task-removed" } },
      unseenIdleByWorkspace: { [WS]: { removed: true } },
      lastSettledAtBySession: { removed: "2026-08-17T07:00:00Z" },
    });

    const after = removeQuickChatSessionsForTask(before, "task-removed");

    expect(after.sessionOwnership).toEqual({});
    expect(after.unseenIdleByWorkspace).toEqual({});
    expect(after.lastSettledAtBySession).toEqual({});
  });

  it("clears a pre-existing unseen marker when the session is removed directly", () => {
    const before = state([chat("a")], {
      unseenIdleByWorkspace: { [WS]: { a: true } },
    });

    const after = removeQuickChatSession(before, "a");

    expect(after.unseenIdleByWorkspace).toEqual({});
    expect(after.sessions.map((s) => s.sessionId)).toEqual([]);
  });

  it("returns the same state when no tab is affected", () => {
    const before = state([chat("a")]);

    expect(removeQuickChatSessionsForTask(before, "task-unknown")).toBe(before);
  });

  it("ignores setup tabs, which have no task id", () => {
    const setup: QuickChatSession = { kind: "chat", sessionId: SETUP_ID, workspaceId: WS };
    const before = state([setup]);

    expect(removeQuickChatSessionsForTask(before, "task-a")).toBe(before);
  });
});
