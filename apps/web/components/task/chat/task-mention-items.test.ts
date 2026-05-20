import { describe, it, expect } from "vitest";
import { buildTaskMentionItems } from "./task-mention-items";
import type { AppState } from "@/lib/state/store";

function makeState(overrides: Partial<AppState> = {}): AppState {
  const base = {
    kanban: { workflowId: "wf-1", steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    workflows: { items: [], activeId: null },
    tasks: { activeTaskId: null, activeSessionId: null, pinnedSessionId: null },
  } as unknown as AppState;
  return { ...base, ...overrides } as AppState;
}

describe("buildTaskMentionItems / basics", () => {
  it("returns tasks from the current workflow with workflow/step names resolved", () => {
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

    const items = buildTaskMentionItems(state, null);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "task",
      label: "Implement auth",
      description: "Main flow · Todo",
      task: {
        taskId: "task-a",
        title: "Implement auth",
        workflowId: "wf-1",
        workflowStepId: "step-1",
        state: "in_progress",
      },
    });
  });

  it("excludes the current task by id", () => {
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

    const items = buildTaskMentionItems(state, "task-a");
    expect(items.map((i) => i.task?.taskId)).toEqual(["task-b"]);
  });
});

describe("buildTaskMentionItems / merging and filtering", () => {
  it("merges tasks from kanbanMulti snapshots and dedupes by id", () => {
    const state = makeState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [{ id: "task-a", workflowStepId: "step-1", title: "A", position: 0 }],
      },
      kanbanMulti: {
        snapshots: {
          "wf-1": {
            workflowId: "wf-1",
            workflowName: "Main",
            steps: [],
            tasks: [
              { id: "task-a", workflowStepId: "step-1", title: "A (dup)", position: 0 },
              { id: "task-c", workflowStepId: "step-2", title: "C", position: 0 },
            ],
          },
          "wf-2": {
            workflowId: "wf-2",
            workflowName: "Other",
            steps: [{ id: "step-9", title: "Review", color: "", position: 0 }],
            tasks: [{ id: "task-d", workflowStepId: "step-9", title: "D", position: 0 }],
          },
        },
        isLoading: false,
      },
    } as unknown as Partial<AppState>);

    const ids = buildTaskMentionItems(state, null).map((i) => i.task?.taskId);
    expect(ids).toEqual(["task-a", "task-c", "task-d"]);
  });

  it("skips stale kanban tasks whose step is not in the current workflow's steps", () => {
    const state = makeState({
      kanban: {
        workflowId: "wf-1",
        steps: [{ id: "step-current", title: "Todo", color: "", position: 0 }],
        tasks: [
          { id: "task-fresh", workflowStepId: "step-current", title: "Fresh", position: 0 },
          // Left over from a previous workflow: its step is not in wf-1's steps,
          // so tagging it with wf-1 would be wrong.
          { id: "task-stale", workflowStepId: "step-other", title: "Stale", position: 1 },
        ],
      },
    } as unknown as Partial<AppState>);

    const ids = buildTaskMentionItems(state, null).map((i) => i.task?.taskId);
    expect(ids).toEqual(["task-fresh"]);
  });

  it("falls back to placeholder names when workflow/step are missing", () => {
    const state = makeState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [{ id: "task-a", workflowStepId: "step-missing", title: "A", position: 0 }],
      },
    } as unknown as Partial<AppState>);

    const [item] = buildTaskMentionItems(state, null);
    expect(item.description).toBe("Workflow · Step");
  });
});
