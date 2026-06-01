import { describe, it, expect } from "vitest";
import { buildTaskMentionItems, type TaskMentionSource } from "./task-mention-items";

function makeSource(overrides: Partial<TaskMentionSource> = {}): TaskMentionSource {
  return {
    snapshots: {},
    workflows: [],
    ...overrides,
  };
}

describe("buildTaskMentionItems / basics", () => {
  it("returns tasks from the snapshots with workflow/step names resolved", () => {
    const source = makeSource({
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "Main flow",
          steps: [{ id: "step-1", title: "Todo", color: "", position: 0 }],
          tasks: [
            {
              id: "task-a",
              workflowStepId: "step-1",
              title: "Implement auth",
              position: 0,
              state: "IN_PROGRESS",
            },
          ],
        },
      },
      workflows: [{ id: "wf-1", workspaceId: "ws-1", name: "Main flow" }],
    });

    const items = buildTaskMentionItems(source, null);
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
        state: "IN_PROGRESS",
      },
    });
  });

  it("excludes the current task by id", () => {
    const source = makeSource({
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "Main",
          steps: [],
          tasks: [
            { id: "task-a", workflowStepId: "step-1", title: "A", position: 0 },
            { id: "task-b", workflowStepId: "step-1", title: "B", position: 1 },
          ],
        },
      },
    });

    const items = buildTaskMentionItems(source, "task-a");
    expect(items.map((i) => i.task?.taskId)).toEqual(["task-b"]);
  });
});

describe("buildTaskMentionItems / merging and filtering", () => {
  it("merges tasks across snapshots and dedupes by id", () => {
    const source = makeSource({
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "Main",
          steps: [],
          tasks: [
            { id: "task-a", workflowStepId: "step-1", title: "A", position: 0 },
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
    });

    const ids = buildTaskMentionItems(source, null).map((i) => i.task?.taskId);
    expect(ids).toEqual(["task-a", "task-c", "task-d"]);
  });

  it("resolves workflow name from the workflows list when the snapshot omits it", () => {
    const source = makeSource({
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "",
          steps: [{ id: "step-1", title: "Todo", color: "", position: 0 }],
          tasks: [{ id: "task-a", workflowStepId: "step-1", title: "A", position: 0 }],
        },
      },
      workflows: [{ id: "wf-1", workspaceId: "ws-1", name: "From list" }],
    });

    const [item] = buildTaskMentionItems(source, null);
    expect(item.description).toBe("From list · Todo");
  });

  it("falls back to placeholder names when workflow/step are missing", () => {
    const source = makeSource({
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "",
          steps: [],
          tasks: [{ id: "task-a", workflowStepId: "step-missing", title: "A", position: 0 }],
        },
      },
    });

    const [item] = buildTaskMentionItems(source, null);
    expect(item.description).toBe("Workflow · Step");
  });
});
