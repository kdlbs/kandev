import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createKanbanSlice } from "./kanban-slice";
import type { KanbanSlice } from "./types";

function makeStore() {
  return create<KanbanSlice>()(immer(createKanbanSlice));
}

describe("kanban slice active session selection", () => {
  it("clears a stale user pin when an automatic session switch takes over", () => {
    const store = makeStore();

    store.getState().setActiveSession("task-1", "session-pinned");
    store.getState().setActiveSessionAuto("task-1", "session-auto");

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: "task-1",
      activeSessionId: "session-auto",
      pinnedSessionId: null,
      lastSessionByTaskId: { "task-1": "session-auto" },
    });
  });
});
