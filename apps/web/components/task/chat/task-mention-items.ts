import type { MentionItem } from "@/hooks/use-inline-mention";
import type { AppState } from "@/lib/state/store";

type TaskLike = AppState["kanban"]["tasks"][number];

function buildWorkflowNameMap(state: AppState): Map<string, string> {
  const names = new Map<string, string>();
  for (const workflow of state.workflows.items) names.set(workflow.id, workflow.name);
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
    if (!names.has(workflowId) && snapshot.workflowName) {
      names.set(workflowId, snapshot.workflowName);
    }
  }
  return names;
}

function buildStepTitleMap(state: AppState): Map<string, string> {
  const titles = new Map<string, string>();
  for (const step of state.kanban.steps) titles.set(step.id, step.title);
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    for (const step of snapshot.steps ?? []) titles.set(step.id, step.title);
  }
  return titles;
}

function toMentionItem(
  task: TaskLike,
  workflowId: string,
  workflowName: string,
  stepTitle: string,
): MentionItem {
  return {
    id: `task:${task.id}`,
    kind: "task",
    label: task.title,
    description: `${workflowName} · ${stepTitle}`,
    task: {
      taskId: task.id,
      title: task.title,
      workflowId,
      workflowStepId: task.workflowStepId,
      state: task.state ?? null,
    },
    onSelect: () => {},
  };
}

export function buildTaskMentionItems(
  state: AppState,
  currentTaskId: string | null,
): MentionItem[] {
  const items: MentionItem[] = [];
  const seen = new Set<string>();
  const workflowNameById = buildWorkflowNameMap(state);
  const stepTitleById = buildStepTitleMap(state);

  const addTask = (task: TaskLike, workflowId: string) => {
    if (task.id === currentTaskId || seen.has(task.id)) return;
    seen.add(task.id);
    const workflowName = workflowNameById.get(workflowId) ?? "Workflow";
    const stepTitle = stepTitleById.get(task.workflowStepId) ?? "Step";
    items.push(toMentionItem(task, workflowId, workflowName, stepTitle));
  };

  if (state.kanban.workflowId) {
    const activeStepIds = new Set(state.kanban.steps.map((step) => step.id));
    for (const task of state.kanban.tasks) {
      if (activeStepIds.size > 0 && !activeStepIds.has(task.workflowStepId)) continue;
      addTask(task, state.kanban.workflowId);
    }
  }
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
    for (const task of snapshot.tasks) addTask(task, workflowId);
  }

  return items;
}
