import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";
import { makeStore, makeTask, makeMessage } from "./tasks.test-helpers";

describe("task.updated runner mutability projection", () => {
  // AC-TASKS-RUNNER-SWITCH-001.9: unlike executor identity, an omitted
  // runner_editable/runner_ineligible_reason must NOT fall back to the cached
  // task's last-known reading — a permission-shaped flag going stale-open is
  // worse than going stale-closed.
  it("does not gap-fill a cached editable=true when a lightweight update omits the projection", () => {
    const existingTask = {
      id: "t1",
      workflowStepId: "step1",
      title: "Old title",
      position: 0,
      runnerEditable: true,
      runnerIneligibleReason: "eligible",
    };
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [existingTask],
      } as unknown as AppState["kanban"],
    });

    registerTasksHandlers(store)["task.updated"]!(makeMessage(makeTask("t1", null)));

    const task = store.getState().kanban.tasks.find((item) => item.id === "t1");
    expect(task?.runnerEditable).toBe(false);
    expect(task?.runnerIneligibleReason).toBe("evaluation_unavailable");
  });

  it("carries an explicit projection through to the store", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [] } as unknown as AppState["kanban"],
    });

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask("t1", null),
        runner_editable: false,
        runner_ineligible_reason: "session_exists",
      }),
    );

    const task = store.getState().kanban.tasks.find((item) => item.id === "t1");
    expect(task?.runnerEditable).toBe(false);
    expect(task?.runnerIneligibleReason).toBe("session_exists");
  });
});
