import { describe, expect, it } from "vitest";
import { defaultState, mergeInitialState } from "./default-state";
import type { HydrationState } from "./store";

describe("turn hydration state", () => {
  it("defaults and deep-merges loaded session markers", () => {
    const state = mergeInitialState({
      turns: {
        bySession: { "session-1": [] },
        activeBySession: {},
        loadedBySession: {},
      },
    } as unknown as HydrationState);

    expect(defaultState.turns.loadedBySession).toEqual({});
    expect(state.turns.loadedBySession).toEqual({ "session-1": true });
  });
});

describe("quick chat hydration state", () => {
  it("marks an empty boot snapshot ready for the active workspace", () => {
    const state = mergeInitialState({
      workspaces: { items: [], activeId: "workspace-1" },
      quickChat: { sessions: [] },
    } as unknown as HydrationState);

    expect(state.quickChat.selectionReadyByWorkspace).toEqual({ "workspace-1": true });
  });
});

describe("failed Inbox hydration state", () => {
  it("keeps the request revision map when older persisted state omits it", () => {
    const state = mergeInitialState({
      failedInbox: {
        byWorkspaceId: {},
        generationByWorkspaceId: {},
      },
    } as unknown as HydrationState);

    expect(state.failedInbox.readRevisionByWorkspaceId).toEqual({});
  });
});
