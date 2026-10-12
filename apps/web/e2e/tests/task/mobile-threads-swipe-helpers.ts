import type { Page } from "@playwright/test";

export async function swipeDeckLeft(page: Page, whileHeld?: () => Promise<void>) {
  return swipeDeck(page, "left", whileHeld);
}

export async function swipeDeckRight(page: Page) {
  return swipeDeck(page, "right");
}

async function swipeDeck(page: Page, direction: "left" | "right", whileHeld?: () => Promise<void>) {
  const box = await page.getByTestId("threads-board").boundingBox();
  if (!box) throw new Error("Threads deck has no bounding box");
  const client = await page.context().newCDPSession(page);
  const y = box.y + 24;
  const startX = box.x + box.width * (direction === "left" ? 0.85 : 0.15);
  const distance = box.width * 0.7 * (direction === "left" ? -1 : 1);
  try {
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: startX, y }],
    });
    for (let step = 1; step <= 12; step++) {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x: startX + (distance * step) / 12, y }],
      });
    }
    await whileHeld?.();
  } finally {
    try {
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    } finally {
      await client.detach();
    }
  }
}
