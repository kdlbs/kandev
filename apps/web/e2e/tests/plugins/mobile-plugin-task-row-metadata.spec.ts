import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

async function menuPresentation(apiClient: ApiClient, taskId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/plugins/${PLUGIN_ID}/user-state/task/${taskId}/primary-menu-presentation`,
  );
  if (response.status !== 200) return null;
  return ((await response.json()) as { value: string }).value;
}

test.describe("Mobile plugin task row metadata", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("uses the same metadata and primary action contract in the phone task sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixturePlugin(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile plugin row metadata",
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
    await session.mobileSessionMenu.tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const row = sheet.getByTestId("sidebar-task-item").filter({ hasText: task.title });
    await expect(row.getByTestId("e2e-row-metadata")).toHaveAttribute("data-task-id", task.id);
    await row.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Inspect task metadata" }).tap();

    await expect.poll(() => menuPresentation(apiClient, task.id)).toBe("mobile");
  });
});
