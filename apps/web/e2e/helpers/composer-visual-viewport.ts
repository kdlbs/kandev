import { expect, type Locator, type Page } from "@playwright/test";

/** Containment stimulus, not an emulation of WebKit or the OS keyboard. */
export async function offsetComposerViewport(page: Page, height = 150) {
  await page.evaluate((visibleHeight) => {
    const viewport = Object.assign(new EventTarget(), {
      offsetLeft: 0,
      offsetTop: window.innerHeight - visibleHeight,
      width: window.innerWidth,
      height: visibleHeight,
      scale: 1,
    });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: viewport });
    window.dispatchEvent(new Event("resize"));
  }, height);
}

export async function expectReachableComposerOption(surface: Locator, option: Locator) {
  await expect
    .poll(() =>
      surface.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        const viewport = window.visualViewport!;
        return (
          rect.height >= 76 &&
          rect.top >= viewport.offsetTop &&
          rect.bottom <= viewport.offsetTop + viewport.height + 1 &&
          rect.left >= viewport.offsetLeft &&
          rect.right <= viewport.offsetLeft + viewport.width + 1
        );
      }),
    )
    .toBe(true);
  await expect
    .poll(() =>
      option.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        const hit = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
        return rect.height >= 44 && element.contains(hit);
      }),
    )
    .toBe(true);
}

export async function expandComposerViewport(page: Page) {
  await page.evaluate(() => {
    // Keep the same EventTarget so the open menu's subscriptions must reflow it.
    const viewport = window.visualViewport!;
    Object.assign(viewport, { offsetTop: viewport.offsetTop - 50, height: viewport.height + 50 });
    viewport.dispatchEvent(new Event("scroll"));
    viewport.dispatchEvent(new Event("resize"));
  });
}
