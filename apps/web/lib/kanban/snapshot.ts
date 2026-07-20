import { toKanbanTask } from "@/lib/kanban/map-task";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { Task, WorkflowSnapshot, WorkflowStepDTO } from "@/lib/types/http";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];

export function workflowStepToKanbanStep(step: WorkflowStepDTO): KanbanStep {
  return {
    id: step.id,
    title: step.name,
    color: step.color ?? "bg-neutral-400",
    position: step.position,
    events: step.events,
    allow_manual_move: step.allow_manual_move,
    prompt: step.prompt,
    is_start_step: step.is_start_step,
    show_in_command_panel: step.show_in_command_panel,
    agent_profile_id: step.agent_profile_id,
    wip_limit: step.wip_limit,
    pull_from_step_id: step.pull_from_step_id ?? null,
    stage_type: step.stage_type,
  };
}

function mergeRuntimeTaskFields(task: KanbanTask, existing: KanbanTask | undefined): KanbanTask {
  if (!existing) return task;
  return {
    ...task,
    primarySessionId: task.primarySessionId || existing.primarySessionId,
    primarySessionState: task.primarySessionState || existing.primarySessionState,
  };
}

export function workflowSnapshotToKanbanData(
  snapshot: WorkflowSnapshot,
  existing?: WorkflowSnapshotData | null,
): WorkflowSnapshotData {
  const steps = snapshot.steps.map(workflowStepToKanbanStep);
  const stepIds = new Set(steps.map((step) => step.id));
  const existingById = new Map((existing?.tasks ?? []).map((task) => [task.id, task]));
  const tasks = snapshot.tasks
    .filter((task) => !task.is_ephemeral)
    .map((task) => taskToSnapshotTask(task, stepIds, existingById.get(task.id)))
    .filter((task): task is KanbanTask => task !== null);

  return {
    workflowId: snapshot.workflow.id,
    workflowName: snapshot.workflow.name,
    steps,
    tasks,
  };
}

export function workflowSnapshotToKanbanState(
  snapshot: WorkflowSnapshot,
  existing?: KanbanState | null,
): KanbanState {
  const existingData =
    existing && existing.workflowId === snapshot.workflow.id
      ? {
          workflowId: existing.workflowId,
          workflowName: snapshot.workflow.name,
          steps: existing.steps,
          tasks: existing.tasks,
        }
      : null;
  const data = workflowSnapshotToKanbanData(snapshot, existingData);
  return {
    workflowId: data.workflowId,
    steps: data.steps,
    tasks: data.tasks,
    isLoading: false,
  };
}

function taskToSnapshotTask(
  task: Task,
  stepIds: Set<string>,
  existing: KanbanTask | undefined,
): KanbanTask | null {
  if (!task.workflow_step_id || !stepIds.has(task.workflow_step_id)) return null;
  return mergeRuntimeTaskFields(toKanbanTask(task), existing);
}
