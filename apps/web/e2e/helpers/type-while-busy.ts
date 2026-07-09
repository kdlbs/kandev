import { type Locator, type Page } from "@playwright/test";

/**
 * Type text into the TipTap editor while the agent is busy.
 * fill() silently fails on TipTap when the busy placeholder is shown,
 * so we retry clicking and typing until text appears in the editor.
 */
export async function typeWhileBusy(page: Page, editor: Locator, text: string): Promise<void> {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.scrollIntoViewIfNeeded();
  for (let attempt = 0; attempt < 3; attempt++) {
    const box = await editor.boundingBox();
    if (!box) throw new Error("Editor bounding box not found");
    await page.mouse.click(box.x + 20, box.y + box.height / 2);
    await page.waitForTimeout(200);
    await page.keyboard.type(text);
    await page.waitForTimeout(100);
    const content = await editor.textContent();
    if (content?.includes(text)) return;
    // Text wasn't entered; select all and clear for retry
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press("Backspace");
    await page.waitForTimeout(200);
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}
