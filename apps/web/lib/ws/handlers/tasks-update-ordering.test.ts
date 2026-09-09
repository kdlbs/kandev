import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";
import { makeStore, makeTask, makeMessage } from "./tasks.test-helpers";

const WORKFLOW_ID = "wf1";
const TASK_ID = "t1";
const OLDER_TIMESTAMP = "2026-01-01T00:00:10.000Z";
const NEWER_TIMESTAMP = "2026-01-01T00:00:20.000Z";

// AC-TASKS-RUNNER-SWITCH-002.19: ordered delivery is not guaranteed, so the
// client must compare the incoming event's updated_at against the cached
// value and ignore anything older rather than trust arrival order.
describe("task.updated stale-event ordering guard", () => {
  it("ignores an event whose updated_at is older than the cached task", () => {
    const existingTask = {
      id: TASK_ID,
      workflowStepId: "step1",
      title: "Current title",
      position: 0,
      updatedAt: OLDER_TIMESTAMP,
    };
    const store = makeStore({
      kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [existingTask] } as unknown as AppState["kanban"],
    });

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask(TASK_ID, null),
        title: "Stale title from a late event",
        updated_at: "2026-01-01T00:00:05.000Z",
      }),
    );

    const task = store.getState().kanban.tasks.find((item) => item.id === TASK_ID);
    expect(task?.title).toBe("Current title");
    expect(task?.updatedAt).toBe(OLDER_TIMESTAMP);
  });

  it("applies an event whose updated_at is newer than the cached task", () => {
    const existingTask = {
      id: TASK_ID,
      workflowStepId: "step1",
      title: "Current title",
      position: 0,
      updatedAt: OLDER_TIMESTAMP,
    };
    const store = makeStore({
      kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [existingTask] } as unknown as AppState["kanban"],
    });

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask(TASK_ID, null),
        title: "Fresh title",
        updated_at: NEWER_TIMESTAMP,
      }),
    );

    const task = store.getState().kanban.tasks.find((item) => item.id === TASK_ID);
    expect(task?.title).toBe("Fresh title");
    expect(task?.updatedAt).toBe(NEWER_TIMESTAMP);
  });

  it("converges on the transaction that committed second regardless of delivery order", () => {
    const existingTask = {
      id: TASK_ID,
      workflowStepId: "step1",
      title: "Original title",
      position: 0,
    };
    const store = makeStore({
      kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [existingTask] } as unknown as AppState["kanban"],
    });
    const handlers = registerTasksHandlers(store);

    // The second switch (committed later) is delivered first...
    handlers["task.updated"]!(
      makeMessage({
        ...makeTask(TASK_ID, null),
        title: "Second switch",
        updated_at: NEWER_TIMESTAMP,
      }),
    );
    // ...then the first switch (committed earlier) arrives late.
    handlers["task.updated"]!(
      makeMessage({
        ...makeTask(TASK_ID, null),
        title: "First switch",
        updated_at: OLDER_TIMESTAMP,
      }),
    );

    const task = store.getState().kanban.tasks.find((item) => item.id === TASK_ID);
    expect(task?.title).toBe("Second switch");
  });
});
