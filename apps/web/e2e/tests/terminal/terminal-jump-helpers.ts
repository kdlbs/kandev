import { expect, type Locator, type Page } from "@playwright/test";

export const INITIAL_MARKER = "jump-line-200";
export const LATER_MARKER = "jump-later-marker";
export const OUTPUT_COMMAND =
  "seq 1 200 | sed 's/^/jump-line-/'; sleep 10; printf 'jump-%s-%s\\n' later marker; sleep 60";

export async function terminalState(panel: Locator): Promise<{ viewport: number; text: string }> {
  return panel.locator(".xterm").evaluate((element) => {
    type Host = HTMLElement & {
      __xtermReadViewportY?: () => number;
      __xtermReadBuffer?: () => string;
    };
    const host = element.parentElement as Host;
    return {
      viewport: host.__xtermReadViewportY?.() ?? -1,
      text: host.__xtermReadBuffer?.() ?? "",
    };
  });
}

export async function swipeDown(page: Page, panel: Locator) {
  const box = await panel.locator(".xterm").boundingBox();
  if (!box) throw new Error("Terminal has no visible bounds");
  const client = await page.context().newCDPSession(page);
  try {
    const x = box.x + box.width / 2;
    const y = box.y + 20;
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x, y, id: 1 }],
    });
    for (let i = 1; i <= 5; i++) {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y: y + i * 35, id: 1 }],
      });
    }
    await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  } finally {
    await client.detach();
  }
}

export async function verifyJump(
  page: Page,
  panel: Locator,
  mobile: boolean,
  capture?: () => Promise<void>,
  frames?: unknown[],
) {
  await expect
    .poll(async () => (await terminalState(panel)).text, { timeout: 30000 })
    .toContain(INITIAL_MARKER);
  const initial = await terminalState(panel);
  expect(initial.viewport).toBeGreaterThan(0);
  const action = panel.getByRole("button", { name: "Jump to latest output" });
  await expect(action).toHaveCount(0);
  const documentY = await page.evaluate(() => window.scrollY);
  if (mobile) await swipeDown(page, panel);
  else {
    await panel.locator(".xterm").hover();
    await page.mouse.wheel(0, -450);
  }
  await expect
    .poll(async () => (await terminalState(panel)).viewport)
    .toBeLessThan(initial.viewport);
  await expect(action).toBeVisible();
  await capture?.();
  const scrolled = await terminalState(panel);
  await expect
    .poll(async () => (await terminalState(panel)).text, { timeout: 30000 })
    .toContain(LATER_MARKER);
  expect((await terminalState(panel)).viewport).toBe(scrolled.viewport);
  if (frames) expect(frames.length).toBeGreaterThan(0);
  const frameCount = frames?.length;
  if (mobile) {
    const button = await action.boundingBox();
    const bounds = await panel.boundingBox();
    expect(button).not.toBeNull();
    expect(bounds).not.toBeNull();
    expect(button!.width).toBeGreaterThanOrEqual(44);
    expect(button!.height).toBeGreaterThanOrEqual(44);
    expect(button!.x).toBeGreaterThanOrEqual(bounds!.x);
    expect(button!.y + button!.height).toBeLessThanOrEqual(bounds!.y + bounds!.height);
    await action.tap();
  } else {
    await action.focus();
    await action.press("Enter");
  }
  await expect(action).toHaveCount(0);
  if (frames) expect(frames.length).toBe(frameCount);
  await expect
    .poll(async () => (await terminalState(panel)).viewport)
    .toBeGreaterThan(initial.viewport);
  expect(await page.evaluate(() => window.scrollY)).toBe(documentY);
}
