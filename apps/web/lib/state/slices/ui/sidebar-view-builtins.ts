import type { SidebarView } from "./sidebar-view-types";

export const BUILTIN_VIEW_IDS = {
  ALL: "builtin-all",
  NO_PR: "builtin-no-pr",
  ACTIVE: "builtin-active",
  ARCHIVED: "builtin-archived",
} as const;

export const BUILTIN_VIEWS: SidebarView[] = [
  {
    id: BUILTIN_VIEW_IDS.ALL,
    name: "All tasks",
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "repository",
    collapsedGroups: [],
    isBuiltIn: true,
  },
  {
    id: BUILTIN_VIEW_IDS.NO_PR,
    name: "No PR reviews",
    filters: [{ id: "builtin-no-pr-f1", dimension: "hasPR", op: "is", value: false }],
    sort: { key: "state", direction: "asc" },
    group: "repository",
    collapsedGroups: [],
    isBuiltIn: true,
  },
  {
    id: BUILTIN_VIEW_IDS.ACTIVE,
    name: "Active",
    filters: [
      {
        id: "builtin-active-f1",
        dimension: "state",
        op: "in",
        value: ["review", "in_progress"],
      },
    ],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsedGroups: [],
    isBuiltIn: true,
  },
  {
    id: BUILTIN_VIEW_IDS.ARCHIVED,
    name: "Archived",
    filters: [{ id: "builtin-archived-f1", dimension: "archived", op: "is", value: true }],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsedGroups: [],
    isBuiltIn: true,
  },
];

export const DEFAULT_ACTIVE_VIEW_ID = BUILTIN_VIEW_IDS.ALL;
