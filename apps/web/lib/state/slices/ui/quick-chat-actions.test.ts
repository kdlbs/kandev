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

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

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
