import { describe, expect, it } from "vitest";
import {
  advanceTaskNavigationRevision,
  beginTaskRemoval,
  createTaskRemovalState,
  isTaskRemovalPending,
  ownsTaskRemovalDeparture,
  pendingTaskRemovalAction,
  pendingTaskRemovalTaskIds,
  recordTaskRemovalResult,
  releaseTaskRemoval,
  taskRemovalCoversTask,
  taskRemovalOwnsDepartureForTask,
} from "./task-removal";

const REMOVAL_ONE = "removal-1";
const REMOVAL_TWO = "removal-2";
const WORKSPACE_ONE = "workspace-1";
const WORKSPACE_TWO = "workspace-2";
const TASK_ONE = "task-1";
const TASK_TWO = "task-2";
const PARENT_TASK = "task-parent";
const CHILD_TASK = "task-child";

describe("task removal state", () => {
  it("captures the complete removal set and rejects overlapping pending work", () => {
    const initial = createTaskRemovalState();
    const started = beginTaskRemoval(initial, {
      token: REMOVAL_ONE,
      action: "archive",
      workspaceId: WORKSPACE_ONE,
      taskIds: [PARENT_TASK, CHILD_TASK],
      requestIds: [PARENT_TASK],
      departure: {
        taskId: CHILD_TASK,
        sessionId: "session-child",
        navigationRevision: 0,
        origin: "detail",
      },
    });

    expect(started).not.toBeNull();
    expect(taskRemovalCoversTask(started!, CHILD_TASK)).toBe(true);
    expect(started!.operationsByToken[REMOVAL_ONE].taskIds).toEqual(["task-parent", "task-child"]);
    expect(
      beginTaskRemoval(started!, {
        token: "removal-2",
        action: "delete",
        workspaceId: WORKSPACE_ONE,
        taskIds: [CHILD_TASK],
        requestIds: [CHILD_TASK],
        departure: null,
      }),
    ).toBeNull();
  });

  it("invalidates automatic navigation after leaving and returning to a task", () => {
    let state = beginTaskRemoval(createTaskRemovalState(), {
      token: REMOVAL_ONE,
      action: "delete",
      workspaceId: WORKSPACE_ONE,
      taskIds: [TASK_ONE],
      requestIds: [TASK_ONE],
      departure: { taskId: TASK_ONE, sessionId: null, navigationRevision: 0, origin: "detail" },
    })!;

    expect(ownsTaskRemovalDeparture(state, REMOVAL_ONE, 0)).toBe(true);
    state = advanceTaskNavigationRevision(state);
    state = advanceTaskNavigationRevision(state);
    expect(state.navigationRevision).toBe(2);
    expect(ownsTaskRemovalDeparture(state, REMOVAL_ONE, 0)).toBe(false);
    expect(ownsTaskRemovalDeparture(state, REMOVAL_ONE, 2)).toBe(false);
  });

  it("keeps per-request outcomes until the coordinator releases the operation", () => {
    let state = beginTaskRemoval(createTaskRemovalState(), {
      token: REMOVAL_ONE,
      action: "delete",
      workspaceId: WORKSPACE_ONE,
      taskIds: [TASK_ONE, TASK_TWO],
      requestIds: [TASK_ONE, TASK_TWO],
      departure: null,
    })!;

    state = recordTaskRemovalResult(state, REMOVAL_ONE, [TASK_ONE], "succeeded");
    expect(state.operationsByToken[REMOVAL_ONE].outcomesByTaskId).toEqual({
      [TASK_ONE]: "succeeded",
      [TASK_TWO]: "pending",
    });
    expect(taskRemovalCoversTask(state, TASK_TWO)).toBe(true);

    state = recordTaskRemovalResult(state, REMOVAL_ONE, [TASK_TWO], "failed");
    expect(taskRemovalCoversTask(state, TASK_TWO)).toBe(true);
    state = releaseTaskRemoval(state, REMOVAL_ONE);
    expect(taskRemovalCoversTask(state, TASK_ONE)).toBe(false);
    expect(taskRemovalCoversTask(state, TASK_TWO)).toBe(false);
  });

  it("recognizes local departure ownership for every task in a cascade", () => {
    const state = beginTaskRemoval(createTaskRemovalState(), {
      token: REMOVAL_ONE,
      action: "archive",
      workspaceId: "workspace-1",
      taskIds: [PARENT_TASK, CHILD_TASK],
      requestIds: [PARENT_TASK],
      departure: {
        taskId: PARENT_TASK,
        sessionId: null,
        navigationRevision: 0,
        origin: "detail",
      },
    })!;

    expect(taskRemovalOwnsDepartureForTask(state, PARENT_TASK)).toBe(true);
    expect(taskRemovalOwnsDepartureForTask(state, CHILD_TASK)).toBe(true);
    expect(taskRemovalOwnsDepartureForTask(undefined, PARENT_TASK)).toBe(false);
  });
});

describe("task removal pending membership", () => {
  function beginTwoIndependentRemovals() {
    let state = beginTaskRemoval(createTaskRemovalState(), {
      token: REMOVAL_ONE,
      action: "archive",
      workspaceId: WORKSPACE_ONE,
      taskIds: [TASK_ONE],
      requestIds: [TASK_ONE],
      departure: null,
    })!;
    state = beginTaskRemoval(state, {
      token: REMOVAL_TWO,
      action: "delete",
      workspaceId: WORKSPACE_ONE,
      taskIds: [TASK_TWO],
      requestIds: [TASK_TWO],
      departure: null,
    })!;
    return state;
  }

  it("resolves membership through the token map and its matching operation", () => {
    const state = beginTwoIndependentRemovals();

    expect(pendingTaskRemovalAction(state, TASK_ONE, WORKSPACE_ONE)).toBe("archive");
    expect(pendingTaskRemovalAction(state, TASK_TWO, WORKSPACE_ONE)).toBe("delete");
    expect(isTaskRemovalPending(state, TASK_ONE, WORKSPACE_ONE)).toBe(true);
    expect(isTaskRemovalPending(state, TASK_ONE, WORKSPACE_ONE, ["archive"])).toBe(true);
    expect(isTaskRemovalPending(state, TASK_ONE, WORKSPACE_ONE, ["delete"])).toBe(false);
    expect(pendingTaskRemovalTaskIds(state, WORKSPACE_ONE)).toEqual(new Set([TASK_ONE, TASK_TWO]));
  });

  it("does not treat a stale token or a captured workspace mismatch as membership", () => {
    const state = beginTwoIndependentRemovals();
    const stale = {
      ...state,
      pendingTokenByTaskId: { ...state.pendingTokenByTaskId, [TASK_ONE]: "missing" },
    };

    expect(pendingTaskRemovalAction(stale, TASK_ONE, WORKSPACE_ONE)).toBeNull();
    expect(isTaskRemovalPending(stale, TASK_ONE, WORKSPACE_ONE)).toBe(false);
    expect(pendingTaskRemovalTaskIds(stale, WORKSPACE_ONE)).toEqual(new Set([TASK_TWO]));
    expect(pendingTaskRemovalAction(state, TASK_ONE, WORKSPACE_TWO)).toBeNull();
    expect(pendingTaskRemovalTaskIds(state, WORKSPACE_TWO)).toEqual(new Set());
    expect(pendingTaskRemovalAction(undefined, TASK_ONE, WORKSPACE_ONE)).toBeNull();
  });

  it("releases each independent operation without clearing its sibling", () => {
    let state = beginTwoIndependentRemovals();

    state = releaseTaskRemoval(state, REMOVAL_ONE);

    expect(isTaskRemovalPending(state, TASK_ONE, WORKSPACE_ONE)).toBe(false);
    expect(isTaskRemovalPending(state, TASK_TWO, WORKSPACE_ONE)).toBe(true);

    state = releaseTaskRemoval(state, REMOVAL_TWO);

    expect(pendingTaskRemovalTaskIds(state, WORKSPACE_ONE)).toEqual(new Set());
  });
});
