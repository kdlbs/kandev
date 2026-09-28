import { expect, test } from "../../fixtures/test-base";

test("phone transfer and release", async ({ testPage, apiClient, seedData }) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone management claim", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const seeded = await apiClient.rawRequest(
    "POST",
    `/api/v1/_test/tasks/${task.id}/management-claim`,
    { installation_id: "disabled-manager", instance_key: "phone-manager" },
  );
  expect(seeded.ok).toBe(true);

  await testPage.goto(`/t/${task.id}`);
  const toolbar = testPage.getByTestId("mobile-task-control-toolbar");
  await expect(toolbar).toBeVisible();
  expect((await toolbar.boundingBox())?.height).toBeLessThanOrEqual(60);
  const row = testPage.getByTestId("task-management-claim-row");
  await expect(row).toBeVisible();
  await expect(row.getByTestId("task-management-claim-owner")).toContainText("disabled-manager");

  const open = row.getByTestId("task-management-claim-open");
  await expect(open).toBeVisible();
  expect((await open.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await open.tap();

  const surface = testPage.getByTestId("task-management-claim-surface");
  await expect(surface).toBeVisible();
  const reason = surface.getByTestId("task-management-claim-reason");
  await reason.fill("Take ownership on phone");
  const transfer = surface.getByTestId("task-management-claim-transfer");
  expect((await transfer.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await transfer.tap();
  await expect(surface.getByTestId("task-management-claim-owner")).toContainText("Human");

  await reason.fill("Return management to plugins");
  const release = surface.getByTestId("task-management-claim-release");
  expect((await release.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await release.tap();
  await expect(row.getByTestId("task-management-claim-owner")).toContainText("None");

  const dimensions = await testPage.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
});
