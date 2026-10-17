import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

const FACET = `facet:${PLUGIN_ID}:fixture-tags`;
const PREFIX = "Facet proof";
type Values = Record<string, Array<{ value: string; label: string; color?: string }>>;

async function seedTasks(
  apiClient: ApiClient,
  seedData: {
    workspaceId: string;
    workflowId: string;
    startStepId: string;
  },
) {
  const create = (name: string) =>
    apiClient.createTask(seedData.workspaceId, `${PREFIX} ${name}`, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
  const [multi, solo, empty] = await Promise.all([
    create("multi"),
    create("solo"),
    create("empty"),
  ]);
  return { multi: multi.id, solo: solo.id, empty: empty.id };
}

async function putValues(apiClient: ApiClient, workspaceId: string, values: Values) {
  const response = await apiClient.rawRequest(
    "PUT",
    `/api/plugins/${PLUGIN_ID}/user-state/workspace/${workspaceId}/facet-values`,
    { value: values, writerId: "e2e-sibling-surface" },
  );
  expect(response.ok, await response.text()).toBe(true);
}

async function showOnlyFixtureTasks(page: Page) {
  await page.getByTestId("kanban-header-search").getByPlaceholder("Search tasks...").fill(PREFIX);
  await expect(page.getByTestId("tasks-list-row-title")).toHaveCount(3);
}

test.describe("Plugin task-list facet", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("sorts, sections, handles a failed task, and re-sections from workspace user state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const ids = await seedTasks(apiClient, seedData);
    await putValues(apiClient, seedData.workspaceId, {
      [ids.multi]: [
        { value: "beta", label: "Beta" },
        { value: "alpha", label: "Alpha", color: "#00aaff" },
      ],
      [ids.solo]: [{ value: "beta", label: "Beta" }],
    });
    await testPage.goto("/tasks?group=none");
    await showOnlyFixtureTasks(testPage);

    await testPage.getByTestId("tasks-list-sort").click();
    const sortLabels = await testPage.getByRole("listbox").getByRole("option").allTextContents();
    expect(sortLabels.at(-1)).toBe("Fixture tag");
    expect(sortLabels.indexOf("Title A-Z")).toBeLessThan(sortLabels.length - 1);
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture tag" }).click();
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText([
      `${PREFIX} multi`,
      `${PREFIX} solo`,
      `${PREFIX} empty`,
    ]);

    await testPage.getByTestId("tasks-list-group").click();
    const groupLabels = await testPage.getByRole("listbox").getByRole("option").allTextContents();
    expect(groupLabels.at(-1)).toBe("Fixture tag");
    expect(groupLabels.indexOf("Workflow step")).toBeLessThan(groupLabels.length - 1);
    await testPage.getByRole("listbox").getByRole("option", { name: "Fixture tag" }).click();
    await expect(testPage).toHaveURL((url) => url.searchParams.get("group") === FACET);
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.tasks_list_group)
      .toBe(FACET);
    const sections = testPage.getByTestId("tasks-list-section");
    await expect(sections).toHaveCount(3);
    await expect(sections.nth(0)).toContainText("Alpha");
    await expect(sections.nth(0).getByTestId("tasks-list-row-title")).toHaveText(`${PREFIX} multi`);
    await expect(sections.nth(0).locator('span[style*="background-color"]')).toHaveCount(1);
    await expect(sections.nth(1).getByTestId("tasks-list-row-title")).toHaveText([
      `${PREFIX} multi`,
      `${PREFIX} solo`,
    ]);
    await expect(sections.nth(2)).toContainText("Unassigned");
    await expect(sections.nth(2).getByTestId("tasks-list-row-title")).toHaveText(`${PREFIX} empty`);

    await testPage.evaluate((taskId) => {
      const win = window as typeof window & {
        __e2eFacetValues?: { __throwFor: string };
        __e2eFacetNotify?: () => void;
      };
      win.__e2eFacetValues = { __throwFor: taskId };
      win.__e2eFacetNotify?.();
    }, ids.solo);
    await expect(sections.nth(1).getByTestId("tasks-list-row-title")).toHaveText(`${PREFIX} multi`);
    await expect(sections.nth(2).getByTestId("tasks-list-row-title")).toHaveCount(2);
    await expect(sections.nth(2)).toContainText(`${PREFIX} solo`);
    await expect(sections.nth(2)).toContainText(`${PREFIX} empty`);

    await testPage.evaluate(() => {
      const win = window as typeof window & {
        __e2eFacetValues?: { __throwFor: string };
        __e2eFacetNotify?: () => void;
      };
      delete win.__e2eFacetValues;
      win.__e2eFacetNotify?.();
    });
    await putValues(apiClient, seedData.workspaceId, {
      [ids.multi]: [{ value: "gamma", label: "Gamma" }],
      [ids.solo]: [{ value: "alpha", label: "Alpha" }],
    });
    await expect(sections).toHaveCount(3);
    await expect(sections.nth(0).getByTestId("tasks-list-row-title")).toHaveText(`${PREFIX} solo`);
    await expect(sections.nth(1)).toContainText("Gamma");
    await expect(sections.nth(1).getByTestId("tasks-list-row-title")).toHaveText(`${PREFIX} multi`);
    await expect(sections.nth(2).getByTestId("tasks-list-row-title")).toHaveText(`${PREFIX} empty`);
  });

  test("restores the facet deep link without leaking it to the API or rewriting a missing plugin", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const ids = await seedTasks(apiClient, seedData);
    await putValues(apiClient, seedData.workspaceId, {
      [ids.multi]: [{ value: "alpha", label: "Alpha" }],
      [ids.solo]: [{ value: "beta", label: "Beta" }],
    });
    await testPage.goto(`/tasks?sort=${FACET}&group=${FACET}`);
    await expect(testPage.getByTestId("tasks-list-sort")).toContainText("Fixture tag");
    await expect(testPage.getByTestId("tasks-list-group")).toContainText("Fixture tag");
    await expect(testPage.getByTestId("tasks-list-section").first()).toContainText("Alpha");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return [settings.tasks_list_sort, settings.tasks_list_group];
      })
      .toEqual([FACET, FACET]);

    const sorts: string[] = [];
    testPage.on("request", (request) => {
      const url = new URL(request.url());
      if (request.method() === "GET" && /\/api\/v1\/workspaces\/[^/]+\/tasks$/.test(url.pathname)) {
        sorts.push(url.searchParams.get("sort") ?? "");
      }
    });
    await showOnlyFixtureTasks(testPage);
    await expect.poll(() => sorts.length).toBeGreaterThan(0);
    expect(sorts.every((sort) => !sort.trim().startsWith("facet:"))).toBe(true);
    await expect(testPage).toHaveURL(
      (url) => url.searchParams.get("sort") === FACET && url.searchParams.get("group") === FACET,
    );

    const disabled = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`);
    expect(disabled.ok, await disabled.text()).toBe(true);
    await testPage.reload();
    await expect(testPage.getByTestId("tasks-list-sort")).toContainText("Updated newest");
    await expect(testPage.getByTestId("tasks-list-group")).toContainText("Workflow step");
    await expect(testPage.getByTestId("tasks-list-section")).toHaveCount(1);
    await expect(testPage).toHaveURL(
      (url) => url.searchParams.get("sort") === FACET && url.searchParams.get("group") === FACET,
    );
    let settings = (await apiClient.getUserSettings()).settings;
    expect([settings.tasks_list_sort, settings.tasks_list_group]).toEqual([FACET, FACET]);

    await testPage.reload();
    settings = (await apiClient.getUserSettings()).settings;
    expect([settings.tasks_list_sort, settings.tasks_list_group]).toEqual([FACET, FACET]);
    const enabled = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/enable`);
    expect(enabled.ok, await enabled.text()).toBe(true);
    await testPage.reload();
    await expect(testPage.getByTestId("tasks-list-sort")).toContainText("Fixture tag");
    await expect(testPage.getByTestId("tasks-list-group")).toContainText("Fixture tag");
    await expect(testPage.getByTestId("tasks-list-section").first()).toContainText("Alpha");
    settings = (await apiClient.getUserSettings()).settings;
    expect([settings.tasks_list_sort, settings.tasks_list_group]).toEqual([FACET, FACET]);
  });
});
