import { test, expect } from "../../fixtures/test-base";
import {
  allTaskActionOutcomes,
  seedActionThreads,
  seedLinkedIssue,
  ThreadActionsPage,
  withTaskActionSettings,
} from "./threads-task-actions-helpers";
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
    await withTaskActionSettings(apiClient, () => outcome(testPage, apiClient, seedData, false));
  });
}

test("contains long desktop workflow submenus and keeps final steps reachable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const { a, destination } = await seedActionThreads(apiClient, seedData);
  for (let index = 0; index < 30; index++)
    await apiClient.createWorkflowStep(
      destination.id,
      `${index} ${"LongStep".repeat(20)}`,
      index + 1,
    );
  await testPage.setViewportSize({ width: 900, height: 500 });
  await testPage.goto(`/threads?workspace=${seedData.workspaceId}`);
  const ui = new ThreadActionsPage(testPage);
  await ui.open(a.id);
  await ui.nested("Send to workflow");
  await ui.nested(destination.name);
  const submenu = testPage
    .getByRole("menu")
    .filter({ has: ui.choice(`29 ${"LongStep".repeat(20)}`) })
    .last();
  await ui.contained(submenu);
  await ui.choice(`29 ${"LongStep".repeat(20)}`).scrollIntoViewIfNeeded();
  await expect(ui.choice(`29 ${"LongStep".repeat(20)}`)).toBeInViewport();
  await testPage.keyboard.press("Escape");
  await expect(ui.trigger(a.id)).toBeFocused();
});

test("completes all six task actions from desktop Threads and cancels destructive choices", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(240_000);
  await seedLinkedIssue(apiClient);
  try {
    await allTaskActionOutcomes(testPage, apiClient, seedData, false);
  } finally {
    await apiClient.saveUserSettings({ confirm_task_archive: true });
  }
});

// @covers AC-TASKS-THREADS-ACTIONS-003.6, AC-TASKS-THREADS-ACTIONS-004.1, AC-TASKS-THREADS-ACTIONS-004.6
test("keeps native chat context, supports header right-click and keyboard dismissal", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const { a } = await seedActionThreads(apiClient, seedData);
  const ui = new ThreadActionsPage(testPage);
  await testPage.goto(`/threads?workspace=${seedData.workspaceId}&taskId=${a.id}`);
  await ui.expectHeaderActionAlignment(a.id);
  await ui.column(a.id).locator("header p").click({ button: "right" });
  await expect(testPage.getByTestId("task-management-menu")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(ui.trigger(a.id)).toBeFocused();
  await ui.trigger(a.id).press("Enter");
  await testPage.getByTestId("task-context-priority").focus();
  await testPage.keyboard.press("ArrowRight");
  await expect(testPage.getByTestId("task-context-priority-low")).toBeVisible();
  await testPage.keyboard.press("ArrowLeft");
  await testPage.keyboard.press("Escape");
  await expect(ui.trigger(a.id)).toBeFocused();
  const editor = ui.column(a.id).locator(".tiptap.ProseMirror");
  await editor.click({ button: "right" });
  await expect(testPage.getByTestId("task-management-menu")).toHaveCount(0);
  await editor.fill("Draft kept after task actions");
  await ui.open(a.id);
  await testPage.keyboard.press("Escape");
  await expect(editor).toHaveText("Draft kept after task actions");
  await ui.open(a.id);
  await testPage.mouse.click(4, 4);
  await expect(testPage.getByTestId("task-management-menu")).toHaveCount(0);
  await expect(ui.trigger(a.id)).toBeFocused();
});
