import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createKanbanSlice } from "./kanban-slice";
import type { KanbanSlice } from "./types";

function makeStore() {
  return create<KanbanSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createKanbanSlice as any)(...a) })),
  );
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
