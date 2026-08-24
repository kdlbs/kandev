// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

test.describe("Mobile plugin task-list facet", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("adds a facet to the mobile Group picker", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);

    await installFixturePlugin(testPage);
    const taskId = (
      await apiClient.createTask(seedData.workspaceId, "Mobile facet task", {
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

    await testPage.getByRole("button", { name: "Open menu" }).tap();
    const menu = testPage.getByRole("dialog", { name: "Menu" });
    await menu.getByTestId("mobile-tasks-list-group").tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture tag" }).tap();

    const section = testPage.getByTestId("tasks-list-section");
    await expect(section).toHaveCount(1);
    await expect(section).toContainText("Alpha");
    await expect(section).toContainText("Mobile facet task");
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
});
