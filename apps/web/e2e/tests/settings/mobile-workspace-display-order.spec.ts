import { test, expect } from "../../fixtures/test-base";
import { setSettingsMenuMode } from "../../helpers/settings-menu";

test.describe("Mobile workspace display order", () => {
  test("places the active workspace first in the phone Settings tree", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("Mobile Non-active Workspace");

    try {
      await testPage.setViewportSize({ width: 390, height: 844 });
      await setSettingsMenuMode(testPage, "accordion");
      await testPage.goto("/settings");

      const { workspaces } = await apiClient.listWorkspaces();
      const expectedWorkspaceIds = [
        seedData.workspaceId,
        ...workspaces
          .filter((workspace) => workspace.id !== seedData.workspaceId)
          .map((workspace) => workspace.id),
      ];
      const index = testPage.getByTestId("settings-index");
      await expect(index).toBeVisible();
      await index.getByRole("button", { name: /Expand Workspaces/ }).tap();

      const workspaceRows = index.locator('[data-record="true"]');
      await expect(workspaceRows).toHaveCount(expectedWorkspaceIds.length);
      const sidebarHrefs = await workspaceRows
        .locator("a")
        .evaluateAll((links) =>
          links.map(
            (link) => new URL(link.getAttribute("href") ?? "", "http://kandev.test").pathname,
          ),
        );
      expect(sidebarHrefs).toEqual(expectedWorkspaceIds.map((id) => `/settings/workspaces/${id}`));
      await expect(workspaceRows.first()).toContainText("Active");

      const documentWidth = await testPage.evaluate(() => ({
        clientWidth: document.documentElement.clientWidth,
        scrollWidth: document.documentElement.scrollWidth,
      }));
      expect(documentWidth.scrollWidth).toBeLessThanOrEqual(documentWidth.clientWidth);
    } finally {
      await apiClient.deleteWorkspace(other.id, other.name);
    }
  });
});
