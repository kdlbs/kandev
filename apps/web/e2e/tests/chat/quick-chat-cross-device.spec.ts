import type { Browser, BrowserContext, Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { openQuickChatWithAgent, startQuickChatFromSetup } from "./quick-chat-helpers";

/**
 * Quick chats are shared state: they are created, renamed and closed from any
 * device, and every client must converge on the same tab strip. Before this
 * suite existed, the list was a one-shot boot-payload snapshot, so two
 * long-lived clients silently drifted apart.
 *
 * "Device B" is a second browser context against the same backend — a distinct
 * client with its own store and WebSocket, which is exactly what a phone is.
 */

const DESKTOP_VIEWPORT = { width: 1440, height: 900 };
const MOBILE_VIEWPORT = { width: 393, height: 851 };

/**
 * Opens an independent client against the same backend: its own context means
 * its own store, WebSocket and localStorage, which is what makes it a
 * meaningful stand-in for the user's other device.
 */
async function openSecondDevice(
  browser: Browser,
  frontendUrl: string,
  backendPort: number,
  viewport: { width: number; height: number },
): Promise<{ page: Page; context: BrowserContext }> {
  const context = await browser.newContext({ baseURL: frontendUrl, viewport });
  const page = await context.newPage();
  await page.addInitScript(
    ({ port }: { port: number }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = String(port);
      window.__KANDEV_E2E_EXPOSE_STORE__ = true;
    },
    { port: backendPort },
  );
  return { page, context };
}

/** Opens quick chat on a client that already has at least one chat to restore. */
async function openExistingQuickChat(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 15_000 });
  return dialog;
}

function tabNames(dialog: Locator): Promise<string[]> {
  return dialog.getByTestId("quick-chat-tab").locator("span").allTextContents();
}

test.describe("Quick Chat cross-device sync", () => {
  test("a chat started on one device appears on the other, and renames follow", async ({
    testPage,
    backend,
    browser,
  }, testInfo) => {
    test.setTimeout(180_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);

    // Device A: desktop. Start one chat so both clients share a baseline.
    await testPage.setViewportSize(DESKTOP_VIEWPORT);
    const dialogA = await openQuickChatWithAgent(testPage);
    await expect(dialogA.getByTestId("quick-chat-tab")).toHaveCount(1);

    // Device B: a second client, opened after that first chat already existed.
    const second = await openSecondDevice(
      browser,
      backend.frontendUrl,
      backend.port,
      MOBILE_VIEWPORT,
    );
    const dialogB = await openExistingQuickChat(second.page);
    await expect(dialogB.getByTestId("quick-chat-tab")).toHaveCount(1);

    // Device A starts a second chat while B is already open and idle. B must
    // learn about it from the task event alone — no reload, no navigation.
    await dialogA.getByLabel("Start new chat").click();
    await startQuickChatFromSetup(dialogA, testPage);
    await expect(dialogA.getByTestId("quick-chat-tab")).toHaveCount(2);

    await expect(dialogB.getByTestId("quick-chat-tab")).toHaveCount(2, { timeout: 30_000 });

    // Renaming on A must re-label the same tab on B, because the name now
    // lives on the backing task rather than in A's localStorage.
    const renamed = "Renamed from desktop";
    const activeTabA = dialogA.getByTestId("quick-chat-tab").last();
    await activeTabA.dblclick();
    const renameInput = dialogA.locator('[data-testid="quick-chat-tab"] input');
    await expect(renameInput).toBeVisible({ timeout: 5_000 });
    await renameInput.fill(renamed);
    await renameInput.press("Enter");

    await expect
      .poll(() => tabNames(dialogB), { timeout: 30_000 })
      .toEqual(expect.arrayContaining([renamed]));

    await capture.screenshot("desktop-quick-chat-tabs", {
      caption: "Desktop: two quick chats, the second renamed here",
    });
    await capture.screenshot("mobile-quick-chat-tabs", {
      page: second.page,
      caption: "Mobile, same account: both tabs and the rename arrived live",
    });
    capture.flush();

    await second.context.close();
  });

  test("closing a chat on one device removes it from the other", async ({
    testPage,
    backend,
    browser,
  }) => {
    test.setTimeout(180_000);

    await testPage.setViewportSize(DESKTOP_VIEWPORT);
    const dialogA = await openQuickChatWithAgent(testPage);
    await dialogA.getByLabel("Start new chat").click();
    await startQuickChatFromSetup(dialogA, testPage);
    await expect(dialogA.getByTestId("quick-chat-tab")).toHaveCount(2);

    const second = await openSecondDevice(
      browser,
      backend.frontendUrl,
      backend.port,
      DESKTOP_VIEWPORT,
    );
    const dialogB = await openExistingQuickChat(second.page);
    await expect(dialogB.getByTestId("quick-chat-tab")).toHaveCount(2, { timeout: 30_000 });

    // Close the second chat on A and confirm the deletion. Each tab holds both
    // a label button and a close button, so target the close one by its label.
    await dialogA
      .getByTestId("quick-chat-tab")
      .last()
      .getByRole("button", { name: /^Close / })
      .click();
    const confirm = testPage.getByRole("alertdialog");
    await expect(confirm).toBeVisible({ timeout: 10_000 });
    // Target the destructive action exactly; a loose regex plus .last() would
    // happily click Cancel if the footer order ever changed.
    const confirmDelete = confirm.getByRole("button", { name: "Delete", exact: true });
    await expect(confirmDelete).toHaveCount(1);
    await confirmDelete.click();
    await expect(dialogA.getByTestId("quick-chat-tab")).toHaveCount(1, { timeout: 30_000 });

    // B must drop the tab too rather than keeping a ghost pointing at a task
    // the backend has already deleted.
    await expect(dialogB.getByTestId("quick-chat-tab")).toHaveCount(1, { timeout: 30_000 });

    await second.context.close();
  });
});
