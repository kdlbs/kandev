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

describe("workflow list server-state", () => {
  it("keeps only active workflow UI state in the kanban slice", () => {
    const state = makeStore().getState() as unknown as Record<string, unknown>;
    const workflows = state.workflows as Record<string, unknown>;

    expect(workflows).toEqual({ activeId: null });
    expect("setWorkflows" in state).toBe(false);
    expect("reorderWorkflowItems" in state).toBe(false);
  });

  it("keeps multi-workflow snapshots without legacy loading or mutation actions", () => {
    const state = makeStore().getState() as unknown as Record<string, unknown>;

    expect("kanban" in state).toBe(false);
    expect("kanbanMulti" in state).toBe(false);
    expect("setWorkflowSnapshot" in state).toBe(false);
    expect("setKanbanMultiLoading" in state).toBe(false);
    expect("updateMultiTask" in state).toBe(false);
    expect("removeMultiTask" in state).toBe(false);
  });
});

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
