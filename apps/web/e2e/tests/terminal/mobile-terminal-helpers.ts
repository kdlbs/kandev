// Shared helpers for the mobile terminal specs (keybar + scroll). Both specs
// run on the mobile-chrome Playwright project and share the same lazy-mount +
// shell-connect path, so the connect/readiness helpers live here to avoid
// drift between the two files.
import { type Page, expect } from "@playwright/test";

export async function tapTerminalTab(testPage: Page): Promise<void> {
  await testPage.getByRole("button", { name: "Terminal" }).tap();
}

export async function switchToTerminalPanel(testPage: Page): Promise<void> {
  // Confirm the panel actually mounted rather than firing a single tap. On
  // mobile the bottom-nav button can be tapped before hydration wires its
  // handler; a lost tap leaves the terminal panel unmounted, which would later
  // strand waitForShellReady polling an element that never appears. Re-tap once
  // if the first tap didn't take.
  const panel = testPage.getByTestId("terminal-panel");
  await tapTerminalTab(testPage);
  if (!(await panel.isVisible())) {
    await tapTerminalTab(testPage);
  }
  await expect(panel).toBeVisible({ timeout: 10_000 });
}

export async function readTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    const xterms = Array.from(panel?.querySelectorAll(".xterm") ?? []);
    const xtermEl = panel?.querySelector(".xterm.focus") ?? xterms.at(-1);
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

export async function focusTerminalForTyping(testPage: Page): Promise<void> {
  await testPage.getByTestId("terminal-panel").locator(".xterm").last().click();
}

/**
 * Wait for the mobile shell to be ready by tailing xterm's buffer until it has
 * any content (a prompt is enough). Mobile mounts the terminal lazily on tab
 * switch so this can take longer than desktop.
 *
 * The shell WS connect can be missed under CI load (the auto-create guard only
 * retries on a WS reconnect). If the panel falls out of view we re-tap it,
 * which forces a remount and kicks the reconnect loop — so we don't blindly
 * wait out the whole budget on a dead connection.
 */
export async function waitForShellReady(testPage: Page, timeout = 45_000): Promise<void> {
  const panel = testPage.getByTestId("terminal-panel");
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if ((await readTerminalBuffer(testPage)).length > 0) return;
    if (!(await panel.isVisible())) {
      await switchToTerminalPanel(testPage);
    }
    await testPage.waitForTimeout(1_000);
  }
  expect(
    (await readTerminalBuffer(testPage)).length,
    "Waiting for mobile terminal shell to connect",
  ).toBeGreaterThan(0);
}
