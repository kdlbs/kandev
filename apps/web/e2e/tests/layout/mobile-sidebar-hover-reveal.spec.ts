import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// @covers AC-UI-SIDEBAR-HOVER-001.5
test("phone navigation stays available by tap with no hover rail", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const first = await apiClient.createTask(seedData.workspaceId, "Hover mobile origin", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const second = await apiClient.createTask(seedData.workspaceId, "Hover mobile destination", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${first.id}`);
  await new SessionPage(testPage).waitForLoad();
  await expect(testPage.getByTestId("app-sidebar")).toBeHidden();
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await expect(drawer).toBeVisible();
  await drawer.evaluate(async (element) => {
    await Promise.all(
      element
        .getAnimations({ subtree: true })
        .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().iterations))
        .map((animation) => animation.finished.catch(() => undefined)),
    );
  });
  const bounds = await drawer.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
  await testPage.screenshot({ path: testInfo.outputPath("mobile-navigation.png") });
  await drawer.getByText("Hover mobile destination", { exact: true }).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${second.id}`));
  await expect
    .poll(() =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      ),
    )
    .toBe(0);
});
