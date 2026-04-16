"use client";

import { useLayoutEffect, type RefObject } from "react";

const PINNED_CLASS = "dv-pinned-tab";
const OFFSET_VAR = "--dv-pin-offset";

/**
 * Recompute sticky `left` offsets for every pinned tab inside `container` so
 * that multiple pinned tabs stack side-by-side instead of overlapping at x=0.
 *
 * The first pinned tab sticks at `left: 0`, the second at `left: width_of_first`,
 * and so on. Widths include horizontal margins so the trailing `margin: 0 2px`
 * gap between `.dv-tab`s is preserved when the tabs are pinned.
 *
 * Exported for unit testing.
 */
export function updatePinnedOffsets(container: HTMLElement | null): void {
  if (!container) return;
  const pinned = container.querySelectorAll<HTMLElement>(`.${PINNED_CLASS}`);
  let offset = 0;
  pinned.forEach((el) => {
    el.style.setProperty(OFFSET_VAR, `${offset}px`);
    const style = getComputedStyle(el);
    const marginLeft = parseFloat(style.marginLeft) || 0;
    const marginRight = parseFloat(style.marginRight) || 0;
    offset += el.offsetWidth + marginLeft + marginRight;
  });
}

/**
 * Mark the parent `.dv-tab` element (created by Dockview) as pinned so it stays
 * visible on the left when the tab strip overflows horizontally.
 *
 * `ref` must point to an element rendered directly inside the Dockview tab
 * wrapper — we walk up to `.dv-tab` and attach the pin class there.
 *
 * Uses `useLayoutEffect` so the class is applied before the browser paints,
 * avoiding a one-frame flash of a non-sticky tab when the strip is already
 * scrolled at mount time.
 */
export function usePinnedDockviewTab(ref: RefObject<HTMLElement | null>): void {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const dvTab = el.closest<HTMLElement>(".dv-tab");
    if (!dvTab) return;

    dvTab.classList.add(PINNED_CLASS);
    const tabsContainer = dvTab.parentElement;
    updatePinnedOffsets(tabsContainer);

    // Recompute when this tab or a sibling resizes (e.g. title changes length,
    // another session tab appears/disappears).
    const resizeObserver = new ResizeObserver(() => updatePinnedOffsets(tabsContainer));
    resizeObserver.observe(dvTab);

    // Recompute when siblings are added/removed (new pinned tabs, reordering).
    const mutationObserver = tabsContainer
      ? new MutationObserver(() => updatePinnedOffsets(tabsContainer))
      : null;
    if (tabsContainer) {
      mutationObserver?.observe(tabsContainer, { childList: true });
    }

    return () => {
      resizeObserver.disconnect();
      mutationObserver?.disconnect();
      dvTab.classList.remove(PINNED_CLASS);
      dvTab.style.removeProperty(OFFSET_VAR);
      updatePinnedOffsets(tabsContainer);
    };
  }, [ref]);
}
