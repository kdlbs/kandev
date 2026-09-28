import { querySidebarTasks } from "@/lib/api";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { KanbanState } from "@/lib/state/slices";
import type { SidebarTaskPageResponse } from "@/lib/types/http";

export type SidebarFallbackPage = {
  tasks: KanbanState["tasks"];
  hasNext: boolean;
};

export type SidebarFallbackSelectionOptions = {
  excludedTaskIds?: ReadonlySet<string>;
  workspaceId?: string | null;
  validateTaskAncestry?: boolean;
};

type SidebarFallbackSelector = (
  tasks: KanbanState["tasks"],
  removedTaskId: string,
  isLive: (taskId: string) => Promise<boolean>,
  options?: SidebarFallbackSelectionOptions,
) => Promise<KanbanState["tasks"][number] | null>;

export async function findSidebarFallbackTask(params: {
  workspaceId: string | null | undefined;
  taskId: string;
  cachedTasks: KanbanState["tasks"];
  excludedTaskIds?: ReadonlySet<string>;
  validateTaskAncestry?: boolean;
  isLive: (taskId: string) => Promise<boolean>;
  selectCandidate: SidebarFallbackSelector;
}): Promise<KanbanState["tasks"][number] | null> {
  const {
    workspaceId,
    taskId,
    cachedTasks,
    excludedTaskIds,
    validateTaskAncestry,
    isLive,
    selectCandidate,
  } = params;
  const selectionOptions = { excludedTaskIds, workspaceId, validateTaskAncestry };
  const liveByTaskId = new Map<string, Promise<boolean>>();
  const memoizedIsLive = (candidateTaskId: string) => {
    let result = liveByTaskId.get(candidateTaskId);
    if (!result) {
      result = isLive(candidateTaskId);
      liveByTaskId.set(candidateTaskId, result);
    }
    return result;
  };
  const cachedCandidate = await selectCandidate(
    cachedTasks,
    taskId,
    memoizedIsLive,
    selectionOptions,
  );
  if (cachedCandidate) return cachedCandidate;

  const serverTasks: KanbanState["tasks"] = [];
  let pageNumber = 1;
  while (true) {
    const page = await loadSidebarFallbackTasks(workspaceId, pageNumber);
    serverTasks.push(...page.tasks);
    const combinedTasks = mergeSidebarFallbackTasks(serverTasks, cachedTasks);
    const pageCandidate = await selectCandidate(
      combinedTasks,
      taskId,
      memoizedIsLive,
      selectionOptions,
    );
    if (pageCandidate) return pageCandidate;
    if (!page.hasNext) return null;
    pageNumber += 1;
  }
}

export async function loadSidebarFallbackTasks(
  workspaceId: string | null | undefined,
  page: number,
): Promise<SidebarFallbackPage> {
  if (!workspaceId) return { tasks: [], hasNext: false };
  try {
    const response: SidebarTaskPageResponse = await querySidebarTasks(
      workspaceId,
      {
        filters: [{ dimension: "archived", op: "is", value: false }],
        sort: { key: "lastActivityAt", direction: "desc" },
        group: "none",
        collapsed_group_keys: [],
        collapsed_task_ids: [],
        page,
        page_size: 100,
        locale: "en",
      },
      { cache: "no-store" },
    );
    return {
      tasks: response.entries.flatMap((entry) =>
        entry.kind === "task" && entry.task ? [toKanbanTask(entry.task)] : [],
      ),
      hasNext: response.has_next,
    };
  } catch {
    return { tasks: [], hasNext: false };
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
