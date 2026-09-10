import type { Page } from "@playwright/test";

export async function swipeDeckLeft(page: Page, whileHeld?: () => Promise<void>) {
  const box = await page.getByTestId("threads-board").boundingBox();
  if (!box) throw new Error("Threads deck has no bounding box");
  const client = await page.context().newCDPSession(page);
  const y = box.y + 24;
  const startX = box.x + box.width * 0.85;
  await client.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: startX, y }],
  });
  try {
    for (let step = 1; step <= 12; step++) {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x: startX - (box.width * 0.7 * step) / 12, y }],
      });
    }
    await whileHeld?.();
  } finally {
    await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await client.detach();
  }
}
