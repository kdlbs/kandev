import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { countAdmittedTasks } from "@/lib/kanban/wip-limit";
import { getDestinationQueue } from "@/lib/kanban/wip-queue";
import type {
  KanbanState,
  WorkflowSnapshotData,
  WorkflowsState,
} from "@/lib/state/slices/kanban/types";

type Task = KanbanState["tasks"][number];

export type WipQueueViewModel = {
  workflowName: string | null;
  destinationTitle: string | null;
  position: number | null;
  total: number | null;
  admittedCount: number | null;
  wipLimit: number | null;
  settingsHref: string | null;
};

type WipQueueViewModelInput = {
  taskId: string;
  snapshots: Record<string, WorkflowSnapshotData>;
  activeWorkflowId: string | null;
  activeTasks: KanbanState["tasks"];
  activeSteps: KanbanState["steps"];
  workflows: WorkflowsState["items"];
};

function workflowSettingsHref(workspaceId: string, workflowId: string): string {
  return `/settings/workspaces/${encodeURIComponent(workspaceId)}/workflows#workflow-card-${encodeURIComponent(workflowId)}`;
}

function findWorkflow(workflows: WorkflowsState["items"], task: Task) {
  if (!task.workspaceId) return undefined;
  return workflows.find(
    (workflow) => workflow.id === task.workflowId && workflow.workspaceId === task.workspaceId,
  );
}

function sourceData(input: WipQueueViewModelInput, task: Task) {
  const activeTask =
    input.activeWorkflowId === task.workflowId
      ? input.activeTasks.find((candidate) => candidate.id === task.id)
      : undefined;
  if (activeTask) {
    return {
      steps: input.activeSteps,
      tasks: input.activeTasks,
      complete: true,
    };
  }
  const snapshot = input.snapshots[task.workflowId];
  if (snapshot) {
    return {
      steps: snapshot.steps,
      tasks: snapshot.tasks,
      complete: !snapshot.isPlaceholder,
    };
  }
  if (input.activeWorkflowId === task.workflowId) {
    return {
      steps: input.activeSteps,
      tasks: input.activeTasks,
      complete: true,
    };
  }
  return { steps: [], tasks: [], complete: false };
}

function findQueuedTask(
  input: WipQueueViewModelInput,
): { task: Task; destinationStepId: string } | null {
  const activeTask = input.activeTasks.find(
    (candidate) => candidate.id === input.taskId && candidate.workflowId === input.activeWorkflowId,
  );
  const task = activeTask ?? findTaskInSnapshots(input.taskId, input.snapshots, input.activeTasks);
  if (!task) return null;
  const destinationStepId = task.queuedForStepId;
  if (!destinationStepId) return null;

  // A task can remain admitted in its one-hop feeder while it waits for the
  // destination step's WIP slot. Destination-resident overflow is marked
  // !wipAdmitted; both forms carry the destination in queuedForStepId.
  if (task.workflowStepId === destinationStepId && task.wipAdmitted === true) return null;
  return { task, destinationStepId };
}

function queueEntry(
  data: ReturnType<typeof sourceData>,
  destinationStepId: string,
  taskId: string,
) {
  if (!data.complete) return undefined;
  return getDestinationQueue(data.tasks, destinationStepId).find(({ task }) => task.id === taskId);
}

function admittedCount(data: ReturnType<typeof sourceData>, destinationStepId: string) {
  if (!data.complete) return null;
  return countAdmittedTasks(
    data.tasks.filter((candidate) => candidate.workflowStepId === destinationStepId),
  );
}

function settingsHref(
  task: Task,
  workflow: ReturnType<typeof findWorkflow>,
  destinationStep: KanbanState["steps"][number] | undefined,
) {
  if (!workflow || !destinationStep || !task.workspaceId) return null;
  return workflowSettingsHref(task.workspaceId, task.workflowId);
}

export function buildWipQueueViewModel(input: WipQueueViewModelInput): WipQueueViewModel | null {
  const queued = findQueuedTask(input);
  if (!queued) return null;
  const { task, destinationStepId } = queued;

  const data = sourceData(input, task);
  const destinationStep = data.steps.find((step) => step.id === destinationStepId);
  const workflow = findWorkflow(input.workflows, task);
  const entry = queueEntry(data, destinationStepId, task.id);

  return {
    workflowName: workflow?.name ?? null,
    destinationTitle: destinationStep?.title ?? null,
    position: entry?.position ?? null,
    total: entry?.total ?? null,
    admittedCount: admittedCount(data, destinationStepId),
    wipLimit: destinationStep?.wip_limit ?? null,
    settingsHref: settingsHref(task, workflow, destinationStep),
  };
}
