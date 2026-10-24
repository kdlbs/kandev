import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// @covers AC-TASKS-CONVERSATION-SEARCH-001.4
test("searches conversations from a labelled phone action", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone conversation destination", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const session = await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    startedAt: new Date(Date.now() - 60000).toISOString(),
    completedAt: new Date().toISOString(),
  });
  await apiClient.seedSessionMessage(session.session_id, {
    type: "message",
    content: "Discuss the mobilequartz budget here.",
  });
  const mobile = new MobileKanbanPage(testPage);
  await testPage.goto("/");
  await mobile.mobileMenuButton.tap();
  await testPage.getByRole("button", { name: "Search tasks", exact: true }).tap();
  const palette = testPage.getByRole("dialog", { name: "Command Palette", exact: true });
  await expect(palette).toBeVisible();
  await expect(palette).toHaveAttribute("data-slot", "drawer-content");
  const input = palette.getByRole("combobox");
  await input.fill("mobilequartz");
  const row = palette.getByRole("option").filter({ hasText: task.title });
  await expect(row).toBeVisible();
  expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(await input.evaluate((element) => getComputedStyle(element).fontSize)).toBe("16px");
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: "test-results/mobile-conversation-search.png" });
  if (prCapture.capturing) {
    await waitForFiniteAnimations(palette);
    await prCapture.screenshot("phone-conversation-search");
  }
  await row.tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}\\?sessionId=${session.session_id}`));
});
