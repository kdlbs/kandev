import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
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
import { pendingArchiveRecovery, pendingLastArchive } from "./threads-pending-archive-helpers";

for (const [name, outcome] of [
  ["pending archive recovers without taking focus", pendingArchiveRecovery],
  ["pending last archive stays empty through settlement", pendingLastArchive],
] as const) {
  test(name, async ({ testPage, apiClient, seedData }, testInfo) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 360, height: 780 });
    await withTaskActionSettings(apiClient, () =>
      outcome(testPage, apiClient, seedData, true, testInfo),
    );
  });
}

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
test("contains the mobile change form and preserves native swiping after dismissal", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(240_000);
  const { a, destination } = await seedActionThreads(apiClient, seedData);
  const destinationName = "Workflow".repeat(12);
  await apiClient.updateWorkflow(destination.id, { name: destinationName });
  const longName = "A deliberately long workflow step " + "unbroken".repeat(24);
  let finalStepId = "";
  for (let index = 0; index < 18; index++) {
    const step = await apiClient.createWorkflowStep(
      destination.id,
      `${index} ${longName}`,
      index + 1,
      {
        session_target: { kind: "initial" },
      },
    );
    if (index === 17) finalStepId = step.id;
  }
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
  await ui.pick("Change workflow...");
  const form = new ChangeWorkflowPage(testPage, true);
  await expect(form.phoneDrawer).toBeVisible();
  await ui.contained(form.phoneDrawer);
  await form.chooseWorkflow(destination.id);
  const scroll = form.form.getByTestId("change-workflow-scroll");
  expect(await scroll.evaluate((element) => getComputedStyle(element).overflowY)).toBe("auto");
  expect(
    await form.phoneDrawer.evaluate((element) =>
      [element, ...element.querySelectorAll("*")]
        .filter(
          (node) =>
            /auto|scroll/.test(getComputedStyle(node).overflowY) &&
            node.scrollHeight > node.clientHeight,
        )
        .map((node) => node.getAttribute("data-testid")),
    ),
  ).toEqual(["change-workflow-scroll"]);
  await form.form.getByTestId("change-workflow-step").tap();
  const finalStep = testPage.locator(`[role="option"][data-value="${finalStepId}"]`);
  await finalStep.scrollIntoViewIfNeeded();
  await expect(finalStep).toBeVisible();
  await expect(finalStep.locator("xpath=ancestor::*[@data-slot='popover-content']")).toHaveCSS(
    "opacity",
    "1",
  );
  expect(
    await finalStep.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const topmost = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
      return topmost === element || element.contains(topmost);
    }),
  ).toBe(true);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile change workflow form");
  await testPage.screenshot({ path: testInfo.outputPath("phone-deep-360.png") });
  await testPage.keyboard.press("Escape");
  await ui.press(form.form.getByTestId("change-workflow-cancel"));
  await expect(ui.trigger(firstId)).toBeFocused();
  await ui.open(firstId);
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
