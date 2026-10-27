import { expect, type Page } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import type { SidebarTaskQuery } from "../../../lib/types/http";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

export const SIDEBAR_QUERY_PATH = /\/api\/v1\/workspaces\/[^/]+\/sidebar\/query$/;

export async function openPhoneList(page: Page) {
  await page.setViewportSize({ width: 393, height: 852 });
  const mobile = new MobileKanbanPage(page);
  await mobile.goto();
  await mobile.viewOptionsButton.tap();
  const options = page.getByRole("dialog", { name: "View options" });
  await options.getByRole("radio", { name: "List", exact: true }).tap();
  await expect(page.getByTestId("sidebar-view-chip-row")).toBeVisible();
}

export function waitForSidebarQuery(page: Page, predicate: (query: SidebarTaskQuery) => boolean) {
  return waitForHttp(page, "POST", SIDEBAR_QUERY_PATH, {
    predicate: (response) => {
      try {
        return predicate(response.request().postDataJSON() as SidebarTaskQuery);
      } catch {
        return false;
      }
    },
  });
}

export async function savePhoneWorkspaceView(
  apiClient: ApiClient,
  workspaceId: string,
  view: Record<string, unknown>,
) {
  const id = String(view.id);
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspaceId,
      views: [
        {
          id: "view-all-tasks",
          name: "All tasks",
          filters: [],
          sort: { key: "state", direction: "asc" },
          group: "none",
          group_indent: true,
          collapsed_groups: [],
        },
        view,
      ],
      active_view_id: id,
      draft: null,
    },
  });
}
