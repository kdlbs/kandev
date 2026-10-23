import { act, cleanup, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import {
  setup,
  current,
  update,
  WORKFLOW_ID,
  ORIGINAL_SESSION,
  STALE_STEP,
  STALE_UPDATED_AT,
  original,
  sibling,
  wireTask,
} from "./use-cross-step-move.test-helpers";
const NEWER_STEP = "newer-step";
const OLDER_FIRST = "older-first";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// @covers AC-TASKS-KANBAN-TASK-REORDERING-001.13, AC-TASKS-KANBAN-TASK-REORDERING-001.28
describe("cross-step optimistic rollback ownership", () => {
  it.each(["authoritative", "optimistic"])(
    "preserves newer accepted step %s and server position",
    async (step) => {
      const h = setup();
      const pending = h.start();
      expect(current(h.store)).toMatchObject({ workflowStepId: "optimistic", position: 7 });
      expect(h.requests).toEqual([{ workflow_id: WORKFLOW_ID, workflow_step_id: "optimistic" }]);
      update(h.store, { workflow_step_id: step, title: "Remote title", position: 42 });
      expect(current(h.store)).toMatchObject({ workflowStepId: step, title: "Remote title" });
      await h.settle(pending);
      expect(current(h.store)).toMatchObject({
        workflowStepId: step,
        title: "Remote title",
        position: 42,
      });
      expect(h.onMoveError).toHaveBeenCalledWith({
        message: "The task could not be moved. Try again.",
        taskId: "task-a",
        sessionId: ORIGINAL_SESSION,
      });
      expect(h.store.getState().taskOverview.reads).toEqual({});
    },
  );

  it("preserves an identical accepted target without relying on timestamp advancement", async () => {
    const h = setup();
    const pending = h.start();
    update(h.store, { workflow_step_id: "optimistic" });
    await h.settle(pending);
    expect(current(h.store)?.workflowStepId).toBe("optimistic");
  });

  it("preserves an accepted away/back step sequence", async () => {
    const h = setup();
    const pending = h.start();
    update(h.store, { workflow_step_id: "elsewhere" });
    update(h.store, { workflow_step_id: "optimistic" });
    await h.settle(pending);
    expect(current(h.store)?.workflowStepId).toBe("optimistic");
  });

  it("rolls back its stage while preserving newer fields from a partial task projection", async () => {
    const h = setup();
    const pending = h.start();
    act(() =>
      registerTasksHandlers(h.store)["task.updated"]!({
        id: "partial",
        type: "notification",
        action: "task.updated",
        payload: {
          task_id: "task-a",
          workflow_id: WORKFLOW_ID,
          title: "Partial title",
          position: 13,
        },
      }),
    );
    await h.settle(pending);
    expect(current(h.store)).toMatchObject({
      workflowStepId: "source",
      title: "Partial title",
      position: 13,
    });
  });

  it("rolls back an ordinary owned failure while preserving another task's event", async () => {
    const h = setup();
    const pending = h.start();
    update(h.store, {
      task_id: "task-b",
      workflow_step_id: "elsewhere",
      title: "Sibling update",
      position: 19,
    });
    await h.settle(pending);
    expect(current(h.store)).toMatchObject({ workflowStepId: "source", position: 7 });
    expect(current(h.store, "task-b")).toMatchObject({
      workflowStepId: "elsewhere",
      title: "Sibling update",
      position: 19,
    });
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });

  it("keeps independent moves and newer same-task moves isolated", async () => {
    const h = setup();
    const old = h.start();
    const other = h.start(sibling, "other");
    const newer = h.start(current(h.store)!, "newer");
    await h.settle(newer, 2, true);
    await h.settle(old);
    await h.settle(other, 1);
    expect(current(h.store)?.workflowStepId).toBe("newer");
    expect(current(h.store, "task-b")?.workflowStepId).toBe("source");
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });
});

describe("cross-step ownership lifecycle", () => {
  it.each(["delete", "membership", "workflow", "workspace", "journal-invalidated"])(
    "does not restore a departed or invalidated owner: %s",
    async (scenario) => {
      const h = setup();
      const pending = h.start();
      act(() => {
        if (scenario === "delete")
          registerTasksHandlers(h.store)["task.deleted"]!({
            id: "deleted",
            type: "notification",
            action: "task.deleted",
            payload: { task_id: "task-a" },
          });
        if (scenario === "membership") h.store.getState().removeMultiTask(WORKFLOW_ID, "task-a");
        if (scenario === "workflow")
          update(h.store, {
            old_workflow_id: WORKFLOW_ID,
            workflow_id: "workflow-b",
            workflow_step_id: "new-workflow-step",
          });
        if (scenario === "workspace") h.store.getState().resetKanbanWorkspaceContext();
        if (scenario === "journal-invalidated")
          h.store.setState((s) => ({ taskOverview: { ...s.taskOverview, reads: {} } }));
      });
      const before = current(h.store);
      await h.settle(pending);
      expect(current(h.store)).toEqual(before);
      expect(h.onMoveError).toHaveBeenCalledTimes(1);
      expect(h.store.getState().taskOverview.reads).toEqual({});
    },
  );

  it("uses current snapshot stage for rollback while preserving captured error identity", async () => {
    const h = setup();
    update(h.store, { workflow_step_id: "current-source" });
    const pending = h.start();
    await h.settle(pending);
    expect(current(h.store)?.workflowStepId).toBe("current-source");
    expect(h.onMoveError).toHaveBeenCalledWith({
      message: "The task could not be moved. Try again.",
      taskId: "task-a",
      sessionId: ORIGINAL_SESSION,
    });
  });

  it("success keeps the optimistic stage and closes ownership tracking", async () => {
    const h = setup();
    const pending = h.start();
    await h.settle(pending, 0, true);
    expect(current(h.store)).toMatchObject({ workflowStepId: "optimistic", position: 7 });
    expect(h.onMoveError).not.toHaveBeenCalled();
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });

  it("does not submit for absent tasks, absent snapshots or current-stage targets", async () => {
    const h = setup();
    const pending: Promise<void>[] = [];
    act(() => {
      pending.push(h.result.current.move("missing", "target", original));
      pending.push(h.result.current.move("task-a", "source", original));
      h.store.getState().clearKanbanMulti();
      pending.push(h.result.current.move("task-a", "target", original));
    });
    try {
      expect(h.requests).toEqual([]);
    } finally {
      await act(async () => {
        for (const respond of h.responses) respond(new Response("{}", { status: 500 }));
        await Promise.all(pending);
      });
    }
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });
});

describe("cross-step pending request lifetime", () => {
  it("does not reclaim membership after removal and reinsertion", async () => {
    const h = setup();
    const pending = h.start();
    const optimistic = current(h.store)!;
    act(() => h.store.getState().removeMultiTask(WORKFLOW_ID, "task-a"));
    act(() => h.store.getState().updateMultiTask(WORKFLOW_ID, optimistic));
    await h.settle(pending);
    expect(current(h.store)?.workflowStepId).toBe("optimistic");
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });

  it("retains the original request and callback across a hook rerender", async () => {
    const h = setup();
    const pending = h.start();
    const replacementError = vi.fn();
    h.rerender({ workflowId: "workflow-b", onError: replacementError });
    await h.settle(pending);
    expect(h.requests).toEqual([{ workflow_id: WORKFLOW_ID, workflow_step_id: "optimistic" }]);
    expect(h.onMoveError).toHaveBeenCalledWith({
      message: "The task could not be moved. Try again.",
      taskId: "task-a",
      sessionId: ORIGINAL_SESSION,
    });
    expect(replacementError).not.toHaveBeenCalled();
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });

  it.each([true, false])(
    "settles and releases ownership after initiator unmount: success=%s",
    async (success) => {
      const h = setup();
      const subscribe = h.store.subscribe;
      let activeSubscriptions = 0;
      const observer = vi.spyOn(h.store, "subscribe").mockImplementation((listener) => {
        activeSubscriptions += 1;
        const unsubscribe = subscribe(listener);
        return () => {
          activeSubscriptions -= 1;
          unsubscribe();
        };
      });
      const pending = h.start();
      expect(activeSubscriptions).toBe(1);
      h.unmount();
      await h.settle(pending, 0, success);
      expect(activeSubscriptions).toBe(0);
      observer.mockRestore();
      expect(current(h.store)?.workflowStepId).toBe(success ? "optimistic" : "source");
      expect(h.store.getState().taskOverview.reads).toEqual({});
    },
  );
});

it("fails closed when the real bounded journal overflows", async () => {
  const h = setup();
  const pending = h.start();
  const readId = Object.keys(h.store.getState().taskOverview.reads)[0];
  act(() =>
    h.store.setState((state) => ({
      taskOverview: {
        ...state.taskOverview,
        reads: {
          ...state.taskOverview.reads,
          [readId]: { ...state.taskOverview.reads[readId], bytes: 1024 * 1024 },
        },
      },
    })),
  );
  update(h.store, { task_id: "task-b", title: "Overflow event" });
  expect(h.store.getState().taskOverview.reads[readId]).toBeUndefined();
  await h.settle(pending);
  expect(current(h.store)?.workflowStepId).toBe("optimistic");
  expect(h.store.getState().taskOverview.reads).toEqual({});
});

describe("accepted stage evidence", () => {
  it("rolls back after an older task event is discarded", async () => {
    const h = setup();
    const pending = h.start();
    update(h.store, {
      workflow_step_id: STALE_STEP,
      title: "Stale title",
      updated_at: STALE_UPDATED_AT,
    });
    expect(current(h.store)).toMatchObject({ workflowStepId: "optimistic", title: "Original" });
    await h.settle(pending);
    expect(current(h.store)).toMatchObject({ workflowStepId: "source", title: "Original" });
    expect(h.store.getState().taskOverview.reads).toEqual({});
  });

  it.each([false, true])(
    "preserves an accepted equal-stage snapshot (identical=%s)",
    async (identical) => {
      const h = setup();
      const pending = h.start();
      const live = h.store.getState().kanbanMulti.snapshots[WORKFLOW_ID];
      act(() =>
        h.store.getState().setWorkflowSnapshot(WORKFLOW_ID, {
          ...live,
          tasks: live.tasks.map((task) =>
            task.id === "task-a"
              ? {
                  ...task,
                  ...(identical ? {} : { title: "Snapshot title", position: 23 }),
                }
              : task,
          ),
        }),
      );
      await h.settle(pending);
      expect(current(h.store)).toMatchObject({
        workflowStepId: "optimistic",
        title: identical ? "Original" : "Snapshot title",
        position: identical ? 7 : 23,
      });
    },
  );

  it("does not treat a discarded stale snapshot as a stage write", async () => {
    const h = setup();
    const pending = h.start();
    const live = h.store.getState().kanbanMulti.snapshots[WORKFLOW_ID];
    act(() =>
      h.store.getState().setWorkflowSnapshot(WORKFLOW_ID, {
        ...live,
        tasks: live.tasks.map((task) =>
          task.id === "task-a"
            ? {
                ...task,
                workflowStepId: STALE_STEP,
                updatedAt: STALE_UPDATED_AT,
              }
            : task,
        ),
      }),
    );
    expect(current(h.store)?.workflowStepId).toBe("optimistic");
    await h.settle(pending);
    expect(current(h.store)?.workflowStepId).toBe("source");
  });
});

it.each(["event", "snapshot"])(
  "keeps independently fresh status summary from a stale %s without claiming the stage",
  async (source) => {
    const h = setup();
    const pending = h.start();
    const summary = { revision: 2, updated_at: "2026-10-22T11:00:00Z", active_subagent_count: 2 };
    if (source === "event")
      update(h.store, {
        workflow_step_id: STALE_STEP,
        updated_at: STALE_UPDATED_AT,
        status_summary: summary,
      });
    else {
      const live = h.store.getState().kanbanMulti.snapshots[WORKFLOW_ID];
      act(() =>
        h.store.getState().setWorkflowSnapshot(WORKFLOW_ID, {
          ...live,
          tasks: live.tasks.map((task) =>
            task.id === "task-a"
              ? {
                  ...task,
                  workflowStepId: STALE_STEP,
                  updatedAt: STALE_UPDATED_AT,
                  statusSummary: summary,
                }
              : task,
          ),
        }),
      );
    }
    expect(current(h.store)).toMatchObject({
      workflowStepId: "optimistic",
      statusSummary: summary,
    });
    await h.settle(pending);
    expect(current(h.store)).toMatchObject({ workflowStepId: "source", statusSummary: summary });
    expect(h.store.getState().taskOverview.reads).toEqual({});
  },
);

it("does not claim an unchanged task when a sibling snapshot row is replaced", async () => {
  const h = setup();
  const pending = h.start();
  const live = h.store.getState().kanbanMulti.snapshots[WORKFLOW_ID];
  act(() =>
    h.store.getState().setWorkflowSnapshot(WORKFLOW_ID, {
      ...live,
      tasks: live.tasks.map((task) =>
        task.id === "task-b" ? { ...task, title: "Snapshot sibling", position: 24 } : task,
      ),
    }),
  );
  await h.settle(pending);
  expect(current(h.store)).toMatchObject({ workflowStepId: "source", position: 7 });
  expect(current(h.store, "task-b")).toMatchObject({ title: "Snapshot sibling", position: 24 });
  expect(h.store.getState().taskOverview.reads).toEqual({});
});

it.each([OLDER_FIRST, "newer-first"])(
  "returns to the source after both admitted moves fail: %s",
  async (order) => {
    const h = setup();
    const older = h.start();
    const newer = h.start(current(h.store)!, NEWER_STEP);
    expect(current(h.store)?.workflowStepId).toBe(NEWER_STEP);
    if (order === OLDER_FIRST) {
      await h.settle(older, 0);
      await h.settle(newer, 1);
    } else {
      await h.settle(newer, 1);
      await h.settle(older, 0);
    }
    expect(h.onMoveError).toHaveBeenCalledTimes(2);
    expect(current(h.store)).toMatchObject({ workflowStepId: "source", position: 7 });
    expect(h.store.getState().taskOverview.reads).toEqual({});
  },
);

it.each([OLDER_FIRST, "newer-first"])(
  "preserves accepted later stage across both failed moves: %s",
  async (order) => {
    const h = setup();
    const older = h.start();
    const newer = h.start(current(h.store)!, NEWER_STEP);
    update(h.store, { workflow_step_id: NEWER_STEP, title: "Accepted stage", position: 35 });
    if (order === OLDER_FIRST) {
      await h.settle(older, 0);
      await h.settle(newer, 1);
    } else {
      await h.settle(newer, 1);
      await h.settle(older, 0);
    }
    expect(current(h.store)).toMatchObject({
      workflowStepId: NEWER_STEP,
      title: "Accepted stage",
      position: 35,
    });
    expect(h.onMoveError).toHaveBeenCalledTimes(2);
    expect(h.store.getState().taskOverview.reads).toEqual({});
  },
);

it("does not chain through an accepted identical stage before the next move", async () => {
  const h = setup();
  const older = h.start();
  update(h.store, { workflow_step_id: "optimistic", title: "Accepted baseline", position: 36 });
  const newer = h.start(current(h.store)!, NEWER_STEP);
  await h.settle(newer, 1);
  await h.settle(older, 0);
  expect(current(h.store)).toMatchObject({
    workflowStepId: "optimistic",
    title: "Accepted baseline",
    position: 36,
  });
  expect(h.store.getState().taskOverview.reads).toEqual({});
});

it.each([
  [2, 1, 0],
  [0, 1, 2],
  [1, 2, 0],
])("removes failed intermediate stages across three admitted moves: %j", async (...order) => {
  const h = setup();
  const pending = [h.start()];
  pending.push(h.start(current(h.store)!, "second-step"));
  pending.push(h.start(current(h.store)!, "third-step"));
  for (const index of order) await h.settle(pending[index], index);
  expect(current(h.store)?.workflowStepId).toBe("source");
  expect(h.onMoveError).toHaveBeenCalledTimes(3);
  expect(h.store.getState().taskOverview.reads).toEqual({});
});

it("never re-arms ownership after bounded overflow invalidates the chain", async () => {
  const h = setup();
  const older = h.start();
  const oldRead = Object.keys(h.store.getState().taskOverview.reads)[0];
  const newer = h.start(current(h.store)!, NEWER_STEP);
  act(() =>
    h.store.setState((state) => ({
      taskOverview: {
        ...state.taskOverview,
        reads: {
          ...state.taskOverview.reads,
          [oldRead]: { ...state.taskOverview.reads[oldRead], bytes: 1024 * 1024 },
        },
      },
    })),
  );
  update(h.store, { task_id: "task-b", title: "Overflow sibling" });
  expect(h.store.getState().taskOverview.reads).toEqual({});
  await h.settle(newer, 1);
  await h.settle(older, 0);
  expect(current(h.store)?.workflowStepId).toBe(NEWER_STEP);
  expect(h.store.getState().taskOverview.reads).toEqual({});
});

it("retiring an old request cannot remove another pending chain after a successful move", async () => {
  const h = setup();
  const older = h.start();
  const accepted = h.start(current(h.store)!, "accepted-step");
  await h.settle(accepted, 1, true);
  const third = h.start(current(h.store)!, "third-step");
  await h.settle(older, 0);
  const fourth = h.start(current(h.store)!, "fourth-step");
  await h.settle(fourth, 3);
  await h.settle(third, 2);
  expect(current(h.store)?.workflowStepId).toBe("accepted-step");
  expect(h.store.getState().taskOverview.reads).toEqual({});
});

it.each(["after-optimism", "before-optimism", "stale-response"])(
  "admits only current HTTP snapshot stage evidence: %s",
  async (schedule) => {
    const h = setup(true);
    let pending!: Promise<void>;
    const enable = () => {
      const live = h.store.getState().kanbanMulti.snapshots[WORKFLOW_ID];
      act(() =>
        h.store.getState().setWorkflowSnapshot(WORKFLOW_ID, { ...live, isPlaceholder: true }),
      );
      h.rerender({ workflowId: WORKFLOW_ID, onError: h.onMoveError, snapshotEnabled: true });
    };
    if (schedule === "before-optimism") {
      enable();
      await waitFor(() => expect(h.snapshotResponses).toHaveLength(1));
      pending = h.start();
    } else {
      pending = h.start();
      enable();
    }
    try {
      await waitFor(() => expect(h.snapshotResponses).toHaveLength(1));
      await act(async () =>
        h.snapshotResponses[0](
          new Response(
            JSON.stringify({
              workflow: { id: WORKFLOW_ID, name: "Workflow" },
              steps: ["source", "optimistic"].map((id, position) => ({
                id,
                name: id,
                color: null,
                position,
              })),
              tasks: [
                {
                  ...wireTask,
                  workflow_step_id: "optimistic",
                  ...(schedule === "stale-response"
                    ? {
                        updated_at: STALE_UPDATED_AT,
                        status_summary: { revision: 3, updated_at: "2026-10-22T11:00:00Z" },
                      }
                    : {}),
                },
                { ...wireTask, task_id: "task-b" },
              ],
            }),
            { status: 200 },
          ),
        ),
      );
      await waitFor(() =>
        expect(h.store.getState().kanbanMulti.snapshots[WORKFLOW_ID].steps).toHaveLength(2),
      );
      await h.settle(pending);
      expect(current(h.store)?.workflowStepId).toBe(
        schedule === "after-optimism" ? "optimistic" : "source",
      );
      if (schedule === "stale-response") expect(current(h.store)?.statusSummary?.revision).toBe(3);
      expect(h.store.getState().taskOverview.reads).toEqual({});
    } finally {
      await h.settle(pending);
    }
  },
);

it("cannot resurrect a predecessor journal that was already closed", async () => {
  const h = setup();
  const older = h.start();
  const oldRead = Object.keys(h.store.getState().taskOverview.reads)[0];
  const newer = h.start(current(h.store)!, NEWER_STEP);
  act(() => h.store.getState().finishTaskOverviewRead(oldRead));
  await h.settle(newer, 1);
  await h.settle(older, 0);
  expect(current(h.store)?.workflowStepId).toBe("optimistic");
  expect(h.store.getState().taskOverview.reads).toEqual({});
});
