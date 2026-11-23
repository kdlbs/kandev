import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import {
  openNewQuickChatSetup,
  openQuickChatSetup,
  sendQuickChatMessage,
  startQuickChatFromSetup,
} from "./quick-chat-helpers";

export async function expectInboxToOpenQuickChat(page: Page, mobile = false) {
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  if (mobile) {
    await page.goto("/");
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("mobile-quick-chat-button").tap();
  } else {
    await openQuickChatSetup(page);
  }

  const target = await startQuickChatFromSetup(dialog, page);
  await sendQuickChatMessage(dialog, page, "/e2e:clarification-multi");
  const clarification = dialog.getByTestId("clarification-overlay");
  await expect(clarification).toBeVisible({ timeout: 30_000 });
  await openNewQuickChatSetup(dialog, page);
  await startQuickChatFromSetup(dialog, page);

  await page.goto("/needs-you-inbox");
  const row = page.getByTestId("needs-you-inbox-row").filter({
    has: page.locator(`a[href="/t/${target.task_id}?sessionId=${target.session_id}"]`),
  });
  const closeFocusTarget = mobile
    ? row.getByRole("button", { name: "Row actions" })
    : row.getByTestId("needs-you-inbox-open-task");
  if (mobile) {
    await closeFocusTarget.tap();
    await page.getByTestId("needs-you-inbox-open-task-menu-item").tap();
  } else {
    await closeFocusTarget.click();
  }

  await expect(page).toHaveURL(/\/needs-you-inbox$/);
  await expect(clarification).toBeVisible();
  if (mobile) {
    await dialog.getByTestId("quick-chat-close").tap();
  } else {
    await page.keyboard.press("Escape");
    await expect(clarification).toBeHidden();
    await page.keyboard.press("Escape");
  }
  await expect(dialog).toBeHidden();
  await expect(closeFocusTarget).toBeFocused();
  await expect(page).toHaveURL(/\/needs-you-inbox$/);
}
