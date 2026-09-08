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
import {
  readQuickChatViewportLayout,
  sendQuickChatMessage,
  startQuickChatFromSetup,
} from "./quick-chat-helpers";

const TASK_LISTING_VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";

test.describe("Quick Chat entry points on mobile", () => {
  // @covers AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.5 AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.6
  test("preserves composer containment at the bottom of the phone dialog", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(dialog, testPage);

    const layout = await readQuickChatViewportLayout(dialog);
    expect(layout.composerTop).toBeGreaterThan(layout.dialogTop);
    expect(layout.composerBottom).toBeLessThanOrEqual(layout.dialogBottom + 1);
    expect(layout.dialogBottom - layout.composerBottom).toBeLessThanOrEqual(8);
    expect(layout.contentBottom).toBeLessThanOrEqual(layout.dialogBottom + 1);
    expect(layout.dialogBottom - layout.contentBottom).toBeLessThanOrEqual(2);
    expect(layout.dialogScrollHeight).toBeLessThanOrEqual(layout.dialogClientHeight + 1);

    await sendQuickChatMessage(dialog, testPage, "/e2e:bulk:20");
    await expect(dialog.getByText(/Done\. Emitted 20 messages/)).toBeVisible({ timeout: 30_000 });

    const longChatLayout = await readQuickChatViewportLayout(dialog);
    expect(longChatLayout.messageScrollerScrollHeight).toBeGreaterThan(
      longChatLayout.messageScrollerClientHeight,
    );
    expect(longChatLayout.composerBottom).toBeLessThanOrEqual(longChatLayout.dialogBottom + 1);
    expect(longChatLayout.dialogBottom - longChatLayout.composerBottom).toBeLessThanOrEqual(8);
    expect(longChatLayout.dialogScrollHeight).toBeLessThanOrEqual(
      longChatLayout.dialogClientHeight + 1,
    );
  });

  // @covers AC-UI-QUICK-CHAT-ELEVATION-001.4 AC-UI-QUICK-CHAT-ELEVATION-001.7
  test("opens from the home header and closes with the touch control", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    await assertNoDocumentHorizontalOverflow(testPage);

    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
    const overlay = testPage.locator('[data-slot="dialog-overlay"]');
    await expect(overlay).toBeAttached();
    const mobileBackdropStyles = await overlay.evaluate((element) => {
      const styles = getComputedStyle(element);
      return {
        backgroundColor: styles.backgroundColor,
        backdropFilter: styles.backdropFilter,
      };
    });
    expect(mobileBackdropStyles.backgroundColor).toMatch(/\/\s*0\.2\)/);
    expect(mobileBackdropStyles.backdropFilter).toBe("none");
    await assertNoDocumentHorizontalOverflow(testPage);

    await dialog.getByTestId("quick-chat-close").tap();
    await expect(dialog).not.toBeVisible();
  });

  test("chooses configuration mode from the setup panel", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    const setup = dialog.getByTestId("quick-chat-setup");
    await expect(setup.getByText(/quick chats stay outside your task board/i)).toBeVisible();
    await setup.getByRole("switch", { name: "Configuration chat" }).tap();

    await expect(dialog.getByTestId("config-chat-setup")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
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

    await testPage.getByTestId("mobile-session-menu").tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(sheet).toBeVisible();
    await testPage.getByTestId("mobile-sheet-quick-chat").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
    // Opening quick chat dismisses the task switcher sheet.
    await expect(sheet).not.toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });

  test("preserves the selected workspace when Home restores List", async ({
    testPage,
    seedData,
  }) => {
    await testPage.addInitScript(
      (key) => window.localStorage.setItem(key, JSON.stringify("list")),
      TASK_LISTING_VIEW_STORAGE_KEY,
    );
    await testPage.goto(`/tasks?workspace=${seedData.workspaceId}`);
    await testPage.waitForLoadState("networkidle");

    const homeLink = testPage.getByRole("link", { name: "Kandev home" });
    await expect(homeLink).toHaveAttribute(
      "href",
      `/?home=overview&workspaceId=${seedData.workspaceId}`,
    );

    await homeLink.click();

    await expect(testPage).toHaveURL((url) => {
      return (
        url.pathname === "/tasks" && url.searchParams.get("workspace") === seedData.workspaceId
      );
    });
    await expect(testPage.getByText("No tasks found.", { exact: true })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
