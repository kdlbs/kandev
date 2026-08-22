import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

test.describe("Plugin task-list facet", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("adds a facet to desktop Sort and Group controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await installFixturePlugin(testPage);
    await apiClient.createTask(seedData.workspaceId, "Desktop facet task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/tasks?group=none");
    await testPage.waitForLoadState("networkidle");

    await testPage.getByTestId("tasks-list-sort").click();
    await expect(
      testPage.getByRole("listbox").getByRole("option", { name: "Fixture facet" }),
    ).toBeVisible();
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture facet" }).click();

    await testPage.getByTestId("tasks-list-group").click();
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture facet" }).click();

    const section = testPage.getByTestId("tasks-list-section");
    await expect(section).toHaveCount(1);
    await expect(section).toContainText("Fixture facet");
    await expect(section).toContainText("Desktop facet task");
  });
});
