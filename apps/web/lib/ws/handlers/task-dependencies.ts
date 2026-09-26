import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

/**
 * Dependency edges must survive a task.updated that does not mention them.
 *
 * `toKanbanTask` defaults the projection to "no edges", which is right for a
 * boot payload (the backend always computes it there) but wrong for a
 * lightweight event: most task.updated publishers carry no dependency fields, so
 * defaulting would erase the hydrated edges and make the dependency chip vanish
 * as soon as the task's agent produced any activity.
 */
export function preserveOmittedDependencyFields(
  existing: KanbanTask,
  merged: KanbanTask,
  payload: Record<string, unknown>,
): void {
  // "Mentions dependencies" means the key is present at all, even as null —
  // an explicit empty list is a real clear and must win.
  const mentions = "depends_on" in payload || "blocks" in payload || "blocked" in payload;
  if (mentions) return;
  merged.blocked = existing.blocked;
  merged.blockedReason = existing.blockedReason;
  merged.dependsOn = existing.dependsOn;
  merged.blocks = existing.blocks;
  merged.startWhenUnblocked = existing.startWhenUnblocked;
}
