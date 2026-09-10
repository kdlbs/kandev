import { expect, type Locator, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "./animations";

export async function readMenuBackdrop(menu: Locator) {
  return menu.evaluate((element) => {
    const positioner = element.parentElement;
    if (!positioner?.hasAttribute("data-radix-popper-content-wrapper")) {
      throw new Error("Menu has no positioning wrapper");
    }
    const backdrop = getComputedStyle(positioner, "::before");
    return {
      content: backdrop.content,
      background: backdrop.backgroundColor,
      blur: backdrop.backdropFilter || backdrop.getPropertyValue("-webkit-backdrop-filter"),
      position: backdrop.position,
      inset: [backdrop.top, backdrop.right, backdrop.bottom, backdrop.left],
      width: Number.parseFloat(backdrop.width),
      height: Number.parseFloat(backdrop.height),
      zIndex: Number.parseInt(backdrop.zIndex, 10),
      pointerEvents: backdrop.pointerEvents,
      foregroundFilter: getComputedStyle(element).filter,
      wrapperTransform: getComputedStyle(positioner).transform,
      viewport: { width: window.innerWidth, height: window.innerHeight },
      supportsBlur:
        CSS.supports("backdrop-filter", "blur(1px)") ||
        CSS.supports("-webkit-backdrop-filter", "blur(1px)"),
    };
  });
}

export async function expectMenuBackdropCount(page: Page, count: number) {
  await expect
    .poll(() =>
      page.locator("[data-radix-popper-content-wrapper]").evaluateAll(
        (positioners) =>
          positioners.filter((positioner) => {
            if (!positioner.querySelector(':scope > [role="menu"]')) return false;
            const style = getComputedStyle(positioner, "::before");
            return !["none", "normal"].includes(style.content) && style.display !== "none";
          }).length,
      ),
    )
    .toBe(count);
}

export async function expectMobileMenuBackdrop(menu: Locator) {
  await expect(menu).toBeVisible();
  await waitForFiniteAnimations(menu);
  await expect.poll(() => readMenuBackdrop(menu)).toMatchObject({ content: '""' });
  const backdrop = await readMenuBackdrop(menu);
  expect(backdrop.background).not.toBe("rgba(0, 0, 0, 0)");
  expect(backdrop.position).toBe("fixed");
  expect(backdrop.inset).toEqual(["0px", "0px", "0px", "0px"]);
  expect(backdrop.width).toBeCloseTo(backdrop.viewport.width, 0);
  expect(backdrop.height).toBeCloseTo(backdrop.viewport.height, 0);
  expect(backdrop.zIndex).toBeLessThan(0);
  expect(backdrop.pointerEvents).toBe("none");
  expect(backdrop.foregroundFilter).toMatch(/^(none|blur\(0px\))$/);
  expect(backdrop.wrapperTransform).toBe("none");
  if (backdrop.supportsBlur) {
    expect(
      Number.parseFloat(backdrop.blur.match(/blur\(([\d.]+)px\)/)?.[1] ?? "0"),
    ).toBeGreaterThan(0);
  }
  return backdrop;
}

export async function expectReachableMenuItem(item: Locator) {
  await item.scrollIntoViewIfNeeded();
  await expect(item).toBeVisible();
  const hit = await item.evaluate(async (element) => {
    const menu = element.closest('[role="menu"]');
    await Promise.all(
      (menu?.getAnimations({ subtree: true }) ?? [])
        .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().iterations))
        .map((animation) => animation.finished.catch(() => undefined)),
    );
    const box = element.getBoundingClientRect();
    const target = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return { reachable: !!target && element.contains(target), height: box.height };
  });
  expect(hit.reachable).toBe(true);
  expect(Math.round(hit.height)).toBeGreaterThanOrEqual(44);
}
