import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { SessionPage } from "../../pages/session-page";

export async function pasteOversizedMessage(
  page: Page,
  session: SessionPage,
  source: string,
  tail: string,
) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.evaluate((text) => navigator.clipboard.writeText(text), source);
  const editor = await session.fillMessage("");
  await editor.press("ControlOrMeta+V");
  await expect(editor).toContainText(tail);
  return editor;
}
