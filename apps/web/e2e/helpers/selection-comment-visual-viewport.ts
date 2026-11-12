import type { Page } from "@playwright/test";

type VisualViewportOverride = {
  viewport: VisualViewport;
  height?: PropertyDescriptor;
  offsetTop?: PropertyDescriptor;
};

const OVERRIDE_KEY = "__kandevSelectionCommentViewportOverride";

export async function setSelectionCommentVisualViewport(
  page: Page,
  { height, offsetTop }: { height: number; offsetTop: number },
) {
  return page.evaluate(
    ({ height, offsetTop, overrideKey }) => {
      const viewport = window.visualViewport;
      if (!viewport) throw new Error("VisualViewport is unavailable");
      const windowState = window as Window & Record<string, unknown>;
      if (!windowState[overrideKey]) {
        windowState[overrideKey] = {
          viewport,
          height: Object.getOwnPropertyDescriptor(viewport, "height"),
          offsetTop: Object.getOwnPropertyDescriptor(viewport, "offsetTop"),
        } satisfies VisualViewportOverride;
      }
      Object.defineProperty(viewport, "height", { configurable: true, value: height });
      Object.defineProperty(viewport, "offsetTop", { configurable: true, value: offsetTop });
      viewport.dispatchEvent(new Event("resize"));
      viewport.dispatchEvent(new Event("scroll"));
      return { innerHeight: window.innerHeight, viewportHeight: viewport.height };
    },
    { height, offsetTop, overrideKey: OVERRIDE_KEY },
  );
}

export async function restoreSelectionCommentVisualViewport(page: Page) {
  if (page.isClosed()) return;
  await page.evaluate((overrideKey) => {
    const windowState = window as Window & Record<string, unknown>;
    const override = windowState[overrideKey] as VisualViewportOverride | undefined;
    if (!override) return;
    const { viewport, height, offsetTop } = override;
    if (height) Object.defineProperty(viewport, "height", height);
    else Reflect.deleteProperty(viewport, "height");
    if (offsetTop) Object.defineProperty(viewport, "offsetTop", offsetTop);
    else Reflect.deleteProperty(viewport, "offsetTop");
    viewport.dispatchEvent(new Event("resize"));
    viewport.dispatchEvent(new Event("scroll"));
    delete windowState[overrideKey];
  }, OVERRIDE_KEY);
}
