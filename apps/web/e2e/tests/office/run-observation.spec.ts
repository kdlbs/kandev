import { expect, test } from "../../fixtures/office-fixture";

test.describe("Office run observation", () => {
  test("direct run entry keeps the named agent and activity label", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      reason: "routine_dispatch_manual",
      inputSnapshot: JSON.stringify({ adapter: "mock", model: "mock-fast" }),
    });
    await apiClient.seedActivity({
      workspaceId: officeSeed.workspaceId,
      actorType: "agent",
      actorId: officeSeed.agentId,
      action: "task_status_changed",
      targetType: "task",
      targetId: "missing-task-id",
      details: JSON.stringify({ task_identifier: "KAN-14" }),
      runId: run.run_id,
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${run.run_id}`);
    await expect(testPage.getByTestId("run-header")).toBeVisible();
    await expect(testPage.getByTestId("run-agent-name")).toHaveText("CEO");

    await testPage.goto("/office/workspace/activity");
    await expect(testPage.getByText("CEO", { exact: true })).toBeVisible();
    await expect(testPage.getByText(/KAN-14/)).toBeVisible();

    const activity = await officeApi.listActivity(officeSeed.workspaceId);
    expect(activity).toBeDefined();
  });
});
