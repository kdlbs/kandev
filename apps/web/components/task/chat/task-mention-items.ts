import type { MentionItem } from "@/hooks/use-inline-mention";
import type { WorkflowItem, WorkflowSnapshotData } from "@/lib/state/slices";

type TaskLike = WorkflowSnapshotData["tasks"][number];
type WorkflowSnapshots = Record<string, WorkflowSnapshotData>;

function buildWorkflowNameMap(
  snapshots: WorkflowSnapshots,
  workflows: WorkflowItem[],
): Map<string, string> {
  const m = new Map<string, string>();
  for (const w of workflows) m.set(w.id, w.name);
  for (const [wfId, snap] of Object.entries(snapshots)) {
    if (!m.has(wfId) && snap.workflowName) m.set(wfId, snap.workflowName);
  }
  return m;
}

function buildStepTitleMap(snapshots: WorkflowSnapshots): Map<string, string> {
  const m = new Map<string, string>();
  for (const snap of Object.values(snapshots)) {
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
  snapshots: WorkflowSnapshots,
  currentTaskId: string | null,
  workflows: WorkflowItem[] = [],
): MentionItem[] {
  const items: MentionItem[] = [];
  const seen = new Set<string>();
  const workflowNameById = buildWorkflowNameMap(snapshots, workflows);
  const stepTitleById = buildStepTitleMap(snapshots);

  const addTask = (task: TaskLike, workflowId: string) => {
    if (task.id === currentTaskId || seen.has(task.id)) return;
    seen.add(task.id);
    const workflowName = workflowNameById.get(workflowId) ?? "Workflow";
    const stepTitle = stepTitleById.get(task.workflowStepId) ?? "Step";
    items.push(toMentionItem(task, workflowId, workflowName, stepTitle));
  };

  for (const [wfId, snap] of Object.entries(snapshots)) {
    const stepIds = new Set(snap.steps.map((s) => s.id));
    for (const t of snap.tasks) {
      if (stepIds.size > 0 && !stepIds.has(t.workflowStepId)) continue;
      addTask(t, wfId);
    }
  }

  return items;
}
