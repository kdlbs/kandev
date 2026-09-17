import type { Locator, Page } from "@playwright/test";
import type { AppState } from "@/lib/state/store";
import { expect } from "../../fixtures/test-base";

type ModelStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => AppState;
    setState: (state: Partial<AppState>) => void;
  };
};

export async function seedLongModelList(page: Page, sessionId?: string) {
  await expect
    .poll(() =>
      page.evaluate((sid) => {
        const store = (window as ModelStoreWindow).__KANDEV_E2E_STORE__;
        if (!store) return false;
        const state = store.getState();
        const id = sid ?? state.quickChat.activeSessionId;
        if (!id) return false;
        const entry = state.sessionModels.bySessionId[id];
        const model = entry?.configOptions.find((option) => option.id === "model");
        if (!model?.options?.some((option) => option.value === "mock-smart")) return false;
        const options = [
          ...Array.from({ length: 25 }, (_, index) => ({
            value: `scroll-model-${index}`,
            name: `Scroll model ${index + 1}`,
          })),
          ...model.options.filter(
            (option) => option.value === "mock-fast" || option.value === "mock-smart",
          ),
        ];
        store.setState({
          sessionModels: {
            ...state.sessionModels,
            bySessionId: {
              ...state.sessionModels.bySessionId,
              [id]: {
                ...entry,
                configOptions: entry.configOptions.map((option) =>
                  option.id === "model" ? { ...option, options } : option,
                ),
              },
            },
          },
        });
        return true;
      }, sessionId),
    )
    .toBe(true);
}

export async function wheelModelList(page: Page, list: Locator) {
  expect(await list.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
  const box = await list.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.mouse.wheel(0, 3000);
  await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
}

export async function swipeModelList(page: Page, list: Locator) {
  const box = await list.boundingBox();
  expect(box).not.toBeNull();
  const cdp = await page.context().newCDPSession(page);
  try {
    const x = box!.x + box!.width / 2;
    const startY = box!.y + box!.height - 20;
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x, y: startY }],
    });
    for (let step = 1; step <= 8; step++) {
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y: startY - ((box!.height - 40) * step) / 8 }],
      });
    }
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  } finally {
    await cdp.detach();
  }
}

export async function assertPickerContained(page: Page) {
  const box = await page.locator('[data-slot="popover-content"][data-state="open"]').boundingBox();
  const viewport = page.viewportSize()!;
  expect(box).not.toBeNull();
  // Floating UI rounds positions to device pixels (Pixel 5 uses fractional DPR).
  const roundingTolerance = 1;
  expect(box!.x).toBeGreaterThanOrEqual(-roundingTolerance);
  expect(box!.y).toBeGreaterThanOrEqual(-roundingTolerance);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width + roundingTolerance);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height + roundingTolerance);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
    viewport.width,
  );
}
