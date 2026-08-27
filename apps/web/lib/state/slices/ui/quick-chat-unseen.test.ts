import { describe, expect, it } from "vitest";
import type { StoreApi } from "zustand";
import { maybeMarkQuickChatUnseenIdle } from "@/lib/ws/handlers/quick-chat-unseen";
import type { AppState } from "@/lib/state/store";
import { createAppStore } from "@/lib/state/store";
import { selectQuickChatHasUnseenIdle } from "./quick-chat-unseen-selectors";
type QuickChatWithUnseenMarkers = {
  unseenIdleByWorkspace: Record<string, Record<string, true>>;
};
const SESSION_ID = "session-1";
const WORKSPACE_ID = "workspace-1";
const SETTLED_AT = "2099-01-01T00:00:00Z";
const OTHER_SESSION_ID = "session-2";
const OTHER_WORKSPACE_ID = "workspace-2";

describe("quick chat unseen idle markers", () => {
  it("clears all markers when opening quick chat", () => {
    const store = createAppStore();
    store.setState({
      quickChat: {
        ...store.getState().quickChat,
        unseenIdleByWorkspace: { [WORKSPACE_ID]: { [SESSION_ID]: true } },
      } as never,
    });

    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);

    expect(
      (store.getState().quickChat as unknown as QuickChatWithUnseenMarkers).unseenIdleByWorkspace,
    ).toEqual({});
  });

  it("scopes markers to their workspace and clears an individual session", () => {
    const store = createAppStore();

    store.getState().markQuickChatUnseenIdle("session-a", "workspace-a");
    store.getState().markQuickChatUnseenIdle("session-b", "workspace-b");
    store.getState().clearQuickChatUnseenIdle("session-a", "workspace-a");

    expect(selectQuickChatHasUnseenIdle(store.getState(), "workspace-a")).toBe(false);
    expect(selectQuickChatHasUnseenIdle(store.getState(), "workspace-b")).toBe(true);
    expect(selectQuickChatHasUnseenIdle(store.getState(), null)).toBe(false);
  });
});

function settle(store: StoreApi<AppState>, sessionId: string, updatedAt: string) {
  maybeMarkQuickChatUnseenIdle(store, sessionId, {
    previousState: "RUNNING",
    fallbackPreviousState: undefined,
    newState: "IDLE",
    updatedAt,
  });
}

describe("quick chat unseen idle WebSocket scheduling", () => {
  it("marks a closed quick chat after an active-to-settled transition", () => {
    const store = createAppStore();
    store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);

    settle(store, SESSION_ID, SETTLED_AT);

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(true);
  });

  it("suppresses a completion observed while quick chat is open", () => {
    const store = createAppStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);

    settle(store, SESSION_ID, SETTLED_AT);
    store.getState().closeQuickChat();
    settle(store, SESSION_ID, SETTLED_AT);

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(false);
  });

  it("suppresses a completion outside the visible quick-chat dialog workspace", () => {
    const store = createAppStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    store.getState().addQuickChatSession(OTHER_SESSION_ID, OTHER_WORKSPACE_ID);

    settle(store, OTHER_SESSION_ID, SETTLED_AT);

    expect(selectQuickChatHasUnseenIdle(store.getState(), OTHER_WORKSPACE_ID)).toBe(false);
  });

  it("does not re-mark after a cleared duplicate completion", () => {
    const store = createAppStore();
    store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);
    settle(store, SESSION_ID, SETTLED_AT);
    store.getState().clearQuickChatUnseenIdle(SESSION_ID, WORKSPACE_ID);

    settle(store, SESSION_ID, SETTLED_AT);

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(false);
  });

  it("does not re-mark the state_changed echo of a turn completion the user viewed", () => {
    const store = createAppStore();
    store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);
    // The turn.completed path records completed_at into the settled ledger.
    store.getState().recordQuickChatSettled(SESSION_ID, "2099-01-01T00:00:00Z");
    store.getState().markQuickChatUnseenIdle(SESSION_ID, WORKSPACE_ID);
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    store.getState().closeQuickChat();
    // The corresponding state_changed arrives moments later with a newer
    // session updated_at; it is the same logical completion.
    settle(store, SESSION_ID, "2099-01-01T00:00:01Z");

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(false);
  });

  it("marks a genuinely new settle that lands outside the echo grace window", () => {
    const store = createAppStore();
    store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);
    store.getState().recordQuickChatSettled(SESSION_ID, "2099-01-01T00:00:00Z");
    store.getState().markQuickChatUnseenIdle(SESSION_ID, WORKSPACE_ID);
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    store.getState().closeQuickChat();

    settle(store, SESSION_ID, "2099-01-01T00:10:00Z");

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(true);
  });

  it("does not mark a completion received before its tab is known", () => {
    const store = createAppStore();
    settle(store, SESSION_ID, SETTLED_AT);
    store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);
    settle(store, SESSION_ID, SETTLED_AT);

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(false);
  });

  it("does not mark a late completion for a removed quick chat", () => {
    const store = createAppStore();
    store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);
    store.getState().removeQuickChatSession(SESSION_ID);

    settle(store, SESSION_ID, SETTLED_AT);

    expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(false);
  });
});
