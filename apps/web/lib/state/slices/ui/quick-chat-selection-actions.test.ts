import { describe, expect, it, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";
import { createAppStore } from "@/lib/state/store";

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const CHAT_FIRST_ID = "chat-first";
const CHAT_SECOND_ID = "chat-second";
const SESSION_A = "session-a";
const SESSION_B = "session-b";

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

beforeEach(() => {
  window.localStorage.clear();
});

describe("Quick Chat remembered selection actions", () => {
  it("persists chat and config selections separately for each workspace", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");

    store.getState().openQuickChat("chat-a", WORKSPACE_A, undefined, "chat");
    store.getState().openQuickChat("config-a", WORKSPACE_A, undefined, "config");
    store.getState().openQuickChat("chat-b", WORKSPACE_B, undefined, "chat");

    expect(store.getState().quickChat.rememberedSelectionByWorkspace).toEqual({
      [WORKSPACE_A]: { chat: "chat-a", config: "config-a" },
      [WORKSPACE_B]: { chat: "chat-b" },
    });

    const reloaded = makeStore();
    reloaded.getState().setQuickChatSelectionIdentity("user-a");
    expect(reloaded.getState().quickChat.rememberedSelectionByWorkspace).toEqual(
      store.getState().quickChat.rememberedSelectionByWorkspace,
    );
  });

  it("does not let terminal activation change the remembered conversation", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat("chat-a", WORKSPACE_A);
    const before = store.getState().quickChat.rememberedSelectionByWorkspace;

    store.getState().createQuickTerminal(WORKSPACE_A);

    expect(store.getState().quickChat.rememberedSelectionByWorkspace).toEqual(before);
  });

  it("does not persist a temporary setup tab as a remembered conversation", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat("", WORKSPACE_A);

    expect(store.getState().quickChat.rememberedSelectionByWorkspace).toEqual({});
    expect(window.localStorage.getItem("kandev.quick-chat.selection.v1.user-a")).toBe(
      '{"version":1,"entries":[]}',
    );
  });

  it("resolves a pending open to the remembered tab after the workspace list arrives", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat("chat-remembered", WORKSPACE_A);
    store.getState().closeQuickChat();
    store.getState().requestQuickChatOpen(WORKSPACE_A);

    store.getState().syncQuickChatSessions(WORKSPACE_A, [
      { sessionId: CHAT_FIRST_ID, workspaceId: WORKSPACE_A, kind: "chat" },
      { sessionId: "chat-remembered", workspaceId: WORKSPACE_A, kind: "chat" },
    ]);

    expect(store.getState().quickChat).toMatchObject({
      activeSessionId: "chat-remembered",
      pendingOpen: null,
      isOpen: true,
      selectionReadyByWorkspace: { [WORKSPACE_A]: true },
    });
  });

  it("falls back and clears a missing remembered tab when the list arrives", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat("chat-removed", WORKSPACE_A);
    store.getState().closeQuickChat();
    store.getState().requestQuickChatOpen(WORKSPACE_A);

    store
      .getState()
      .syncQuickChatSessions(WORKSPACE_A, [
        { sessionId: "chat-first", workspaceId: WORKSPACE_A, kind: "chat" },
      ]);

    expect(store.getState().quickChat).toMatchObject({
      activeSessionId: CHAT_FIRST_ID,
      pendingOpen: null,
      isOpen: true,
    });
    expect(store.getState().quickChat.rememberedSelectionByWorkspace).toEqual({});
  });
});

describe("Quick Chat selection races", () => {
  it("uses the captured persisted order after a remembered tab is removed", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat("chat-removed", WORKSPACE_A);
    store.getState().closeQuickChat();
    store
      .getState()
      .requestQuickChatOpen(WORKSPACE_A, "chat", [
        `conversation:${CHAT_SECOND_ID}`,
        `conversation:${CHAT_FIRST_ID}`,
      ]);

    store.getState().syncQuickChatSessions(WORKSPACE_A, [
      { sessionId: CHAT_FIRST_ID, workspaceId: WORKSPACE_A, kind: "chat" },
      { sessionId: CHAT_SECOND_ID, workspaceId: WORKSPACE_A, kind: "chat" },
    ]);

    expect(store.getState().quickChat.activeSessionId).toBe(CHAT_SECOND_ID);
    expect(store.getState().quickChat.rememberedSelectionByWorkspace).toEqual({});
  });

  it("uses user settings order when a delayed open has no optimistic order", () => {
    const store = createAppStore();
    store.getState().setUserSettings({
      ...store.getState().userSettings,
      quickChatTabOrderByWorkspace: {
        [WORKSPACE_A]: [`conversation:${CHAT_SECOND_ID}`, `conversation:${CHAT_FIRST_ID}`],
      },
    });
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat("chat-removed", WORKSPACE_A);
    store.getState().closeQuickChat();
    store.getState().requestQuickChatOpen(WORKSPACE_A);

    store.getState().syncQuickChatSessions(WORKSPACE_A, [
      { sessionId: CHAT_FIRST_ID, workspaceId: WORKSPACE_A, kind: "chat" },
      { sessionId: CHAT_SECOND_ID, workspaceId: WORKSPACE_A, kind: "chat" },
    ]);

    expect(store.getState().quickChat.activeSessionId).toBe(CHAT_SECOND_ID);
  });

  it("preserves accepted boot readiness while loading the selection identity", () => {
    const store = createAppStore({
      workspaces: { items: [], activeId: WORKSPACE_A },
      quickChat: { sessions: [] },
    });

    expect(store.getState().quickChat.selectionReadyByWorkspace).toEqual({
      [WORKSPACE_A]: true,
    });
  });

  it("remembers the replacement when the selected tab is closed", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A);
    store.getState().openQuickChat(SESSION_B, WORKSPACE_A);
    store.getState().setActiveQuickChatSession(SESSION_A, WORKSPACE_A);

    store.getState().closeQuickChatSession(SESSION_A);

    expect(store.getState().quickChat).toMatchObject({
      activeSessionId: SESSION_B,
      rememberedSelectionByWorkspace: { [WORKSPACE_A]: { chat: SESSION_B } },
    });
  });

  it("invalidates a pending open when its remembered tab is removed", () => {
    const store = makeStore();
    store.getState().setQuickChatSelectionIdentity("user-a");
    store.getState().openQuickChat(SESSION_A, WORKSPACE_A);
    store.getState().closeQuickChat();
    store.getState().requestQuickChatOpen(WORKSPACE_A);

    store.getState().closeQuickChatSession(SESSION_A);

    expect(store.getState().quickChat.pendingOpen).toBeNull();
  });

  it("cancels a pending open when another workspace finishes syncing", () => {
    const store = makeStore();
    store.getState().requestQuickChatOpen(WORKSPACE_A);

    store
      .getState()
      .syncQuickChatSessions(WORKSPACE_B, [
        { sessionId: SESSION_B, workspaceId: WORKSPACE_B, kind: "chat" },
      ]);

    expect(store.getState().quickChat.pendingOpen).toBeNull();
  });
});
