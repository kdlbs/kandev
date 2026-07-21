import type { SidebarView } from "./sidebar-view-types";

export const DEFAULT_VIEW_ID = "view-all-tasks";
export const MAX_SIDEBAR_VIEWS = 50;

export function createDefaultSidebarView(id: string, name: string): SidebarView {
  return {
    id,
    name,
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "repository",
    collapsedGroups: [],
  };
}

export const DEFAULT_VIEW: SidebarView = createDefaultSidebarView(DEFAULT_VIEW_ID, "All tasks");

export const DEFAULT_ACTIVE_VIEW_ID = DEFAULT_VIEW_ID;
