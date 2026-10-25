import { expect, type Locator, type Page } from "@playwright/test";

export async function expectMCPCustomizationScrolling(
  page: Page,
  explorer: Locator,
  touch: boolean,
) {
  const scroll = explorer.getByTestId("mcp-customization-scroll");
  const tabs = explorer.getByRole("tablist");
  const tabsBefore = (await tabs.boundingBox())!;
  const lastChoice = explorer.getByRole("checkbox").last();
  await expect(lastChoice).toHaveCount(1);
  await expect
    .poll(() => scroll.evaluate((element) => element.scrollHeight - element.clientHeight))
    .toBeGreaterThan(0);
  const bounds = (await scroll.boundingBox())!;
  const surface = (await explorer.boundingBox())!;
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(surface.y + surface.height);

  for (let attempt = 0; attempt < 10; attempt++) {
    const position = await scroll.evaluate((element) => ({
      top: element.scrollTop,
      atBottom: element.scrollTop + element.clientHeight >= element.scrollHeight - 1,
    }));
    if (position.atBottom) break;
    if (touch) await swipeUp(page, scroll);
    else {
      await scroll.hover();
      await page.mouse.wheel(0, 1000);
    }
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(position.top);
  }
  await expect
    .poll(async () => {
      const [box, container] = await Promise.all([lastChoice.boundingBox(), scroll.boundingBox()]);
      return (
        !!box &&
        !!container &&
        box.y >= container.y &&
        box.y + box.height <= container.y + container.height + 1
      );
    })
    .toBe(true);
  expect(Math.abs((await tabs.boundingBox())!.y - tabsBefore.y)).toBeLessThanOrEqual(1);
  await expect(explorer.getByTestId("mcp-explorer-close")).toBeVisible();

  // Return to the metadata without changing any saved selection.
  await explorer
    .getByTestId("mcp-inherited-group")
    .getByRole("heading")
    .first()
    .scrollIntoViewIfNeeded();
}

async function swipeUp(page: Page, scroll: Locator) {
  const bounds = (await scroll.boundingBox())!;
  const client = await page.context().newCDPSession(page);
  const x = bounds.x + bounds.width * 0.8;
  const startY = bounds.y + bounds.height - 32;
  const distance = Math.min(bounds.height - 64, 450);
  try {
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x, y: startY }],
    });
    for (let step = 1; step <= 12; step++) {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y: startY - (distance * step) / 12 }],
      });
    }
  } finally {
    try {
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    } finally {
      await client.detach();
    }
  }
}
