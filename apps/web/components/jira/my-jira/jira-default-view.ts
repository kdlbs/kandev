import type { FilterState } from "./filter-model";
import { DEFAULT_VIEW, type SavedView } from "./use-saved-views";

export type InitialJiraView = {
  filters: FilterState;
  activeViewId: string;
  customJql: string | null;
  showJqlEditor: boolean;
};

export function initialFilters(defaultProjectKey: string): FilterState {
  const key = defaultProjectKey.trim();
  if (!key) return DEFAULT_VIEW.filters;
  return { ...DEFAULT_VIEW.filters, projectKeys: [key] };
}

export function resolveInitialJiraView(
  defaultViewId: string,
  views: SavedView[],
  defaultProjectKey: string,
): InitialJiraView {
  const view = views.find((candidate) => candidate.id === defaultViewId.trim());
  if (!view) {
    return {
      filters: initialFilters(defaultProjectKey),
      activeViewId: DEFAULT_VIEW.id,
      customJql: null,
      showJqlEditor: false,
    };
  }

  const customJql = view.customJql ?? null;
  return {
    filters: view.filters,
    activeViewId: view.id,
    customJql,
    showJqlEditor: customJql !== null,
  };
}
