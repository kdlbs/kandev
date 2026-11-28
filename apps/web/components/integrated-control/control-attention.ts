import type { NativeTask } from "@/lib/types/control-native";
import {
  taskSignal,
  workspaceStatus,
  type WorkspaceFigures,
  type TaskSignal,
} from "./control-signals";
import type { RepeatedMoves } from "./control-step-moves";
import type { WorkspaceInboxFigures } from "./control-inbox-model";

export type AttentionKind = "question" | "repeat" | "stalled" | "failed" | "config";

export type AttentionItem = {
  kind: AttentionKind;
  scope: "workspace" | "task" | "global";
  workspaceId?: string;
  task?: NativeTask;
  repeat?: RepeatedMoves;
  figures?: WorkspaceFigures;
  /** For the coordinator rule: how many workspaces have none. */
  workspaceCount?: number;
  /** For a question row: what the workspace's Needs-you inbox is waiting on. */
  inbox?: WorkspaceInboxFigures;
};

export type AttentionInput = {
  workspaceIds: string[];
  /** Recorded figures per workspace; a missing entry is an unread workspace, never zero. */
  figures: Record<string, WorkspaceFigures | undefined>;
  /** Sessions waiting on the capacity ceiling, or null when unknown. */
  waiting: number | null;
  tasks: NativeTask[];
  repeats: Map<string, RepeatedMoves>;
  coordinators: { enabled: boolean; known: boolean; total: number };
  /** Needs-you figures per workspace; a workspace with nothing waiting is absent. */
  inbox?: Record<string, WorkspaceInboxFigures>;
};

const TASK_KINDS: Partial<Record<TaskSignal, AttentionKind>> = {
  repeat: "repeat",
  stalled: "stalled",
  failed: "failed",
};
const ORDER: Record<string, number> = {
  "question:workspace": -1,
  "repeat:task": 0,
  "stalled:workspace": 1,
  "stalled:task": 2,
  "failed:task": 3,
  "config:global": 4,
};

/**
 * The rows the overview raises for a person, from recorded state only:
 * a workspace whose Needs-you inbox holds questions or recovery decisions
 * (listed first), a stalled workspace, a task with unconsumed messages and nothing running,
 * a failed primary session on unfinished work, a task moving between steps
 * at the repeat rate, and coordinators switched on with none configured.
 * Each task appears once, under its most urgent signal.
 */
export function buildAttention(input: AttentionInput): AttentionItem[] {
  const items: AttentionItem[] = [];
  for (const workspaceId of input.workspaceIds) {
    const inbox = input.inbox?.[workspaceId];
    if (inbox) items.push({ kind: "question", scope: "workspace", workspaceId, inbox });
    const figures = input.figures[workspaceId];
    if (figures && workspaceStatus(figures, input.waiting) === "stalled")
      items.push({ kind: "stalled", scope: "workspace", workspaceId, figures });
  }
  const repeating = new Set(input.repeats.keys());
  const seen = new Set<string>();
  for (const task of input.tasks) {
    seen.add(task.task_id);
    const kind = TASK_KINDS[taskSignal(task, repeating)];
    if (!kind) continue;
    items.push({
      kind,
      scope: "task",
      workspaceId: task.workspace_id,
      task,
      repeat: input.repeats.get(task.task_id),
    });
  }
  for (const repeat of input.repeats.values()) {
    if (seen.has(repeat.taskId)) continue;
    items.push({ kind: "repeat", scope: "task", workspaceId: repeat.workspaceId, repeat });
  }
  const { enabled, known, total } = input.coordinators;
  if (enabled && known && total === 0 && input.workspaceIds.length > 0)
    items.push({ kind: "config", scope: "global", workspaceCount: input.workspaceIds.length });
  return items.sort(
    (a, b) => ORDER[`${a.kind}:${a.scope}`] - ORDER[`${b.kind}:${b.scope}`] || since(a) - since(b),
  );
}

/** When the condition was last recorded as changing; earlier sorts first. */
function since(item: AttentionItem): number {
  const value = item.task?.updated_at ?? item.repeat?.end ?? item.inbox?.oldest;
  const time = value ? Date.parse(value) : NaN;
  return Number.isNaN(time) ? Number.MAX_SAFE_INTEGER : time;
}

export function countAttention(items: AttentionItem[]): Record<AttentionKind | "all", number> {
  const counts = { all: items.length, question: 0, repeat: 0, stalled: 0, failed: 0, config: 0 };
  for (const item of items) counts[item.kind] += 1;
  return counts;
}
