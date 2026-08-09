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

/**
 * Catalog key for the built-in view's name.
 *
 * `SidebarView.name` is persisted and user-editable (the view manager renames
 * it in place), so the built-in's name stays canonical English in state and the
 * surfaces that display it resolve this key instead — the same key/persisted-
 * value split the layout profiles and dockview panel titles use. Translating
 * `name` here would write the current locale into the user's synced views.
 */
export const DEFAULT_VIEW_NAME_KEY = "sidebar:viewAllTasks";

export const DEFAULT_VIEW: SidebarView = createDefaultSidebarView(DEFAULT_VIEW_ID, "All tasks");

/** Display name for a view: the built-in resolves through the catalog. */
export function sidebarViewName(
  view: Pick<SidebarView, "id" | "name">,
  translate: (key: string) => string,
): string {
  return view.id === DEFAULT_VIEW_ID ? translate(DEFAULT_VIEW_NAME_KEY) : view.name;
}

export const DEFAULT_ACTIVE_VIEW_ID = DEFAULT_VIEW_ID;
