import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { createWipQueueScenario } from "../../helpers/queue-limit-navigation";
import {
  createQueuedSessionOwnershipScenario,
  waitForLaunchQueue,
} from "./queued-session-ownership-helpers";

test.describe("Queue limit navigation", () => {
  test("labels the global capacity queue and returns without changing the task", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const releaseCeiling = await backend.useEnv({ KANDEV_MAX_CONCURRENT_SESSIONS: "1" });
    let fillerSessionId: string | undefined;

    try {
      const scenario = await createQueuedSessionOwnershipScenario(
        apiClient,
        seedData,
        "Global queue navigation",
      );
      fillerSessionId = scenario.fillerSessionId;

      await testPage.goto(`/t/${scenario.taskId}`);
      await new SessionPage(testPage).waitForLoad();
      const queueStatus = testPage.locator('[data-testid="task-launch-queue-status"]:visible');
      await expect(queueStatus).toContainText("Global session limit");
      await expect(queueStatus).toContainText("All workspaces");

      const settingsLink = queueStatus.getByTestId("launch-queue-session-capacity-link");
      await expect(settingsLink).toHaveAttribute(
        "href",
        "/settings/preferences/task-behavior#setting-session-capacity",
      );
      await settingsLink.click();
      await expect(testPage).toHaveURL(
        /\/settings\/preferences\/task-behavior#setting-session-capacity$/,
      );
      await expect(
        testPage.locator('[data-settings-target="setting-session-capacity"]'),
      ).toHaveAttribute("data-settings-target-highlight", "true");

      await testPage.goBack();
      await new SessionPage(testPage).waitForLoad();
      await expect(
        testPage.locator('[data-testid="task-launch-queue-status"]:visible'),
      ).toBeVisible();
      await expect(
        waitForLaunchQueue(apiClient, seedData.workspaceId, scenario.taskId, 5_000),
      ).resolves.toMatchObject({ reason: "session_capacity" });
    } finally {
      if (fillerSessionId) {
        await apiClient
          .stopSession({
            session_id: fillerSessionId,
            reason: "queue-limit-navigation e2e cleanup",
            force: true,
          })
          .catch(() => undefined);
      }
      await releaseCeiling();
    }
  });

  test("links a WIP queue to its workflow and promotes after a saved limit change", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const scenario = await createWipQueueScenario(apiClient, seedData, "WIP queue navigation");
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(scenario.queuedTaskId)).toBeVisible();
    await kanban.taskCard(scenario.queuedTaskId).click();

    const sessionPage = new SessionPage(testPage);
    await sessionPage.waitForLoad();
    const wipStatus = testPage.getByTestId("task-wip-queue-status");
    await expect(wipStatus).toBeVisible();
    await expect(wipStatus).toContainText("Workflow WIP limit");
    await expect(wipStatus).toContainText("WIP queue navigation workflow / Review");
    await expect(wipStatus).toContainText("1 of 1 task admitted");

    const settingsLink = wipStatus.getByTestId("wip-queue-settings-link");
    await expect(settingsLink).toHaveAttribute(
      "href",
      `/settings/workspaces/${seedData.workspaceId}/workflows#workflow-card-${scenario.workflowId}`,
    );
    await settingsLink.click();
    await expect(testPage).toHaveURL(
      new RegExp(
        `/settings/workspaces/${seedData.workspaceId}/workflows#workflow-card-${scenario.workflowId}$`,
      ),
    );
    await expect(testPage.getByTestId(`workflow-card-${scenario.workflowId}`)).toHaveAttribute(
      "data-settings-target-highlight",
      "true",
    );

    await testPage.goBack();
    await sessionPage.waitForLoad();
    await expect(wipStatus).toBeVisible();

    await apiClient.updateWorkflowStep(scenario.reviewStepId, { wip_limit: 2 });
    await expect
      .poll(async () => (await apiClient.getTask(scenario.queuedTaskId)).wip_admitted)
      .toBe(true);
    await expect(wipStatus).toHaveCount(0);
    await expect(apiClient.getTask(scenario.admittedTaskId)).resolves.toMatchObject({
      wip_admitted: true,
    });
  });
});
