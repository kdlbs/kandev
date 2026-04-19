import type { SidebarView } from "./sidebar-view-types";

export const DEFAULT_VIEW_ID = "view-all-tasks";

export const DEFAULT_VIEW: SidebarView = {
  id: DEFAULT_VIEW_ID,
  name: "All tasks",
  filters: [],
  sort: { key: "state", direction: "asc" },
  group: "repository",
  collapsedGroups: [],
};

export const DEFAULT_ACTIVE_VIEW_ID = DEFAULT_VIEW_ID;
