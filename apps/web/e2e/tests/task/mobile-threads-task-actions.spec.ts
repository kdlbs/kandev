import { test, expect } from "../../fixtures/test-base";
import {
  allTaskActionOutcomes,
  seedActionThreads,
  seedLinkedIssue,
  ThreadActionsPage,
  withTaskActionSettings,
} from "./threads-task-actions-helpers";
import { swipeDeckLeft } from "./mobile-threads-swipe-helpers";
import {
  failedActionOutcomes,
  filteredArchiveOutcome,
  lateArchiveOutcome,
} from "./threads-task-actions-edge-helpers";

for (const [name, outcome] of [
  ["failures and retry", failedActionOutcomes],
  ["late archive retains a newer menu", lateArchiveOutcome],
  ["filtered opener retains archive identity", filteredArchiveOutcome],
] as const) {
  test(name, async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 360, height: 780 });
    await withTaskActionSettings(apiClient, () => outcome(testPage, apiClient, seedData, true));
  });
}

test("completes all six task actions by touch and cancels destructive choices", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(240_000);
  await testPage.setViewportSize({ width: 360, height: 780 });
  await seedLinkedIssue(apiClient);
  try {
    await allTaskActionOutcomes(testPage, apiClient, seedData, true);
  } finally {
    await apiClient.saveUserSettings({ confirm_task_archive: true });
  }
});

// @covers AC-TASKS-THREADS-ACTIONS-004.2 through AC-TASKS-THREADS-ACTIONS-004.7
test("contains nested choices with long labels and preserves native swiping after dismissal", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(240_000);
  const { a, destination } = await seedActionThreads(apiClient, seedData);
  const destinationName = "Workflow".repeat(12);
  await apiClient.updateWorkflow(destination.id, { name: destinationName });
  const longName = "A deliberately long workflow step " + "unbroken".repeat(24);
  for (let index = 0; index < 18; index++)
    await apiClient.createWorkflowStep(destination.id, `${index} ${longName}`, index + 1);
  await apiClient.updateTaskTitle(a.id, "Task".repeat(15));
  await testPage.setViewportSize({ width: 360, height: 780 });
  await testPage.goto(`/threads?workspace=${seedData.workspaceId}`);
  const ui = new ThreadActionsPage(testPage, true);
  const firstId = await testPage
    .getByTestId("threads-board")
    .locator("[data-thread-column-id]")
    .first()
    .getAttribute("data-thread-column-id");
  if (!firstId) throw new Error("No first thread");
  await expect(ui.trigger(firstId)).toBeVisible();
  await ui.expectHeaderActionAlignment(firstId);
  await testPage.screenshot({ path: testInfo.outputPath("phone-header-360.png") });
  await ui.open(firstId);
  const drawer = testPage.getByTestId("task-management-drawer");
  await ui.contained(drawer);
  await expect(ui.choice("Close")).toHaveCSS("cursor", "pointer");
  await testPage.screenshot({ path: testInfo.outputPath("phone-root-360.png") });
  await ui.nested("Send to workflow");
  await expect(ui.choice("Back")).toHaveCSS("cursor", "pointer");
  await ui.nested(destinationName);
  await ui.contained(drawer);
  await expect(testPage.getByRole("dialog")).toHaveCount(1);
  const scroll = testPage.getByTestId("task-management-scroll");
  expect(await scroll.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  expect(
    await drawer.evaluate(
      (element) =>
        [element, ...element.querySelectorAll("*")].filter(
          (node) =>
            /auto|scroll/.test(getComputedStyle(node).overflowY) &&
            node.scrollHeight > node.clientHeight,
        ).length,
    ),
  ).toBe(1);
  await ui.choice(`17 ${longName}`).scrollIntoViewIfNeeded();
  await expect(ui.choice(`17 ${longName}`)).toBeVisible();
  await testPage.screenshot({ path: testInfo.outputPath("phone-deep-360.png") });
  await ui.press(ui.choice("Back"));
  await expect(ui.choice(destinationName)).toBeFocused();
  await testPage.keyboard.press("Escape");
  await expect(ui.choice("Send to workflow")).toBeFocused();
  await ui.pick("Delete");
  await ui.contained(testPage.getByRole("alertdialog"));
  await testPage.screenshot({ path: testInfo.outputPath("phone-delete-360.png") });
  await ui.press(testPage.getByRole("button", { name: "Cancel", exact: true }));
  await expect(ui.trigger(firstId)).toBeFocused();
  for (const width of [320, 700, 820]) {
    await testPage.setViewportSize({ width, height: 640 });
    await ui.trigger(firstId).scrollIntoViewIfNeeded();
    await ui.expectHeaderActionAlignment(firstId, width < 640);
    const hitTarget = await ui.trigger(firstId).evaluate((button) => {
      const rect = button.getBoundingClientRect();
      return {
        width: rect.width,
        height: rect.height,
        hit: button.contains(
          document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
        ),
      };
    });
    expect(hitTarget).toMatchObject({ width: 44, height: 44, hit: true });
    await ui.open(firstId);
    await ui.contained(drawer);
    const rows = await drawer
      .getByRole("button")
      .evaluateAll((buttons) => buttons.map((button) => button.getBoundingClientRect().height));
    expect(rows.every((height) => height >= 44)).toBe(true);
    if (width === 320)
      await testPage.screenshot({ path: testInfo.outputPath("phone-root-320.png") });
    await ui.pick("Close");
  }
  await ui.open(firstId);
  await testPage.touchscreen.tap(4, 4);
  await expect(drawer).toHaveCount(0);
  await expect(ui.trigger(firstId)).toBeFocused();
  await testPage.setViewportSize({ width: 360, height: 780 });
  await swipeDeckLeft(testPage, async () => {
    await expect(testPage.getByTestId("thread-swipe-cue")).toHaveText("2/2");
  });
  await expect(testPage.getByTestId("thread-swipe-cue")).toHaveText("2/2");
});
