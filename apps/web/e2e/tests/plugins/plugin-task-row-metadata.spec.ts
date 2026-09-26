import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

async function menuPresentation(apiClient: ApiClient, taskId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/plugins/${PLUGIN_ID}/user-state/task/${taskId}/primary-menu-presentation`,
  );
  if (response.status !== 200) return null;
  return ((await response.json()) as { value: string }).value;
}

test.describe("Plugins — task row metadata and primary actions", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("renders generic metadata and invokes the primary action from the desktop sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixturePlugin(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Desktop plugin row metadata",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const row = session.sidebar.getByTestId("sidebar-task-item").filter({ hasText: task.title });
    await expect(row.getByTestId("e2e-row-metadata")).toHaveAttribute("data-task-id", task.id);
    await row.hover();
    await row.getByRole("button", { name: "Task actions" }).click();
    await testPage.getByRole("menuitem", { name: "Inspect task metadata" }).click();

    await expect.poll(() => menuPresentation(apiClient, task.id)).toBe("desktop");
  });
});
