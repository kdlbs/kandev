import { test, expect } from "../../fixtures/test-base";

/**
 * The workspace settings header is the same switcher the sidebar draws: the
 * active workspace is marked by the "Active" pill after its name, next to the
 * type badge, and picking another workspace opens that workspace's copy of the
 * tab you are already on rather than sending you back to the list.
 */
test.describe("Workspace settings switcher", () => {
  test("marks the active workspace and keeps the tab when switching", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("Switcher Target Workspace");

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/secrets`);

    const trigger = testPage.getByTestId("workspace-settings-switcher");
    await expect(trigger).toContainText("E2E Workspace");
    // The seeded workspace is the active one, so its page carries the badge
    // beside the closed picker.
    await expect(testPage.getByTestId("workspace-settings-active-badge")).toBeVisible();
    await trigger.click();

    // The seeded workspace is the active one, so it carries the badge; the
    // freshly created workspace does not.
    const activeRow = testPage.getByTestId(
      `workspace-settings-switcher-item-${seedData.workspaceId}`,
    );
    const targetRow = testPage.getByTestId(`workspace-settings-switcher-item-${other.id}`);
    await expect(activeRow).toBeVisible();
    await expect(activeRow).toContainText("Active");
    await expect(targetRow).not.toContainText("Active");

    const activeText = (await activeRow.innerText()).replace(/\s+/g, " ");
    expect(activeText.indexOf("Active")).toBeGreaterThan(activeText.indexOf("E2E Workspace"));
    expect(activeText.indexOf("Active")).toBeLessThan(activeText.indexOf("Kanban"));

    await targetRow.click();

    await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${other.id}/secrets$`));
    await expect(testPage.getByTestId("workspace-settings-switcher")).toContainText(
      "Switcher Target Workspace",
    );
    // The freshly created workspace is not the active one, so its page shows
    // no header badge.
    await expect(testPage.getByTestId("workspace-settings-active-badge")).toHaveCount(0);
  });

  test("switching settings pages does not change the active workspace", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("Switcher Passive Workspace");

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
    await testPage.getByTestId("workspace-settings-switcher").click();
    await testPage.getByTestId(`workspace-settings-switcher-item-${other.id}`).click();
    await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${other.id}$`));

    // Reopened on the other workspace's page, the badge still points at the
    // seeded workspace: navigating here never activated anything.
    await testPage.getByTestId("workspace-settings-switcher").click();
    await expect(
      testPage.getByTestId(`workspace-settings-switcher-item-${seedData.workspaceId}`),
    ).toContainText("Active");
    await expect(
      testPage.getByTestId(`workspace-settings-switcher-item-${other.id}`),
    ).not.toContainText("Active");
  });
});
