/**
 * Mobile entry points for Quick Chat.
 *
 * Desktop opens Quick Chat via keyboard shortcut, command palette, or the app
 * sidebar — none of which exist on a touch viewport. These tests cover the
 * touch affordances: the listing menu on Home, the task-switcher sheet
 * button on a session page, and the explicit close control (touch devices have
 * no Escape key and the full-screen dialog leaves no overlay to tap).
 *
 * Lives in `mobile-*.spec.ts` so the `mobile-chrome` Playwright project applies
 * the mobile device automatically.
 */
import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { readDialogRenderingMetrics } from "../../helpers/dialog-webkit-metrics";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  readQuickChatViewportLayout,
  sendQuickChatMessage,
  startQuickChatFromSetup,
  waitForQuickChatComposerReady,
} from "./quick-chat-helpers";

const TASK_LISTING_VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";

async function readDynamicViewportBounds(page: Page) {
  return page.evaluate(() => {
    const probe = document.createElement("div");
    probe.style.cssText =
      "position:fixed;inset:0;width:100vw;height:100dvh;visibility:hidden;pointer-events:none";
    document.body.appendChild(probe);
    const bounds = probe.getBoundingClientRect();
    probe.remove();
    return { width: bounds.width, height: bounds.height };
  });
}

test.describe("Quick Chat entry points on mobile", () => {
  test("keeps clarification shortcuts usable after tapping a message", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(dialog, testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-multi");

    const clarification = dialog.getByTestId("clarification-overlay");
    await expect(clarification).toBeVisible({ timeout: 30_000 });
    await dialog
      .getByTestId("quick-chat-messages")
      .getByText("/e2e:clarification-multi", { exact: true })
      .tap();
    await expect(dialog.getByTestId("quick-chat-content")).toBeFocused();

    await testPage.keyboard.press("1");
    await expect(
      clarification.locator('[data-testid="clarification-step"][data-step-index="1"]'),
    ).toHaveAttribute("data-active", "true");
  });

  // @covers AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.5 AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.6
  test("preserves composer containment at the bottom of the phone dialog", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
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

    // Submit once: replaying a successful send while awaiting completion
    // creates duplicate bulk turns and changes the layout under measurement.
    const editor = await waitForQuickChatComposerReady(dialog);
    await editor.fill("/e2e:bulk:20");
    await dialog.getByTestId("submit-message-button").tap();
    await expect(dialog.getByText(/Done\. Emitted 20 messages/)).toBeVisible({ timeout: 30_000 });
    await expect(editor).toHaveText("", { timeout: 15_000 });

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
  test("opens from the home menu and closes with the touch control", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    await assertNoDocumentHorizontalOverflow(testPage);

    await testPage.getByTestId("app-nav-trigger").tap();
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
    await expect(testPage.getByTestId("app-nav-trigger")).toBeFocused();
  });

  // @covers AC-UI-QUICK-CHAT-ELEVATION-001.4 AC-UI-QUICK-CHAT-ELEVATION-001.7
  test("keeps the WebKit Quick Chat dialog full-screen below the desktop breakpoint", async ({
    testPage,
  }) => {
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    await testPage.locator("html").evaluate((root) => {
      root.setAttribute("data-rendering-engine", "webkit");
    });

    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });

    const expectPhoneBounds = async (width: number) => {
      await testPage.setViewportSize({ width, height: 851 });
      await expect
        .poll(async () => {
          const [dialogBounds, viewportBounds] = await Promise.all([
            dialog.boundingBox(),
            readDynamicViewportBounds(testPage),
          ]);
          if (!dialogBounds) return Number.POSITIVE_INFINITY;
          return Math.max(
            Math.abs(dialogBounds.width - viewportBounds.width),
            Math.abs(dialogBounds.height - viewportBounds.height),
          );
        })
        .toBeLessThan(3);
      const box = await dialog.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeCloseTo(0, 0);
      expect(box!.y).toBeCloseTo(0, 0);
      expect(box!.width).toBeCloseTo((await readDynamicViewportBounds(testPage)).width, 0);
      expect(await testPage.evaluate(() => window.matchMedia("(min-width: 640px)").matches)).toBe(
        false,
      );
      await assertNoDocumentHorizontalOverflow(testPage, `Quick Chat at ${width}px`);
    };

    const firstMetrics = await readDialogRenderingMetrics(dialog);
    expect(firstMetrics.animationName).toBe("kandev-dialog-webkit-enter");
    expect(firstMetrics.transform).toBe("none");
    expect(firstMetrics.translate).toBe("none");
    await expectPhoneBounds(testPage.viewportSize()!.width);
    await expectPhoneBounds(639);

    await testPage.setViewportSize({ width: 640, height: 851 });
    await expect
      .poll(async () => {
        const [dialogBounds, viewportBounds] = await Promise.all([
          dialog.boundingBox(),
          readDynamicViewportBounds(testPage),
        ]);
        if (!dialogBounds) return Number.POSITIVE_INFINITY;
        return Math.max(
          Math.abs(dialogBounds.height - viewportBounds.height * 0.85),
          Math.abs(dialogBounds.x + dialogBounds.width / 2 - viewportBounds.width / 2),
        );
      })
      .toBeLessThan(3);
    const desktopMetrics = await readDialogRenderingMetrics(dialog);
    const desktopBox = await dialog.boundingBox();
    expect(desktopBox).not.toBeNull();
    expect(desktopMetrics.animationName).toBe("kandev-dialog-webkit-enter");
    expect(desktopMetrics.transform).toBe("none");
    expect(desktopMetrics.translate).toBe("none");
    expect(desktopMetrics.width).toBeLessThan(640);
    expect(desktopMetrics.centerX).toBeCloseTo(
      (await readDynamicViewportBounds(testPage)).width / 2,
      0,
    );
    expect(await testPage.evaluate(() => window.matchMedia("(min-width: 640px)").matches)).toBe(
      true,
    );
    await assertNoDocumentHorizontalOverflow(testPage, "Quick Chat at 640px");

    await testPage.setViewportSize({ width: 393, height: 851 });
    await dialog.getByTestId("quick-chat-close").tap();
    await expect(dialog).not.toBeVisible();
    await expect(testPage.getByTestId("app-nav-trigger")).toBeFocused();
  });

  test("chooses configuration mode on the opening composer", async ({ testPage }) => {
    await testPage.goto("/");
    const context = testPage.getByTestId("app-nav-trigger");
    await context.tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    const setup = dialog.getByTestId("quick-chat-setup");
    const modeSwitch = setup.getByTestId("quick-chat-configuration-action");
    await modeSwitch.tap();
    await testPage.getByRole("switch", { name: "Configuration chat" }).tap();
    await expect(setup.getByRole("status")).toHaveText("Configuration chat");
    await assertNoDocumentHorizontalOverflow(testPage);
    await dialog.getByTestId("quick-chat-close").tap();
    await expect(dialog).toBeHidden();
    await expect(context).toBeFocused();
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

    await testPage.getByTestId("mobile-task-picker-trigger").tap();
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

    await testPage.getByTestId("app-nav-trigger").tap();
    const homeLink = testPage.getByRole("link", { name: "Home", exact: true });
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
    await expect(
      testPage
        .getByTestId("tasks-list-view")
        .getByText("No tasks match these filters", { exact: true }),
    ).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
