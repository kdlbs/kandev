/**
 * E2E: the `task-card-tags` slot (docs/plans/plugins/PLUGIN-API.md), added
 * alongside `task-card-indicators` so a Kanban-card contribution too wide for
 * the cramped title-row spot (e.g. a row of tag chips) gets its own row.
 *
 * Uses the same real `plugin-fixture` gRPC plugin package as
 * `plugin-task-panel.spec.ts` — see that file's header for how
 * apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz is built
 * (`make -C apps/backend e2e-plugin-package`). The fixture's `ui/bundle.js`
 * registers a `task-card-tags` slot component alongside `task-card-indicators`
 * — see apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";

async function uninstallViaApi(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

test.describe("Plugins — task-card-tags slot", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallViaApi(apiClient);
  });

  test("task-card-tags slot renders the plugin's component on the kanban card (AC2)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await installFixturePlugin(testPage);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Plugin card tags task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/");
    const kanban = new KanbanPage(testPage);
    await expect(kanban.board).toBeVisible({ timeout: 15_000 });

    const tags = kanban.taskCard(seedTask.id).getByTestId("e2e-card-tags");
    await expect(tags).toBeVisible({ timeout: 15_000 });
    await expect(tags).toHaveAttribute("data-task-id", seedTask.id);
  });

  test("no task-card-tags markup renders when the plugin isn't installed (AC3)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "No plugin card tags task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/");
    const kanban = new KanbanPage(testPage);
    await expect(kanban.board).toBeVisible({ timeout: 15_000 });

    await expect(kanban.taskCard(seedTask.id).getByTestId("e2e-card-tags")).toHaveCount(0);
  });

  test("disabling the plugin removes its task-card-tags component from the card without a reload (AC5)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await installFixturePlugin(testPage);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Disable card tags task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/");
    const kanban = new KanbanPage(testPage);
    await expect(kanban.board).toBeVisible({ timeout: 15_000 });

    const tags = kanban.taskCard(seedTask.id).getByTestId("e2e-card-tags");
    await expect(tags).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible({ timeout: 10_000 });

    await testPage.goto("/");
    await expect(kanban.board).toBeVisible({ timeout: 15_000 });
    await expect(kanban.taskCard(seedTask.id).getByTestId("e2e-card-tags")).toHaveCount(0);
  });
});
