import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";

import { snapshotToState } from "./mapper";
import { hydrateState } from "@/lib/state/hydration/hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { WorkflowSnapshot } from "@/lib/types/http";
import { partitionWipTasks } from "@/lib/kanban/wip-queue";

const now = "2026-07-10T12:00:00Z";
const workflowID = workflowId("workflow-1");
const workspaceID = workspaceId("workspace-1");

function snapshotWithPendingAction(action: unknown): WorkflowSnapshot {
  return {
    workflow: {
      id: workflowID,
      workspace_id: workspaceID,
      name: "Workflow",
      created_at: now,
      updated_at: now,
    },
    steps: [
      {
        id: "step-1",
        workflow_id: workflowID,
        name: "Todo",
        position: 0,
        color: "bg-neutral-400",
        allow_manual_move: true,
      },
    ],
    tasks: [
      {
        id: taskId("task-1"),
        workspace_id: workspaceID,
        workflow_id: workflowID,
        workflow_step_id: "step-1",
        position: 0,
        title: "Task",
        description: "",
        state: "TODO",
        priority: "medium",
        primary_session_pending_action: action,
        created_at: now,
        updated_at: now,
      } as WorkflowSnapshot["tasks"][number],
    ],
  };
}

const EXECUTOR_ID = "exec-worktree";
const EXECUTOR_TYPE = "worktree";
const EXECUTOR_NAME = "Worktree";

function snapshotWithExecutor(): WorkflowSnapshot {
  const snapshot = snapshotWithPendingAction(undefined);
  snapshot.tasks[0].primary_executor_id = EXECUTOR_ID;
  snapshot.tasks[0].primary_executor_type = EXECUTOR_TYPE;
  snapshot.tasks[0].primary_executor_name = EXECUTOR_NAME;
  return snapshot;
}

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("snapshotToState", () => {
  it("keeps known primary session pending action values", () => {
    const state = snapshotToState(snapshotWithPendingAction("permission"));

    expect(state.kanban?.tasks[0]?.primarySessionPendingAction).toBe("permission");
  });

  it("drops unrecognized primary session pending action values", () => {
    const state = snapshotToState(snapshotWithPendingAction("unknown"));

    expect(state.kanban?.tasks[0]?.primarySessionPendingAction).toBeUndefined();
  });

  // The server-rendered boot payload is the first thing a board paints from,
  // so a dropped assignee here shows every card as unassigned until the first
  // WS update happens to arrive.
  it("hydrates the human assignee into the initial kanban state", () => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].assignee_user_id = "user-7";

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.assigneeUserId).toBe("user-7");
  });

  it("hydrates task metadata into the initial kanban state", () => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].metadata = {
      port_forwarding_enabled: true,
      unrelated: "preserve",
    };

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.metadata).toEqual(snapshot.tasks[0].metadata);
  });

  it.each([
    [0, 0],
    [3, 3],
    [undefined, undefined],
  ])("maps active subagent count %s", (wireValue, expected) => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].active_subagent_count = wireValue;

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.activeSubagentCount).toBe(expected);
  });

  it.each([
    [true, true],
    [false, false],
    [undefined, false],
  ])(
    "maps auto_start_failed %s so a page reload does not hide or resurrect the badge",
    (wireValue, expected) => {
      const snapshot = snapshotWithPendingAction(undefined);
      snapshot.tasks[0].auto_start_failed = wireValue;

      const state = snapshotToState(snapshot);

      expect(state.kanban?.tasks[0]?.autoStartFailed).toBe(expected);
    },
  );

  it("hydrates the task status summary into the initial kanban state", () => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].status_summary = {
      revision: 4,
      updated_at: now,
      primary_session: { id: "session-1", state: "WAITING_FOR_INPUT" },
    };

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.statusSummary).toEqual(snapshot.tasks[0].status_summary);
  });

  it("maps the primary executor binding onto the kanban task", () => {
    const snapshot = snapshotWithExecutor();
    snapshot.tasks[0].is_remote_executor = false;

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]).toMatchObject({
      primaryExecutorId: EXECUTOR_ID,
      primaryExecutorType: EXECUTOR_TYPE,
      primaryExecutorName: EXECUTOR_NAME,
      isRemoteExecutor: false,
    });
  });

  it("leaves the executor binding unset when the snapshot omits it", () => {
    // The backend marks every executor field `omitempty`, so a task whose
    // primary session has not bound an executor yet arrives with the keys
    // absent rather than null. The mapper must yield undefined here, never
    // crash and never invent a binding.
    const state = snapshotToState(snapshotWithPendingAction(undefined));

    expect(state.kanban?.tasks[0]?.primaryExecutorId).toBeUndefined();
    expect(state.kanban?.tasks[0]?.primaryExecutorType).toBeUndefined();
    expect(state.kanban?.tasks[0]?.primaryExecutorName).toBeUndefined();
    expect(state.kanban?.tasks[0]?.isRemoteExecutor).toBe(false);
  });

  it("leaves an explicitly undefined executor binding unset", () => {
    // Reviewer-requested wire-contract coverage. JSON omits undefined values,
    // but typed snapshot fixtures can retain the property; both representations
    // must produce the same unresolved client state.
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].primary_executor_type = undefined;

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.primaryExecutorType).toBeUndefined();
  });

  it("preserves workflow step WIP fields", () => {
    const state = snapshotToState({
      workflow: {
        id: workflowID,
        workspace_id: workspaceID,
        name: "Workflow",
        created_at: now,
        updated_at: now,
      },
      steps: [
        {
          id: "step-1",
          workflow_id: workflowID,
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          allow_manual_move: true,
          wip_limit: 2,
          pull_from_step_id: "step-0",
        },
      ],
      tasks: [],
    } as unknown as WorkflowSnapshot);

    expect(state.kanban?.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("forwards task WIP admission + overflow fields into the kanban state", () => {
    // The HTTP snapshot path must preserve the backend queue projection.
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].state = "IN_PROGRESS";
    snapshot.tasks[0].priority = "high";
    snapshot.tasks[0].wip_admitted = false;
    snapshot.tasks[0].queued_for_step_id = "step-1";
    snapshot.tasks[0].queued_at = now;
    snapshot.tasks.push({
      ...snapshot.tasks[0],
      id: taskId("task-2"),
      priority: "critical",
    });

    const state = snapshotToState(snapshot);
    const task = state.kanban?.tasks.find((candidate) => candidate.id === taskId("task-1"));
    const tasks = state.kanban?.tasks ?? [];

    expect(task).toBeDefined();
    if (!task) return;

    expect(task).toMatchObject({
      state: "IN_PROGRESS",
      priority: "high",
      createdAt: now,
      wipAdmitted: false,
      queuedForStepId: "step-1",
      queuedAt: now,
    });
    const partition = partitionWipTasks(tasks, "step-1");
    expect(partition).toMatchObject({ admitted: [] });
    expect(partition.queued.map((queuedTask) => queuedTask.id)).toEqual([
      taskId("task-2"),
      taskId("task-1"),
    ]);
  });
});

describe("snapshotToState feeding hydrateState", () => {
  it("does not blank an already-known executor when a re-hydration lands", () => {
    // Regression guard for the bug this change fixes, exercised through the
    // real chain: use-workflow-snapshot calls hydrate(snapshotToState(...)) on
    // (re)mount, and mergeKanbanTasks replaces the cached task wholesale when
    // the incoming updatedAt is not older. While snapshotToState dropped the
    // executor fields, that replacement silently reset primaryExecutorType to
    // undefined and the "Add folder" control disappeared.
    const draft = structuredClone(defaultState) as AppState;
    draft.kanban.tasks = [
      {
        id: taskId("task-1"),
        workflowId: workflowID,
        workflowStepId: "step-1",
        title: "Task",
        position: 0,
        updatedAt: now,
        primaryExecutorId: EXECUTOR_ID,
        primaryExecutorType: EXECUTOR_TYPE,
        primaryExecutorName: EXECUTOR_NAME,
      },
    ] as AppState["kanban"]["tasks"];

    const result = produce(draft, (d: Draft<AppState>) => {
      hydrateState(d, snapshotToState(snapshotWithExecutor()));
    });

    expect(result.kanban.tasks[0]?.primaryExecutorType).toBe(EXECUTOR_TYPE);
  });
});
