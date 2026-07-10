import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createKanbanSlice } from "./kanban-slice";
import type { KanbanSlice } from "./types";

const TASK_ID = "task-1";
const AUTO_SESSION_ID = "session-auto";
const PINNED_SESSION_ID = "session-pinned";

function makeStore() {
  return create<KanbanSlice>()(immer(createKanbanSlice));
}

describe("kanban slice active session selection", () => {
  it("updates active session state without creating a user pin", () => {
    const store = makeStore();

    store.getState().setActiveSessionAuto(TASK_ID, AUTO_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: TASK_ID,
      activeSessionId: AUTO_SESSION_ID,
      pinnedSessionId: null,
      lastSessionByTaskId: { [TASK_ID]: AUTO_SESSION_ID },
    });
  });

  it("preserves an existing pin when auto-selecting the pinned session", () => {
    const store = makeStore();

    store.getState().setActiveSession(TASK_ID, PINNED_SESSION_ID);
    store.getState().setActiveSessionAuto(TASK_ID, PINNED_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: TASK_ID,
      activeSessionId: PINNED_SESSION_ID,
      pinnedSessionId: PINNED_SESSION_ID,
      lastSessionByTaskId: { [TASK_ID]: PINNED_SESSION_ID },
    });
  });

  it("leaves non-matching pins for callers to resolve", () => {
    const store = makeStore();

    store.getState().setActiveSession(TASK_ID, PINNED_SESSION_ID);
    store.getState().setActiveSessionAuto(TASK_ID, AUTO_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: TASK_ID,
      activeSessionId: AUTO_SESSION_ID,
      pinnedSessionId: PINNED_SESSION_ID,
      lastSessionByTaskId: { [TASK_ID]: AUTO_SESSION_ID },
    });
  });
});
