import type { ThreadView } from "./thread-view-types";

export const DEFAULT_THREAD_VIEW_ID = "view-all-threads";
// i18n-exempt: canonical persisted name for the built-in view; renderers may translate this id.
export const DEFAULT_THREAD_VIEW_NAME = "All threads";
export const DEFAULT_THREAD_VIEW_NAME_KEY = "threads:allThreads";
export const DEFAULT_THREAD_VIEW_MAX_COLUMNS = 5;
export const MAX_THREAD_VIEWS = 50;

export function createDefaultThreadView(id: string, name: string): ThreadView {
  return {
    id,
    name,
    taskScope: { mode: "all", taskIds: [] },
    filters: [],
    sort: { key: "attention", direction: "asc" },
    maxColumns: DEFAULT_THREAD_VIEW_MAX_COLUMNS,
  };
}

// i18n-exempt: canonical English persisted as the built-in name; renderers may translate this id.
export const DEFAULT_THREAD_VIEW: ThreadView = createDefaultThreadView(
  DEFAULT_THREAD_VIEW_ID,
  DEFAULT_THREAD_VIEW_NAME,
);

export const DEFAULT_THREAD_ACTIVE_VIEW_ID = DEFAULT_THREAD_VIEW_ID;

export function threadViewName(
  view: Pick<ThreadView, "id" | "name">,
  translate: (key: string) => string,
): string {
  const isUnrenamedBuiltIn =
    view.id === DEFAULT_THREAD_VIEW_ID && view.name === DEFAULT_THREAD_VIEW_NAME;
  return isUnrenamedBuiltIn ? translate(DEFAULT_THREAD_VIEW_NAME_KEY) : view.name;
}

export function normalizeThreadViews(views: ThreadView[] | undefined): ThreadView[] {
  if (!views || views.length === 0) return [DEFAULT_THREAD_VIEW];
  const seen = new Set<string>();
  const valid = views.filter((view) => {
    if (!view.id.trim() || !view.name.trim() || seen.has(view.id)) return false;
    seen.add(view.id);
    return true;
  });
  return valid.length > 0 ? valid : [DEFAULT_THREAD_VIEW];
}
