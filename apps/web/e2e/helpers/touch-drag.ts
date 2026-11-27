import { expect, type Locator, type Page } from "@playwright/test";
import { dwell } from "./causal-waits";

export async function scrollSidebarFilterListTopIntoView(list: Locator): Promise<void> {
  await list.evaluate((element) => {
    const scrollRegion = element.closest<HTMLElement>('[data-testid="sidebar-filter-popover"]');
    if (!scrollRegion) throw new Error("Sidebar reorder list has no editor scroll region");
    scrollRegion.scrollTop +=
      element.getBoundingClientRect().top - scrollRegion.getBoundingClientRect().top - 12;
  });
}

export async function touchDragToPoint(
  page: Page,
  source: Locator,
  end: { x: number; y: number },
  afterActivation?: () => Promise<void>,
): Promise<void> {
  const sourceBox = await source.boundingBox();
  expect(sourceBox).not.toBeNull();
  await touchDragBetween(
    page,
    { x: sourceBox!.x + sourceBox!.width / 2, y: sourceBox!.y + sourceBox!.height / 2 },
    end,
    afterActivation,
  );
}

export async function touchDragBetween(
  page: Page,
  start: { x: number; y: number },
  end: { x: number; y: number },
  afterActivation?: () => Promise<void>,
): Promise<void> {
  const client = await page.context().newCDPSession(page);
  await client.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ id: 1, x: start.x, y: start.y }],
  });
  await dwell(300, "library-timer", "dnd-kit TouchSensor waits 250ms before activating");
  await afterActivation?.();
  for (let step = 1; step <= 12; step += 1) {
    const progress = step / 12;
    await client.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        {
          id: 1,
          x: start.x + (end.x - start.x) * progress,
          y: start.y + (end.y - start.y) * progress,
        },
      ],
    });
    await page.evaluate(
      () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())),
    );
  }
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await client.detach();
}
