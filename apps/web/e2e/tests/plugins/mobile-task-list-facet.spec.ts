// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { openPhoneList, savePhoneWorkspaceView } from "../task/mobile-list-helpers";
import {
  readPreviousSidebarViewState,
  restoreSidebarViewState,
} from "../task/sidebar-running-first-activity-sort-helpers";

test.describe("Mobile plugin task-list facet", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("shows workspace user-state values in List view options and groups matching tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const token = `Mobile facet ${Date.now()}`;
    const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
    await installFixturePlugin(testPage);
    const create = (title: string) =>
      apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
    const [multi, single, empty] = await Promise.all([
      create(`${token} multi`),
      create(`${token} single`),
      create(`${token} empty`),
    ]);
    await apiClient.rawRequest(
      "PUT",
      `/api/plugins/${PLUGIN_ID}/user-state/workspace/${seedData.workspaceId}/facet-values`,
      {
        value: {
          [multi.id]: [
            { value: "alpha", label: "Alpha" },
            { value: "beta", label: "Beta" },
          ],
          [single.id]: [{ value: "beta", label: "Beta" }],
        },
        writerId: "e2e-sibling-surface",
      },
    );
    await savePhoneWorkspaceView(apiClient, seedData.workspaceId, {
      id: `${token}-view`,
      name: `${token} view`,
      filters: [{ id: `${token}-filter`, dimension: "titleMatch", op: "matches", value: token }],
      sort: { key: "title", direction: "asc" },
      group: "none",
      group_indent: true,
      collapsed_groups: [],
    });

    try {
      await openPhoneList(testPage);
      await testPage.getByTestId("mobile-topbar-page-context").tap();
      const menu = testPage.getByRole("dialog", { name: "View options" });
      const editorScroll = menu.getByTestId("mobile-list-view-editor-scroll");
      const additional = menu.getByTestId("mobile-list-additional-options-toggle");
      await additional.scrollIntoViewIfNeeded();
      await additional.tap();

      const sortSelect = menu.getByTestId("mobile-list-facet-sort");
      await sortSelect.scrollIntoViewIfNeeded();
      await sortSelect.tap();
      const sortOption = testPage.getByRole("option", { name: "Fixture tag", exact: true });
      await expect(sortOption).toBeVisible();
      await sortOption.tap();
      await expect(sortSelect).toContainText("Fixture tag");

      const groupSelect = menu.getByTestId("mobile-list-facet-group");
      await groupSelect.scrollIntoViewIfNeeded();
      await groupSelect.tap();
      await testPage.getByRole("option", { name: "Fixture tag", exact: true }).tap();
      await expect(groupSelect).toContainText("Fixture tag");
      await expect(editorScroll.locator(".overflow-y-auto")).toHaveCount(0);

      await testPage.keyboard.press("Escape");
      const list = testPage.getByTestId("mobile-tasks-list-scroll");
      const groups = list.getByTestId("sidebar-group");
      await expect(groups).toHaveCount(3);
      await expect(groups.nth(0).getByTestId("sidebar-group-header")).toContainText("Alpha");
      await expect(
        groups.nth(0).getByTestId("sidebar-task-item").filter({ hasText: multi.title }),
      ).toBeVisible();
      await expect(groups.nth(1).getByTestId("sidebar-group-header")).toContainText("Beta");
      await expect(
        groups.nth(1).getByTestId("sidebar-task-item").filter({ hasText: multi.title }),
      ).toBeVisible();
      await expect(
        groups.nth(1).getByTestId("sidebar-task-item").filter({ hasText: single.title }),
      ).toBeVisible();
      await expect(groups.nth(2).getByTestId("sidebar-group-header")).toContainText("Unassigned");
      await expect(
        groups.nth(2).getByTestId("sidebar-task-item").filter({ hasText: empty.title }),
      ).toBeVisible();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
    }
  });
});
