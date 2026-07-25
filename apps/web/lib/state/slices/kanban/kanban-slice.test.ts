import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createKanbanSlice } from "./kanban-slice";
import type { KanbanSlice } from "./types";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-a";
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

  it("clears a pin when auto-selecting a session for a different task", () => {
    const store = makeStore();

    store.getState().setActiveSession(TASK_ID, PINNED_SESSION_ID);
    store.getState().setActiveSessionAuto("task-2", AUTO_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: "task-2",
      activeSessionId: AUTO_SESSION_ID,
      pinnedSessionId: null,
      lastSessionByTaskId: { [TASK_ID]: PINNED_SESSION_ID, "task-2": AUTO_SESSION_ID },
    });
  });
});

describe("kanban slice workspace transition", () => {
  it("clears workflow, board, and active task context before loading another workspace", () => {
    const store = makeStore();
    store.setState({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [{ id: "step-a", title: "Todo", color: "blue", position: 0 }],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: "step-a",
            title: "Workspace A task",
            position: 0,
          },
        ],
      },
      kanbanMulti: {
        snapshots: {
          [WORKFLOW_ID]: {
            workflowId: WORKFLOW_ID,
            workflowName: "Workflow A",
            steps: [],
            tasks: [],
          },
        },
        isLoading: true,
      },
      workflows: {
        items: [{ id: WORKFLOW_ID, workspaceId: "workspace-a", name: "Workflow A" }],
        activeId: WORKFLOW_ID,
      },
      workspaceContextGeneration: 7,
      tasks: {
        activeTaskId: TASK_ID,
        activeSessionId: PINNED_SESSION_ID,
        pinnedSessionId: PINNED_SESSION_ID,
        lastSessionByTaskId: { [TASK_ID]: PINNED_SESSION_ID },
      },
    });

    store.getState().resetKanbanWorkspaceContext();

    expect(store.getState()).toMatchObject({
      kanban: { workflowId: null, steps: [], tasks: [] },
      kanbanMulti: { snapshots: {}, isLoading: false },
      workflows: { items: [], activeId: null },
      workspaceContextGeneration: 8,
      tasks: {
        activeTaskId: null,
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
  });
});
