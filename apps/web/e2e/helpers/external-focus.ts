import { expect, type Page } from "@playwright/test";

/** Exercise native browser focus instead of Playwright's always-focused emulation. */
export async function visitOtherBrowserTab(page: Page, whileAway: () => Promise<void>) {
  const context = page.context();
  const session = await context.newCDPSession(page);
  await session.send("Emulation.setFocusEmulationEnabled", { enabled: false });
  const events = await page.evaluateHandle(() => {
    const state = { blurs: 0, focuses: 0 };
    const blur = () => {
      state.blurs += 1;
    };
    const focus = () => {
      state.focuses += 1;
    };
    window.addEventListener("blur", blur);
    window.addEventListener("focus", focus);
    return {
      state,
      dispose: () => {
        window.removeEventListener("blur", blur);
        window.removeEventListener("focus", focus);
      },
    };
  });
  const other = await context.newPage();
  try {
    const otherSession = await context.newCDPSession(other);
    await otherSession.send("Emulation.setFocusEmulationEnabled", { enabled: false });
    await other.goto("about:blank");
    await other.bringToFront();
    await expect.poll(() => page.evaluate(() => document.hasFocus())).toBe(false);
    await expect.poll(() => events.evaluate(({ state }) => state.blurs)).toBeGreaterThan(0);
    await whileAway();
    await page.bringToFront();
    await expect.poll(() => page.evaluate(() => document.hasFocus())).toBe(true);
    await expect.poll(() => events.evaluate(({ state }) => state.focuses)).toBeGreaterThan(0);
  } finally {
    await other.close();
    await events.evaluate(({ dispose }) => dispose());
    await events.dispose();
    await session.send("Emulation.setFocusEmulationEnabled", { enabled: true });
    await session.detach();
  }
}
