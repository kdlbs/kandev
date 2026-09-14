import { expect, type Page } from "@playwright/test";
import type { SessionPage } from "../../pages/session-page";

export async function assertComposerFocusAfterSend(
  session: SessionPage,
  page: Page,
  submitFollowUp: () => Promise<void>,
) {
  const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");

  await session.sendMessageViaButton("first message after send");
  await expect(
    session.activeChat().getByText("first message after send", { exact: false }),
  ).toBeVisible({ timeout: 15_000 });
  await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
  await expect(editor).toBeFocused({ timeout: 10_000 });

  // No click or editor focus here. The composer must already hold focus.
  await page.keyboard.type("second message after send");
  await submitFollowUp();
  await expect(
    session.activeChat().getByText("second message after send", { exact: false }),
  ).toBeVisible({ timeout: 15_000 });
  await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
  await expect(editor).toBeFocused({ timeout: 10_000 });
}
