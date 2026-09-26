import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
import type { UISlice } from "./types";

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const SESSION_A = "session-a";
const SESSION_B = "session-b";

type TerminalActions = {
  reuseOrCreateQuickTerminal: (workspaceId: string) => string;
  createQuickTerminal: (workspaceId: string) => string;
  activateQuickTerminal: (tabId: string, workspaceId: string) => void;
  updateQuickTerminal: (
    tabId: string,
    update: {
      sessionId?: string | null;
      status?: "connecting" | "running" | "exited" | "error";
      exitCode?: number;
      error?: string;
    },
  ) => void;
  removeQuickTerminal: (tabId: string) => void;
};

function withTerminalActions(store: ReturnType<typeof makeStore>) {
  return store.getState() as typeof store extends { getState: () => infer State }
    ? State & TerminalActions
    : never;
}

function terminalState(store: ReturnType<typeof makeStore>) {
  return store.getState().quickChat as unknown as {
    terminalTabs: Array<{
      tabId: string;
      workspaceId: string;
      sessionId: string | null;
      sequence: number;
      status: "connecting" | "running" | "exited" | "error";
      exitCode?: number;
      error?: string;
    }>;
    activeKind: "conversation" | "terminal";
    activeTerminalTabId: string | null;
    lastTerminalTabIdByWorkspace: Record<string, string>;
  };
}

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

// eslint-disable-next-line max-lines-per-function -- related quick-chat session state transitions share a store fixture.
describe("typed quick chat sessions", () => {
  it("registers a shared session without opening the Quick Chat dialog", () => {
    const store = makeStore();

    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A, "agent-a", "config");
    store.getState().setQuickChatInitialPrompt(SESSION_A, "Configure my workflow");

    expect(store.getState().quickChat).toMatchObject({
      isOpen: false,
      activeSessionId: SESSION_A,
      sessions: [
        {
          sessionId: SESSION_A,
          workspaceId: WORKSPACE_A,
          kind: "config",
          initialPrompt: "Configure my workflow",
        },
      ],
    });
  });

  it("preserves task ownership when reopening or refreshing an existing session", () => {
    const store = makeStore();
    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A, "agent-a", "chat", "task-a");
    store.getState().closeQuickChat();

    store.getState().openQuickChat(SESSION_A, WORKSPACE_A);
    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A);

    expect(store.getState().quickChat.sessionOwnership[SESSION_A]).toEqual({
      workspaceId: WORKSPACE_A,
      taskId: "task-a",
    });
  });

  it("clears every workspace's unseen markers when the dialog opens", () => {
    const store = makeStore();
    store.getState().markQuickChatUnseenIdle("session-a", WORKSPACE_A);
    store.getState().markQuickChatUnseenIdle("session-b", WORKSPACE_B);

    store.getState().openQuickChat(SESSION_A, WORKSPACE_A);

    expect(store.getState().quickChat.unseenIdleByWorkspace).toEqual({});
  });

  it("prunes the workspace marker bucket when the active session is selected", () => {
    const store = makeStore();
    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A);
    store.getState().markQuickChatUnseenIdle(SESSION_A, WORKSPACE_A);

    store.getState().setActiveQuickChatSession(SESSION_A, WORKSPACE_A);

    expect(store.getState().quickChat.unseenIdleByWorkspace).toEqual({});
  });

  it("keeps unseen markers when opening a session owned by another workspace fails", () => {
    const store = makeStore();
    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A);
    store.getState().markQuickChatUnseenIdle(SESSION_A, WORKSPACE_A);

    store.getState().openQuickChat(SESSION_A, WORKSPACE_B);

    expect(store.getState().quickChat.isOpen).toBe(false);
    expect(store.getState().quickChat.unseenIdleByWorkspace).toEqual({
      [WORKSPACE_A]: { [SESSION_A]: true },
    });
  });

  it("opens a configuration setup tab in the unified quick chat store", () => {
    const store = makeStore();
    const setupId = getQuickChatSetupSessionId(WORKSPACE_A, "config");

    store.getState().openQuickChat("", WORKSPACE_A, undefined, "config");

    expect(store.getState().quickChat).toMatchObject({
      isOpen: true,
      activeSessionId: setupId,
      sessions: [{ sessionId: setupId, workspaceId: WORKSPACE_A, kind: "config" }],
    });
  });

  it("keeps ordinary and configuration sessions together without cross-workspace activation", () => {
    const store = makeStore();

    store.getState().openQuickChat("chat-a", WORKSPACE_A, "agent-a", "chat");
    store.getState().openQuickChat("config-a", WORKSPACE_A, "agent-config", "config");
    store.getState().openQuickChat("chat-b", WORKSPACE_B, "agent-b", "chat");
    store.getState().setActiveQuickChatSession("config-a", WORKSPACE_B);

    expect(store.getState().quickChat.sessions).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ sessionId: "chat-a", kind: "chat" }),
        expect.objectContaining({ sessionId: "config-a", kind: "config" }),
      ]),
    );
    expect(store.getState().quickChat.activeSessionId).toBe("chat-b");
  });

  it("keeps setup tabs distinct by workspace and kind", () => {
    const store = makeStore();

    store.getState().openQuickChat("", WORKSPACE_A, undefined, "config");
    store.getState().openQuickChat("", WORKSPACE_A, undefined, "chat");
    store.getState().openQuickChat("", WORKSPACE_B, undefined, "config");

    const setupTabs = store.getState().quickChat.sessions;
    expect(setupTabs).toHaveLength(3);
    expect(new Set(setupTabs.map((session) => session.sessionId)).size).toBe(3);
    expect(setupTabs).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ workspaceId: WORKSPACE_A, kind: "config" }),
        expect.objectContaining({ workspaceId: WORKSPACE_A, kind: "chat" }),
        expect.objectContaining({ workspaceId: WORKSPACE_B, kind: "config" }),
      ]),
    );
  });

  it("does not open the modal for an existing session from another workspace", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A, undefined, "chat");
    store.getState().closeQuickChat();

    store.getState().openQuickChat(SESSION_A, WORKSPACE_B, undefined, "chat");

    expect(store.getState().quickChat.isOpen).toBe(false);
    expect(store.getState().quickChat.activeSessionId).toBe(SESSION_A);
  });

  it("does not activate a registered session from another workspace while quick chat is open", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_B, WORKSPACE_B, undefined, "chat");

    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A, "agent-a", "config");

    expect(store.getState().quickChat).toMatchObject({
      isOpen: true,
      activeSessionId: SESSION_B,
      sessions: expect.arrayContaining([
        expect.objectContaining({ sessionId: SESSION_A, workspaceId: WORKSPACE_A }),
        expect.objectContaining({ sessionId: SESSION_B, workspaceId: WORKSPACE_B }),
      ]),
    });
  });

  it("does not activate another workspace after the last local tab closes", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_B, WORKSPACE_B, undefined, "chat");
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A, undefined, "config");

    store.getState().closeQuickChatSession(SESSION_A);

    expect(store.getState().quickChat).toMatchObject({
      isOpen: false,
      activeSessionId: null,
      sessions: [expect.objectContaining({ sessionId: SESSION_B })],
    });
  });
});

describe("configuration quick chat uniqueness", () => {
  it("opens the existing configuration session instead of creating another setup", () => {
    const store = makeStore();
    store.getState().addQuickChatSession(SESSION_A, WORKSPACE_A, "agent-a", "config");

    store.getState().openQuickChat("", WORKSPACE_A, undefined, "config");

    expect(store.getState().quickChat).toMatchObject({
      isOpen: true,
      activeSessionId: SESSION_A,
      sessions: [expect.objectContaining({ sessionId: SESSION_A, kind: "config" })],
    });
  });
});

describe("quick terminal tabs", () => {
  it("reuses the workspace terminal and always-new appends a distinct tab", () => {
    const store = makeStore();
    const actions = withTerminalActions(store);

    const firstId = actions.reuseOrCreateQuickTerminal(WORKSPACE_A);
    const reusedId = actions.reuseOrCreateQuickTerminal(WORKSPACE_A);
    const secondId = actions.createQuickTerminal(WORKSPACE_A);

    expect(reusedId).toBe(firstId);
    expect(secondId).not.toBe(firstId);
    expect(terminalState(store).terminalTabs.map((tab) => tab.sequence)).toEqual([1, 2]);
    expect(terminalState(store).activeTerminalTabId).toBe(secondId);
    expect(terminalState(store).activeKind).toBe("terminal");
  });

  it("keeps terminal selection per workspace and does not overwrite the last terminal", () => {
    const store = makeStore();
    const actions = withTerminalActions(store);
    const firstTerminal = actions.createQuickTerminal(WORKSPACE_A);
    const workspaceBTerminal = actions.createQuickTerminal(WORKSPACE_B);

    store.getState().openQuickChat(SESSION_A, WORKSPACE_A, undefined, "chat");
    actions.activateQuickTerminal(firstTerminal, WORKSPACE_A);
    store.getState().setActiveQuickChatSession(SESSION_A, WORKSPACE_A);

    expect(terminalState(store).activeKind).toBe("conversation");
    expect(terminalState(store).activeTerminalTabId).toBe(firstTerminal);
    expect(terminalState(store).lastTerminalTabIdByWorkspace).toMatchObject({
      [WORKSPACE_A]: firstTerminal,
      [WORKSPACE_B]: workspaceBTerminal,
    });
  });

  it("updates lifecycle state without changing terminal identity", () => {
    const store = makeStore();
    const actions = withTerminalActions(store);
    const tabId = actions.createQuickTerminal(WORKSPACE_A);

    actions.updateQuickTerminal(tabId, {
      sessionId: "pty-1",
      status: "running",
    });
    actions.updateQuickTerminal(tabId, {
      status: "exited",
      exitCode: 7,
    });

    expect(terminalState(store).terminalTabs[0]).toMatchObject({
      tabId,
      sessionId: "pty-1",
      status: "exited",
      exitCode: 7,
    });
  });

  it("falls back to adjacent terminals, then a conversation, and finally closes", () => {
    const store = makeStore();
    const actions = withTerminalActions(store);
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A, undefined, "chat");
    const firstId = actions.createQuickTerminal(WORKSPACE_A);
    const secondId = actions.createQuickTerminal(WORKSPACE_A);

    actions.removeQuickTerminal(secondId);
    expect(terminalState(store).activeTerminalTabId).toBe(firstId);
    expect(terminalState(store).activeKind).toBe("terminal");

    actions.removeQuickTerminal(firstId);
    expect(store.getState().quickChat.activeSessionId).toBe(SESSION_A);
    expect(terminalState(store).activeKind).toBe("conversation");

    store.getState().closeQuickChatSession(SESSION_A);
    expect(store.getState().quickChat.isOpen).toBe(false);
  });

  it("keeps the terminal selected when its retained conversation closes", () => {
    const store = makeStore();
    const actions = withTerminalActions(store);
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A, undefined, "chat");
    store.getState().openQuickChat(SESSION_B, WORKSPACE_A, undefined, "chat");
    const terminalId = actions.createQuickTerminal(WORKSPACE_A);

    store.getState().closeQuickChatSession(SESSION_B);

    expect(terminalState(store).activeKind).toBe("terminal");
    expect(terminalState(store).activeTerminalTabId).toBe(terminalId);
    expect(store.getState().quickChat.isOpen).toBe(true);
  });

  it("falls back to the final conversation in rendered order", () => {
    const store = makeStore();
    const actions = withTerminalActions(store);
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A, undefined, "chat");
    store.getState().openQuickChat(SESSION_B, WORKSPACE_A, undefined, "config");
    const terminalId = actions.createQuickTerminal(WORKSPACE_A);

    actions.removeQuickTerminal(terminalId);

    expect(store.getState().quickChat.activeSessionId).toBe(SESSION_B);
    expect(terminalState(store).activeKind).toBe("conversation");
  });
});
