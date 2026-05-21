import type { MentionItem } from "@/hooks/use-inline-mention";
import type { AppState } from "@/lib/state/store";

type TaskLike = AppState["kanban"]["tasks"][number];

function buildWorkflowNameMap(state: AppState): Map<string, string> {
  const m = new Map<string, string>();
  for (const w of state.workflows.items) m.set(w.id, w.name);
  for (const [wfId, snap] of Object.entries(state.kanbanMulti.snapshots)) {
    if (!m.has(wfId) && snap.workflowName) m.set(wfId, snap.workflowName);
  }
  return m;
}

function buildStepTitleMap(state: AppState): Map<string, string> {
  const m = new Map<string, string>();
  for (const s of state.kanban.steps) m.set(s.id, s.title);
  for (const snap of Object.values(state.kanbanMulti.snapshots)) {
    for (const s of snap.steps ?? []) m.set(s.id, s.title);
  }
  return m;
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
    // Guard against stale tasks left over from a previous workflow: only add
    // entries whose workflowStepId belongs to the current workflow's steps,
    // since we tag them with state.kanban.workflowId here.
    const activeStepIds = new Set(state.kanban.steps.map((s) => s.id));
    for (const t of state.kanban.tasks) {
      if (activeStepIds.size > 0 && !activeStepIds.has(t.workflowStepId)) continue;
      addTask(t, state.kanban.workflowId);
    }
  }
  for (const [wfId, snap] of Object.entries(state.kanbanMulti.snapshots)) {
    for (const t of snap.tasks) addTask(t, wfId);
  }

  return items;
}
