import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/app-state-types";
import type { AggregatedSidebarTasks } from "./task-session-sidebar-aggregate";
import { findSidebarTask } from "./task-session-sidebar-task-lookup";

type SidebarTask = AggregatedSidebarTasks["allTasks"][number];

function task(id: string, overrides: Partial<SidebarTask> = {}): SidebarTask {
  return { id, _workflowId: "workflow", isArchived: false, ...overrides } as SidebarTask;
}

function state(
  overrides: Partial<Pick<AppState, "kanbanMulti" | "kanban" | "sidebarArchivedTasks">> = {},
) {
  return {
    kanbanMulti: { snapshots: {} },
    kanban: { tasks: [] },
    sidebarArchivedTasks: {
      itemsByWorkspaceId: {},
      loadedByWorkspaceId: {},
      loadingByWorkspaceId: {},
      errorByWorkspaceId: {},
      revisionByWorkspaceId: {},
    },
    ...overrides,
  } as unknown as Pick<AppState, "kanbanMulti" | "kanban" | "sidebarArchivedTasks">;
}

describe("findSidebarTask", () => {
  it("uses current-page metadata when the paged task is absent from store caches", () => {
    const archived = task("archived", { isArchived: true, primarySessionId: "session-1" });

    expect(findSidebarTask(state(), "archived", [archived])).toBe(archived);
  });

  it("prefers the page projection over a stale cached task", () => {
    const current = task("task", { isArchived: true, primarySessionId: "new-session" });
    const stale = task("task", { primarySessionId: "old-session" });
    const store = state({ kanban: { tasks: [stale] } as unknown as AppState["kanban"] });

    expect(findSidebarTask(store, "task", [current])).toBe(current);
  });

  it("falls back to active snapshots and archived cache rows", () => {
    const active = task("active");
    const archived = task("archived", { isArchived: true });
    const store = state({
      kanban: { tasks: [active] } as unknown as AppState["kanban"],
      sidebarArchivedTasks: {
        itemsByWorkspaceId: { workspace: [archived] },
        loadedByWorkspaceId: {},
        loadingByWorkspaceId: {},
        errorByWorkspaceId: {},
        revisionByWorkspaceId: {},
      },
    });

    expect(findSidebarTask(store, "active", [])).toBe(active);
    expect(findSidebarTask(store, "archived", [])).toBe(archived);
  });
});
