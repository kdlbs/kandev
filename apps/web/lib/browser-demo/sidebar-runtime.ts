import { toKanbanTask } from "@/lib/kanban/map-task";
import {
  projectLocalSidebarTasks,
  type LocalSidebarMetadata,
} from "@/lib/sidebar/sidebar-local-projection";
import { localSidebarPage } from "@/lib/sidebar/sidebar-local-view";
import type { SidebarTaskQuery, Task } from "@/lib/types/http";
import { createBootPayload, DEMO_IDS, type DemoState } from "./scenario";

export function createDemoSidebarPage(state: DemoState, query: SidebarTaskQuery) {
  const metadata = createBootPayload(state).initialState as LocalSidebarMetadata;
  const tasks = projectLocalSidebarTasks(
    state.tasks.map(toKanbanTask),
    metadata,
    DEMO_IDS.workspace,
  );
  const page = localSidebarPage(tasks, query, { pinnedTaskIds: [], orderedTaskIds: [] });
  const byId = new Map<string, Task>(state.tasks.map((task) => [task.id, task]));
  return {
    ...page,
    entries: page.entries.map((entry) => ({
      ...entry,
      ...(entry.task_id ? { task: byId.get(entry.task_id) } : {}),
    })),
  };
}
