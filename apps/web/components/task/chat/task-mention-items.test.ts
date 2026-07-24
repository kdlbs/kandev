import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/store";
import { buildTaskMentionItems } from "./task-mention-items";

function makeState(overrides: Partial<AppState> = {}): AppState {
  const base = {
    kanban: { workflowId: "wf-1", steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    workflows: { items: [], activeId: null },
    tasks: { activeTaskId: null, activeSessionId: null, pinnedSessionId: null },
  } as unknown as AppState;
  return { ...base, ...overrides } as AppState;
}

describe("buildTaskMentionItems", () => {
  it("returns sibling tasks with resolved workflow and step names", () => {
    const state = makeState({
      kanban: {
        workflowId: "wf-1",
        steps: [{ id: "step-1", title: "Todo", color: "", position: 0 }],
        tasks: [
          {
            id: "task-a",
            workflowStepId: "step-1",
            title: "Implement auth",
            position: 0,
            state: "in_progress",
          },
        ],
      },
      workflows: {
        items: [{ id: "wf-1", workspaceId: "ws-1", name: "Main flow" }],
        activeId: "wf-1",
      },
    } as unknown as Partial<AppState>);

    expect(buildTaskMentionItems(state, null)).toEqual([
      expect.objectContaining({
        kind: "task",
        label: "Implement auth",
        description: "Main flow · Todo",
        task: expect.objectContaining({ taskId: "task-a", workflowId: "wf-1" }),
      }),
    ]);
  });

  it("excludes the current task", () => {
    const state = makeState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          { id: "task-a", workflowStepId: "step-1", title: "A", position: 0 },
          { id: "task-b", workflowStepId: "step-1", title: "B", position: 1 },
        ],
      },
    } as unknown as Partial<AppState>);

    expect(buildTaskMentionItems(state, "task-a").map((item) => item.task?.taskId)).toEqual([
      "task-b",
    ]);
  });

  it("merges workflow snapshots and skips stale active-workflow tasks", () => {
    const state = makeState({
      kanban: {
        workflowId: "wf-1",
        steps: [{ id: "step-current", title: "Todo", color: "", position: 0 }],
        tasks: [
          { id: "task-a", workflowStepId: "step-current", title: "A", position: 0 },
          { id: "task-stale", workflowStepId: "step-other", title: "Stale", position: 1 },
        ],
      },
      kanbanMulti: {
        snapshots: {
          "wf-1": {
            workflowId: "wf-1",
            workflowName: "Main",
            steps: [],
            tasks: [{ id: "task-a", workflowStepId: "step-current", title: "A", position: 0 }],
          },
          "wf-2": {
            workflowId: "wf-2",
            workflowName: "Other",
            steps: [{ id: "step-2", title: "Review", color: "", position: 0 }],
            tasks: [{ id: "task-b", workflowStepId: "step-2", title: "B", position: 0 }],
          },
        },
        isLoading: false,
      },
    } as unknown as Partial<AppState>);

    expect(buildTaskMentionItems(state, null).map((item) => item.task?.taskId)).toEqual([
      "task-a",
      "task-b",
    ]);
  });
});
