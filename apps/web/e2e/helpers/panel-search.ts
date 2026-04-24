import { type Page, type Locator, expect } from "@playwright/test";

export type PanelKind = "session" | "plan" | "terminal";

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";

/** Locator for any search bar currently mounted in the DOM. */
export function panelSearchBar(page: Page): Locator {
  return page.locator("[data-panel-search-bar]");
}

/** Locator for the search input inside the bar. */
export function panelSearchInput(page: Page): Locator {
  return panelSearchBar(page).locator('input[type="text"]');
}

/** Locator for the "N / M" match counter. */
export function panelSearchMatchCounter(page: Page): Locator {
  return panelSearchBar(page).locator('[aria-live="polite"]');
}

/** Locator for a toggle button by title ("Match case" | "Regular expression"). */
export function panelSearchToggle(page: Page, title: string): Locator {
  return panelSearchBar(page).getByRole("button", { name: title });
}

/** Focus the panel of the given kind so the Ctrl+F listener fires within it. */
async function focusPanel(page: Page, kind: PanelKind): Promise<void> {
  if (kind === "session") {
    // Focus the chat TipTap editor — it lives inside the session-chat panel,
    // so container.contains(document.activeElement) returns true and our hook fires.
    const editor = page.getByTestId("session-chat").locator(".tiptap.ProseMirror").first();
    await editor.click();
    return;
  }
  if (kind === "plan") {
    const editor = page.getByTestId("plan-panel").locator(".ProseMirror").first();
    await editor.click();
    return;
  }
  // terminal: click the xterm canvas area so xterm captures keydown
  const termXterm = page.getByTestId("terminal-panel").locator(".xterm").first();
  await termXterm.click();
}

/** Open the panel-scoped search bar via Ctrl/Cmd+F while the given panel is focused. */
export async function openPanelSearch(page: Page, kind: PanelKind): Promise<void> {
  await focusPanel(page, kind);
  await page.keyboard.press(`${MODIFIER}+f`);
  await expect(panelSearchBar(page)).toBeVisible({ timeout: 5_000 });
}

/** Close the panel-scoped search bar via Escape. */
export async function closePanelSearch(page: Page): Promise<void> {
  await page.keyboard.press("Escape");
  await expect(panelSearchBar(page)).toHaveCount(0, { timeout: 5_000 });
}

/** Assert match counter reads the given current / total pair. */
export async function expectMatchCounter(
  page: Page,
  current: number,
  total: number,
): Promise<void> {
  const text = total === 0 ? "0 / 0" : `${current} / ${total}`;
  await expect(panelSearchMatchCounter(page)).toHaveText(text);
}

/** Type a query into the search input (bypasses debounce by also firing Enter is caller's job). */
export async function typeSearchQuery(page: Page, query: string): Promise<void> {
  const input = panelSearchInput(page);
  await input.click();
  await input.fill(query);
}
