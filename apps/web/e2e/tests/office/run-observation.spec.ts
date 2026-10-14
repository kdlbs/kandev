import { expect, test } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Office run observation", () => {
  test("direct run entry keeps the named agent and activity label", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
    seedData,
  }) => {
    const agentName = "Run Observation Agent";
    const createdAgent = await officeApi.createAgent(officeSeed.workspaceId, {
      name: agentName,
      role: "worker",
      agent_profile_id: seedData.agentProfileId,
    });
    const agentId = String(createdAgent.id);
    const run = await apiClient.seedRun({
      agentProfileId: agentId,
      status: "finished",
      reason: "routine_dispatch_manual",
      inputSnapshot: JSON.stringify({ adapter: "mock", model: "mock-fast" }),
    });
    await apiClient.seedActivity({
      workspaceId: officeSeed.workspaceId,
      actorType: "agent",
      actorId: agentId,
      action: "task_status_changed",
      targetType: "task",
      targetId: "missing-task-id",
      details: JSON.stringify({ task_identifier: "KAN-14" }),
      runId: run.run_id,
    });

    const { runs } = (await officeApi.listRuns(officeSeed.workspaceId)) as {
      runs: Array<Record<string, unknown>>;
    };
    expect(Array.isArray(runs)).toBe(true);
    expect(runs).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: run.run_id,
          agent_profile_id: agentId,
          status: "finished",
          requested_at: expect.any(String),
        }),
      ]),
    );

    await testPage.goto(`/office/agents/${agentId}/runs/${run.run_id}`);
    await expect(testPage.getByTestId("run-header")).toBeVisible();
    await expect(testPage.getByTestId("run-agent-name")).toHaveText(agentName);

    await testPage.goto("/office/workspace/activity");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Activity/i, { timeout: 10_000 });
    const taskActivity = testPage.getByText(/KAN-14/);
    await expect(taskActivity).toBeVisible();
    await expect(
      taskActivity.locator("xpath=../..").getByText(agentName, { exact: true }),
    ).toBeVisible();

    const activity = await officeApi.listActivity(officeSeed.workspaceId);
    expect(activity).toBeDefined();
  });
});
