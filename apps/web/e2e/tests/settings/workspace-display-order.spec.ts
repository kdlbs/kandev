import { test, expect } from "../../fixtures/test-base";
import { setSettingsMenuMode } from "../../helpers/settings-menu";

test.describe("Workspace display order", () => {
  test("places the active workspace first on the page and sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("Non-active Workspace");

    try {
      await testPage.goto("/settings/workspaces");

      const { workspaces } = await apiClient.listWorkspaces();
      const expectedWorkspaceIds = [
        seedData.workspaceId,
        ...workspaces
          .filter((workspace) => workspace.id !== seedData.workspaceId)
          .map((workspace) => workspace.id),
      ];
      const cards = testPage.getByTestId("workspace-list-item");
      await expect(cards).toHaveCount(expectedWorkspaceIds.length);
      const cardHrefs = await cards
        .locator('[data-testid="workspace-overview-link"]')
        .evaluateAll((links) =>
          links.map(
            (link) => new URL(link.getAttribute("href") ?? "", "http://kandev.test").pathname,
          ),
        );
      expect(cardHrefs).toEqual(expectedWorkspaceIds.map((id) => `/settings/workspaces/${id}`));
      await expect(cards.first()).toContainText("Active");

      await setSettingsMenuMode(testPage, "accordion");
      const takeover = testPage.getByTestId("app-sidebar-settings-mode");
      await expect(takeover.getByRole("button", { name: /Expand Workspaces/ })).toBeVisible();
      await takeover.getByRole("button", { name: /Expand Workspaces/ }).click();

      const workspaceRows = takeover.locator('[data-record="true"]');
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
    } finally {
      await apiClient.deleteWorkspace(other.id, other.name);
    }
  });
});
