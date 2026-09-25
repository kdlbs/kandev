import { describe, expect, it } from "vitest";
import { repositoryId, taskId, workflowId, type Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import {
  buildTaskFromKanban,
  resolveEffectiveTask,
  resolveLatestTaskProjection,
  resolveWorkflowCurrentStepId,
} from "./task-page-content-helpers";

type KanbanTask = KanbanState["tasks"][number];

const TASK_ID = "task-1";
const SOURCE_WORKFLOW = "workflow-source";
const DESTINATION_WORKFLOW = "workflow-destination";
const SOURCE_STEP = "step-source";
const DESTINATION_STEP = "step-destination";
const SOURCE_UPDATED_AT = "2026-07-18T00:00:00Z";
const DESTINATION_UPDATED_AT = "2026-07-19T00:00:00Z";

function makeTaskDetails(overrides: Partial<Task> = {}): Task {
  return {
    id: taskId(TASK_ID),
    title: "Workflow migration task",
    description: "Task details",
    workflow_step_id: SOURCE_STEP,
    position: 0,
    state: "TODO",
    workspace_id: "workspace-1",
    workflow_id: workflowId(SOURCE_WORKFLOW),
    priority: "medium",
    repositories: [],
    created_at: "",
    updated_at: SOURCE_UPDATED_AT,
    ...overrides,
  } as Task;
}

function makeKanbanTask(overrides: Partial<KanbanTask> = {}): KanbanTask {
  return {
    id: TASK_ID,
    title: "Workflow migration task",
    workflowId: SOURCE_WORKFLOW,
    workflowStepId: SOURCE_STEP,
    position: 0,
    state: "TODO",
    ...overrides,
  } as KanbanTask;
}

describe("task workflow placement", () => {
  // @covers AC-TASKS-CHANGE-WORKFLOW-001.8
  it("applies a newer destination placement and retains task-only details", () => {
    const repositories: Task["repositories"] = [
      {
        id: "task-repo-1",
        task_id: taskId(TASK_ID),
        repository_id: repositoryId("repo-1"),
        base_branch: "main",
        position: 0,
        created_at: "",
        updated_at: "",
      },
    ];
    const details = makeTaskDetails({ repositories });
    const destination = makeKanbanTask({
      workflowId: DESTINATION_WORKFLOW,
      workflowStepId: DESTINATION_STEP,
      updatedAt: DESTINATION_UPDATED_AT,
    });

    const resolved = resolveEffectiveTask(details, null, destination, TASK_ID);

    expect(resolved).toMatchObject({
      workflow_id: DESTINATION_WORKFLOW,
      workflow_step_id: DESTINATION_STEP,
      repositories,
    });
  });

  // @covers AC-TASKS-CHANGE-WORKFLOW-001.8
  it("rejects an older source placement as one workflow and step pair", () => {
    const details = makeTaskDetails({
      workflow_id: workflowId(DESTINATION_WORKFLOW),
      workflow_step_id: DESTINATION_STEP,
      updated_at: DESTINATION_UPDATED_AT,
    });
    const staleSource = makeKanbanTask({ updatedAt: SOURCE_UPDATED_AT });

    const resolved = resolveEffectiveTask(details, null, staleSource, TASK_ID);

    expect(resolved).toMatchObject({
      workflow_id: DESTINATION_WORKFLOW,
      workflow_step_id: DESTINATION_STEP,
    });
  });

  // @covers AC-TASKS-CHANGE-WORKFLOW-001.8
  it("preserves the known placement when a live row omits its destination step", () => {
    const details = makeTaskDetails();
    const partial = makeKanbanTask({
      workflowId: DESTINATION_WORKFLOW,
      workflowStepId: "",
    });

    const resolved = resolveEffectiveTask(details, null, partial, TASK_ID);

    expect(resolved).toMatchObject({
      workflow_id: SOURCE_WORKFLOW,
      workflow_step_id: SOURCE_STEP,
    });
  });
});

describe("task workflow projection selection", () => {
  // @covers AC-TASKS-CHANGE-WORKFLOW-001.8
  it("selects the newer destination row when an older source snapshot remains", () => {
    const source = makeKanbanTask({ updatedAt: SOURCE_UPDATED_AT });
    const destination = makeKanbanTask({
      workflowId: DESTINATION_WORKFLOW,
      workflowStepId: DESTINATION_STEP,
      updatedAt: DESTINATION_UPDATED_AT,
    });

    const selected = resolveLatestTaskProjection(TASK_ID, [source], {
      [SOURCE_WORKFLOW]: { tasks: [source] },
      [DESTINATION_WORKFLOW]: { tasks: [destination] },
    });

    expect(selected).toBe(destination);
  });

  // @covers AC-TASKS-CHANGE-WORKFLOW-001.8
  it("uses a task step when the session step belongs to the source workflow", () => {
    expect(
      resolveWorkflowCurrentStepId(SOURCE_STEP, DESTINATION_STEP, [DESTINATION_STEP, "step-next"]),
    ).toBe(DESTINATION_STEP);
  });

  it("uses the fresh task-detail step over an older cached step in the same workflow", () => {
    const details = makeTaskDetails({
      workflow_id: workflowId(SOURCE_WORKFLOW),
      workflow_step_id: DESTINATION_STEP,
      updated_at: DESTINATION_UPDATED_AT,
    });
    const staleCachedTask = makeKanbanTask({
      workflowId: SOURCE_WORKFLOW,
      workflowStepId: SOURCE_STEP,
      updatedAt: SOURCE_UPDATED_AT,
    });
    const resolvedTask = resolveEffectiveTask(details, null, staleCachedTask, TASK_ID);

    expect(
      resolveWorkflowCurrentStepId(
        staleCachedTask.workflowStepId,
        resolvedTask?.workflow_step_id ?? null,
        [SOURCE_STEP, DESTINATION_STEP],
      ),
    ).toBe(DESTINATION_STEP);
  });

  it("includes the workflow ID when building a task from a projection", () => {
    const task = buildTaskFromKanban(
      makeKanbanTask({ workflowId: DESTINATION_WORKFLOW, workflowStepId: DESTINATION_STEP }),
    );

    expect(task.workflow_id).toBe(DESTINATION_WORKFLOW);
  });
});
