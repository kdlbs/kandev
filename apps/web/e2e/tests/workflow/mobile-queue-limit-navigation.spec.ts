import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { createWipQueueScenario } from "../../helpers/queue-limit-navigation";

test("mobile WIP queue links to workflow settings and updates without reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const scenario = await createWipQueueScenario(apiClient, seedData, "Mobile WIP navigation");

  await testPage.goto(`/t/${scenario.queuedTaskId}`);
  const sessionPage = new SessionPage(testPage);
  await sessionPage.waitForLoad();

  const wipStatus = testPage.getByTestId("task-wip-queue-status");
  await expect(wipStatus).toBeVisible();
  await expect(wipStatus).toContainText("Workflow WIP limit");
  await expect(wipStatus).toContainText("Mobile WIP navigation workflow / Review");
  const settingsLink = wipStatus.getByTestId("wip-queue-settings-link");
  const linkBox = await settingsLink.boundingBox();
  expect(linkBox).not.toBeNull();
  expect(linkBox!.height).toBeGreaterThanOrEqual(44);

  await settingsLink.tap();
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
  await expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
