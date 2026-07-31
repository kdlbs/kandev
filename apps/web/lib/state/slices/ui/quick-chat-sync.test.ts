import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  reconcileQuickChatSessions,
  removeQuickChatSessionsForTask,
  upsertQuickChatSession,
} from "./quick-chat-sync";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickChatState } from "./types";

const mockStoredNames = vi.hoisted(() => ({ value: {} as Record<string, string> }));

vi.mock("@/lib/local-storage", () => ({
  getStoredQuickChatNames: () => mockStoredNames.value,
}));

const WS = "ws-1";
const OTHER_WS = "ws-2";
const SETUP_ID = getQuickChatSetupSessionId(WS, "chat");

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
  return { isOpen: true, sessions, activeSessionId: null, ...overrides };
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
