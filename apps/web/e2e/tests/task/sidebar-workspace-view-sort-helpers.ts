import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

const SORT_CHAIN = {
  key: "running",
  direction: "desc",
  then_by: [
    { key: "color", color: "red", direction: "desc" },
    { key: "lastActivityAt", direction: "desc" },
  ],
};

export async function setWorkspaceViewSortAndIndent(
  api: ApiClient,
  workspaceId: string,
  viewName: string,
): Promise<void> {
  const { settings } = await api.getUserSettings();
  const state = settings.sidebar_views_by_workspace[workspaceId];
  const target = state?.views.find((view) => view.name === viewName);
  if (!state || !target) throw new Error(`Sidebar view ${viewName} was not saved`);

  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspaceId,
      views: state.views.map((view) =>
        view.id === target.id ? { ...view, sort: SORT_CHAIN, group_indent: false } : view,
      ),
      active_view_id: state.active_view_id,
      draft: state.draft,
    },
  });
}

export function expectWorkspaceViewSortAndIndent(view: Record<string, unknown> | undefined) {
  expect(view?.sort).toEqual(SORT_CHAIN);
  expect(view?.group_indent).toBe(false);
}

export function expectDefaultWorkspaceViewSortAndIndent(view: Record<string, unknown> | undefined) {
  expect(view?.group_indent).toBe(true);
  expect(view?.sort).not.toHaveProperty("then_by");
}
