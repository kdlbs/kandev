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
    const taskId = (
      await apiClient.createTask(seedData.workspaceId, "Desktop facet task", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      })
    ).id;

    await testPage.goto("/tasks?group=none");
    await testPage.waitForLoadState("networkidle");

    await testPage.evaluate(
      ({ taskId }: { taskId: string }) => {
        const win = window as Record<string, unknown>;
        win.__e2eFacetValues = { [taskId]: [{ value: "alpha", label: "Alpha" }] };
        (win.__e2eFacetNotify as () => void)();
      },
      { taskId },
    );

    await testPage.getByTestId("tasks-list-sort").click();
    await expect(
      testPage.getByRole("listbox").getByRole("option", { name: "Fixture tag" }),
    ).toBeVisible();
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture tag" }).click();

    await testPage.getByTestId("tasks-list-group").click();
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture tag" }).click();

    const section = testPage.getByTestId("tasks-list-section");
    await expect(section).toHaveCount(1);
    await expect(section).toContainText("Alpha");
    await expect(section).toContainText("Desktop facet task");
  });
});
