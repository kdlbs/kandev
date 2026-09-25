import { describe, expect, it } from "vitest";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { buildWipQueueViewModel } from "./wip-queue-view-model";

type Task = KanbanState["tasks"][number];
const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";
const FEEDER_STEP_ID = "backlog";
const STEP_ID = "review";

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: "queued-task",
    workspaceId: WORKSPACE_ID,
    workflowId: WORKFLOW_ID,
    workflowStepId: STEP_ID,
    queuedForStepId: STEP_ID,
    wipAdmitted: false,
    title: "Queued task",
    position: 1,
    priority: "medium",
    queuedAt: "2026-09-18T10:00:00Z",
    createdAt: "2026-09-18T09:00:00Z",
    ...overrides,
  };
}

function snapshot(tasks: Task[] = [task()]): WorkflowSnapshotData {
  return {
    workflowId: WORKFLOW_ID,
    workflowName: "Development",
    steps: [
      {
        id: FEEDER_STEP_ID,
        title: "Backlog",
        color: "gray",
        position: 0,
        wip_limit: 0,
      },
      {
        id: STEP_ID,
        title: "Review",
        color: "blue",
        position: 1,
        wip_limit: 2,
      },
    ],
    tasks,
  };
}

function input(overrides: Partial<Parameters<typeof buildWipQueueViewModel>[0]> = {}) {
  return {
    taskId: "queued-task",
    snapshots: { [WORKFLOW_ID]: snapshot() },
    activeWorkflowId: WORKFLOW_ID,
    activeTasks: [],
    activeSteps: [],
    workflows: [{ id: WORKFLOW_ID, workspaceId: WORKSPACE_ID, name: "Development" }],
    ...overrides,
  };
}

describe("buildWipQueueViewModel", () => {
  it("uses the destination step and workspace identity", () => {
    const view = buildWipQueueViewModel(input());

    expect(view).toMatchObject({
      workflowName: "Development",
      destinationTitle: "Review",
      position: 1,
      total: 1,
      admittedCount: 0,
      wipLimit: 2,
      settingsHref: `/settings/workspaces/${WORKSPACE_ID}/workflows#workflow-card-${WORKFLOW_ID}`,
    });
  });

  it("keeps the WIP banner for a task without a session", () => {
    const view = buildWipQueueViewModel(
      input({ activeTasks: [task({ primarySessionId: null })], snapshots: {} }),
    );

    expect(view?.position).toBe(1);
  });

  it("prefers the active workflow projection over a stale snapshot", () => {
    const view = buildWipQueueViewModel(
      input({
        snapshots: {
          [WORKFLOW_ID]: snapshot([task()]),
        },
        activeTasks: [task({ queuedForStepId: undefined, wipAdmitted: true })],
        activeSteps: snapshot().steps,
      }),
    );

    expect(view).toBeNull();
  });

  it("keeps the WIP banner for a task queued from its feeder", () => {
    const view = buildWipQueueViewModel(
      input({
        snapshots: {
          [WORKFLOW_ID]: snapshot([
            task({
              workflowStepId: FEEDER_STEP_ID,
              queuedForStepId: STEP_ID,
              wipAdmitted: true,
            }),
          ]),
        },
      }),
    );

    expect(view).toMatchObject({
      workflowName: "Development",
      destinationTitle: "Review",
      position: null,
      total: null,
      admittedCount: 0,
      wipLimit: 2,
      settingsHref: `/settings/workspaces/${WORKSPACE_ID}/workflows#workflow-card-${WORKFLOW_ID}`,
    });
  });

  it("does not invent a workflow link when identity is unavailable", () => {
    const view = buildWipQueueViewModel(input({ workflows: [] }));

    expect(view).toMatchObject({
      workflowName: null,
      destinationTitle: "Review",
      settingsHref: null,
    });
  });

  it("does not report a WIP queue for an admitted task", () => {
    expect(
      buildWipQueueViewModel(
        input({
          snapshots: {
            [WORKFLOW_ID]: snapshot([task({ wipAdmitted: true, queuedForStepId: undefined })]),
          },
        }),
      ),
    ).toBeNull();
  });

  it("keeps position and counts unavailable for a placeholder snapshot", () => {
    const placeholder = { ...snapshot(), isPlaceholder: true };
    const view = buildWipQueueViewModel(input({ snapshots: { [WORKFLOW_ID]: placeholder } }));

    expect(view).toMatchObject({ position: null, total: null, admittedCount: null });
  });

  it("uses the active workflow projection when the snapshot is a placeholder", () => {
    const view = buildWipQueueViewModel(
      input({
        snapshots: { [WORKFLOW_ID]: { ...snapshot(), isPlaceholder: true } },
        activeTasks: [task()],
        activeSteps: snapshot().steps,
      }),
    );

    expect(view).toMatchObject({ position: 1, total: 1, admittedCount: 0 });
  });
});
