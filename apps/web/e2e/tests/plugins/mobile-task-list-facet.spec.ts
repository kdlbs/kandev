// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

test.describe("Mobile plugin task-list facet", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("shows workspace user-state values in mobile Sort, Group, and sections", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const create = (title: string) =>
      apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
    const [multi, single] = await Promise.all([
      create("Mobile facet multi"),
      create("Mobile facet single"),
      create("Mobile facet empty"),
    ]);
    const stored = await apiClient.rawRequest(
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
    expect(stored.ok, await stored.text()).toBe(true);
    await testPage.goto("/tasks?group=none");
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const menu = testPage.getByRole("dialog", { name: "View options" });
    const listbox = testPage.getByRole("listbox");
    const sortSelect = menu.getByTestId("mobile-tasks-list-sort");
    await sortSelect.evaluate((element) => element.scrollIntoView({ block: "center" }));
    await sortSelect.tap();
    const sortFacet = listbox.getByRole("option", { name: "Fixture tag" });
    await sortFacet.scrollIntoViewIfNeeded();
    await expect(sortFacet).toBeInViewport();
    await sortFacet.click();
    await expect(sortSelect).toContainText("Fixture tag");
    const groupSelect = menu.getByTestId("mobile-tasks-list-group");
    await groupSelect.evaluate((element) => element.scrollIntoView({ block: "center" }));
    await expect(async () => {
      if (!(await listbox.isVisible())) await groupSelect.tap();
      const facet = listbox.getByRole("option", { name: "Fixture tag" });
      await expect(facet).toBeInViewport({ timeout: 1_000 });
      await facet.click();
      await expect(groupSelect).toContainText("Fixture tag", { timeout: 1_000 });
    }).toPass({ timeout: 10_000 });
    const sections = testPage.getByTestId("tasks-list-section");
    await expect(sections).toHaveCount(3);
    await expect(sections.nth(0)).toContainText("Alpha");
    await expect(sections.nth(0).getByTestId("tasks-list-row-title")).toHaveText(
      "Mobile facet multi",
    );
    await expect(sections.nth(1).getByTestId("tasks-list-row-title")).toHaveText([
      "Mobile facet multi",
      "Mobile facet single",
    ]);
    await expect(sections.nth(2)).toContainText("Unassigned");
    await expect(sections.nth(2).getByTestId("tasks-list-row-title")).toHaveText(
      "Mobile facet empty",
    );
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
});
