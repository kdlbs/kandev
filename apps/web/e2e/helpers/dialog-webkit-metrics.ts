import { expect, type Locator } from "@playwright/test";

export type DialogRenderingMetrics = {
  animationName: string;
  transform: string;
  translate: string;
  zIndex: string;
  overlayZIndex: string;
  centerX: number;
  centerY: number;
  width: number;
  height: number;
  contentOverOverlay: boolean;
};

type DialogRenderingMetricsOptions = {
  overlaySelector?: string;
  contentSelector?: string;
};

export async function readDialogRenderingMetrics(
  dialog: Locator,
  {
    overlaySelector = '[data-slot="dialog-overlay"]',
    contentSelector,
  }: DialogRenderingMetricsOptions = {},
): Promise<DialogRenderingMetrics> {
  await dialog.evaluate(async (element) => {
    const animations = element.getAnimations().filter((animation) => {
      if (animation.playState !== "running") return false;
      const iterations = animation.effect?.getComputedTiming().iterations;
      return typeof iterations === "number" && Number.isFinite(iterations);
    });
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
  });

  return dialog.evaluate(
    (element: HTMLElement, selectors: DialogRenderingMetricsOptions) => {
      const style = getComputedStyle(element);
      const box = element.getBoundingClientRect();
      const center = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
      const overlay = document.querySelector<HTMLElement>(selectors.overlaySelector!);
      return {
        animationName: style.animationName,
        transform: style.transform,
        translate: style.translate,
        zIndex: style.zIndex,
        overlayZIndex: overlay ? getComputedStyle(overlay).zIndex : "",
        centerX: box.left + box.width / 2,
        centerY: box.top + box.height / 2,
        width: box.width,
        height: box.height,
        contentOverOverlay: selectors.contentSelector
          ? center?.closest(selectors.contentSelector) === element
          : false,
      };
    },
    { overlaySelector, contentSelector },
  );
}

export async function expectWebkitDialogMotion(
  dialog: Locator,
  options: DialogRenderingMetricsOptions & {
    contentZIndex: string;
    overlayZIndex: string;
  },
): Promise<DialogRenderingMetrics> {
  const metrics = await readDialogRenderingMetrics(dialog, options);
  expect(metrics.animationName).toBe("kandev-dialog-webkit-enter");
  expect(["none", "matrix(1, 0, 0, 1, 0, 0)"]).toContain(metrics.transform);
  expect(["none", "0px", "0px 0px"]).toContain(metrics.translate);
  expect(metrics.zIndex).toBe(options.contentZIndex);
  expect(metrics.overlayZIndex).toBe(options.overlayZIndex);
  return metrics;
}
