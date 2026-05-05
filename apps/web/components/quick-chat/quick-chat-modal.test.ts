import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "@/lib/state/slices/ui/ui-slice";
import { getStoredQuickChatNames } from "@/lib/local-storage";
import type { UISlice } from "@/lib/state/slices/ui/types";

const SESSION_ID = "sess-1";
const WORKSPACE_ID = "ws-1";
const PROFILE_ID = "profile-pass";

function makeStore() {
  return create<UISlice>()(immer(createUISlice));
}

function findSession(store: ReturnType<typeof makeStore>) {
  return store.getState().quickChat.sessions.find((s) => s.sessionId === SESSION_ID);
}

describe("openQuickChat agentProfileId persistence", () => {
  it("stores agentProfileId on a new session entry", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID, PROFILE_ID);
    expect(findSession(store)?.agentProfileId).toBe(PROFILE_ID);
    expect(store.getState().quickChat.activeSessionId).toBe(SESSION_ID);
  });

  it("backfills agentProfileId on an existing session entry without one", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    expect(findSession(store)?.agentProfileId).toBeUndefined();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID, PROFILE_ID);
    expect(findSession(store)?.agentProfileId).toBe(PROFILE_ID);
  });

  it("does not duplicate sessions when reopening with the same id", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID, "profile-a");
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID, "profile-a");
    expect(
      store.getState().quickChat.sessions.filter((s) => s.sessionId === SESSION_ID),
    ).toHaveLength(1);
  });

  it("keeps existing agentProfileId when reopened without one", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID, PROFILE_ID);
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID); // no profile
    expect(findSession(store)?.agentProfileId).toBe(PROFILE_ID);
  });
});

describe("renameQuickChatSession local persistence", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("updates the session name in the store", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    store.getState().renameQuickChatSession(SESSION_ID, "My renamed chat");
    expect(findSession(store)?.name).toBe("My renamed chat");
  });

  it("persists the rename to localStorage so it survives reload", () => {
    const store = makeStore();
    store.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    store.getState().renameQuickChatSession(SESSION_ID, "Persisted name");
    expect(getStoredQuickChatNames()).toEqual({ [SESSION_ID]: "Persisted name" });
  });

  it("does not write to storage when the session does not exist in state", () => {
    const store = makeStore();
    expect(() =>
      store.getState().renameQuickChatSession("ghost-session", "Whatever"),
    ).not.toThrow();
    expect(getStoredQuickChatNames()).toEqual({});
  });
});
