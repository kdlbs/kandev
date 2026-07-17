/**
 * Mobile entry points for Quick Chat.
 *
 * Desktop opens Quick Chat via keyboard shortcut, command palette, or the app
 * sidebar — none of which exist on a touch viewport. These tests cover the
 * touch affordances: the kanban header button on Home, the task-switcher sheet
 * button on a session page, and the explicit close control (touch devices have
 * no Escape key and the full-screen dialog leaves no overlay to tap).
 *
 * Lives in `mobile-*.spec.ts` so the `mobile-chrome` Playwright project applies
 * the mobile device automatically.
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Quick Chat entry points on mobile", () => {
  test("opens from the home header and closes with the touch control", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    await assertNoDocumentHorizontalOverflow(testPage);

    await testPage.getByTestId("mobile-quick-chat-button").click();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
    await assertNoDocumentHorizontalOverflow(testPage);

    await dialog.getByTestId("quick-chat-close").click();
    await expect(dialog).not.toBeVisible();
  });

  test("opens from the task switcher sheet on a session page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const seeded = await apiClient.seedTask(seedData.workspaceId, "Mobile Quick Chat Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${seeded.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(sheet).toBeVisible();
    await testPage.getByTestId("mobile-sheet-quick-chat").click();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
    // Opening quick chat dismisses the task switcher sheet.
    await expect(sheet).not.toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
