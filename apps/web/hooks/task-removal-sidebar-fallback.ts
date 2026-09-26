import { querySidebarTasks } from "@/lib/api";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { KanbanState } from "@/lib/state/slices";

export async function loadSidebarFallbackTasks(
  workspaceId: string | null | undefined,
): Promise<KanbanState["tasks"]> {
  if (!workspaceId) return [];
  try {
    const response = await querySidebarTasks(
      workspaceId,
      {
        filters: [{ dimension: "archived", op: "is", value: false }],
        sort: { key: "lastActivityAt", direction: "desc" },
        group: "none",
        collapsed_group_keys: [],
        collapsed_task_ids: [],
        page: 1,
        page_size: 100,
        locale: "en",
      },
      { cache: "no-store" },
    );
    return response.entries.flatMap((entry) =>
      entry.kind === "task" && entry.task ? [toKanbanTask(entry.task)] : [],
    );
  } catch {
    return [];
  }
}

export function mergeSidebarFallbackTasks(
  pageTasks: KanbanState["tasks"],
  cachedTasks: KanbanState["tasks"],
): KanbanState["tasks"] {
  const seen = new Set<string>();
  return [...pageTasks, ...cachedTasks].filter((task) => {
    if (seen.has(task.id)) return false;
    seen.add(task.id);
    return true;
  });
}
