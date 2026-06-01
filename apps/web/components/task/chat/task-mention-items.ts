import type { MentionItem } from "@/hooks/use-inline-mention";
import type { WorkflowSnapshotData, WorkflowsState } from "@/lib/state/slices/kanban/types";

type TaskLike = WorkflowSnapshotData["tasks"][number];

/**
 * Read-side input for {@link buildTaskMentionItems}. Sourced from the TanStack
 * Query kanban caches (`qk.kanban.multi()` snapshots + `qk.kanban.workflowsList`)
 * rather than the Zustand mirror.
 */
export type TaskMentionSource = {
  snapshots: Record<string, WorkflowSnapshotData>;
  workflows: WorkflowsState["items"];
};

function buildWorkflowNameMap(source: TaskMentionSource): Map<string, string> {
  const m = new Map<string, string>();
  for (const w of source.workflows) m.set(w.id, w.name);
  for (const [wfId, snap] of Object.entries(source.snapshots)) {
    if (!m.has(wfId) && snap.workflowName) m.set(wfId, snap.workflowName);
  }
  return m;
}

function buildStepTitleMap(source: TaskMentionSource): Map<string, string> {
  const m = new Map<string, string>();
  for (const snap of Object.values(source.snapshots)) {
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
  source: TaskMentionSource,
  currentTaskId: string | null,
): MentionItem[] {
  const items: MentionItem[] = [];
  const seen = new Set<string>();
  const workflowNameById = buildWorkflowNameMap(source);
  const stepTitleById = buildStepTitleMap(source);

  const addTask = (task: TaskLike, workflowId: string) => {
    if (task.id === currentTaskId || seen.has(task.id)) return;
    seen.add(task.id);
    const workflowName = workflowNameById.get(workflowId) ?? "Workflow";
    const stepTitle = stepTitleById.get(task.workflowStepId) ?? "Step";
    items.push(toMentionItem(task, workflowId, workflowName, stepTitle));
  };

  for (const [wfId, snap] of Object.entries(source.snapshots)) {
    for (const t of snap.tasks) addTask(t, wfId);
  }

  return items;
}
